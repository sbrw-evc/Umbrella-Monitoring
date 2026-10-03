package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/api"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/connector"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/integration"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/notify"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pagerduty"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/rules"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/secrets"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

var version = "dev"

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func fail(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}

func main() {
	addr := flag.String("addr", env("UMBRELLA_ADDR", ":8080"), "listen address")
	web := flag.String("web", env("UMBRELLA_WEB_DIR", "../web/dist"), "built web UI directory")
	dataDir := flag.String("data", env("UMBRELLA_DATA_DIR", "data"), "state directory; empty keeps state in memory only")
	showVersion := flag.Bool("version", false, "print the version and exit")
	healthcheck := flag.Bool("healthcheck", false, "probe /healthz of the running server and exit 0 when it answers")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *healthcheck {
		os.Exit(probe(*addr))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	publicURL := env("UMBRELLA_PUBLIC_URL", "http://localhost:8080")

	vault, err := secrets.New(secrets.Config{
		Addr:               os.Getenv("UMBRELLA_OPENBAO_ADDR"),
		Mount:              env("UMBRELLA_OPENBAO_MOUNT", "umbrella"),
		Namespace:          os.Getenv("UMBRELLA_OPENBAO_NAMESPACE"),
		Token:              os.Getenv("UMBRELLA_OPENBAO_TOKEN"),
		TokenFile:          os.Getenv("UMBRELLA_OPENBAO_TOKEN_FILE"),
		RoleID:             os.Getenv("UMBRELLA_OPENBAO_ROLE_ID"),
		RoleIDFile:         os.Getenv("UMBRELLA_OPENBAO_ROLE_ID_FILE"),
		SecretID:           os.Getenv("UMBRELLA_OPENBAO_SECRET_ID"),
		SecretIDFile:       os.Getenv("UMBRELLA_OPENBAO_SECRET_ID_FILE"),
		CACert:             os.Getenv("UMBRELLA_OPENBAO_CACERT"),
		InsecureSkipVerify: env("UMBRELLA_OPENBAO_SKIP_VERIFY", "false") == "true",
	})
	if err != nil {
		fail("openbao client", err)
	}
	if vault.Enabled() {
		wait, err := time.ParseDuration(env("UMBRELLA_OPENBAO_WAIT", "2m"))
		if err != nil {
			wait = 2 * time.Minute
		}
		if err := vault.Ready(ctx, wait); err != nil {
			fail("openbao is not ready", err)
		}
		slog.Info("openbao connected", "addr", os.Getenv("UMBRELLA_OPENBAO_ADDR"), "mount", vault.Mount())
		go vault.Run(ctx)
	} else {
		slog.Warn("OpenBao is not configured: secrets cannot be stored, PagerDuty, integrations and channels stay unavailable",
			"hint", "set UMBRELLA_OPENBAO_ADDR and UMBRELLA_OPENBAO_ROLE_ID/SECRET_ID or UMBRELLA_OPENBAO_TOKEN")
	}
	secret := func(key string) string {
		v, err := secrets.Value(vault, os.Getenv(key))
		if errors.Is(err, secrets.ErrNotFound) {
			slog.Warn("secret not found in OpenBao, the setting stays empty", "env", key, "ref", os.Getenv(key))
			return ""
		}
		if err != nil {
			fail(key, err)
		}
		return v
	}

	st := store.New()
	if *dataDir != "" {
		cfg, err := store.ReadStorageConfig(*dataDir)
		if err != nil {
			fail("storage config", err)
		}
		backend, err := openStorage(ctx, cfg, *dataDir, vault)
		if err != nil {
			fail("storage", err)
		}
		restored, err := st.Attach(ctx, backend)
		if err != nil {
			fail("state", err)
		}
		if !restored {
			slog.Info("clean start", "storage", backend.Kind(), "where", backend.Where())
		}
	} else {
		slog.Warn("UMBRELLA_DATA_DIR is empty: state lives in memory and is lost on restart")
	}

	hub := api.NewHub()
	if o := env("UMBRELLA_WS_ORIGINS", "localhost:*,127.0.0.1:*"); o != "" {
		hub.Origins = strings.Split(o, ",")
	}
	pd := pagerduty.New(pagerduty.Config{PublicURL: publicURL}, st, vault)
	notifier := notify.New(notify.Config{PublicURL: publicURL,
		AllowHTTP: env("UMBRELLA_ALLOW_HTTP_WEBHOOKS", "false") == "true"}, st, vault)
	publish := func(kind string, v any) {
		hub.Publish(kind, v)
		notifier.Observe(kind, v)
	}
	eng := alert.New(st, pd, publish)
	eng.AutoCMDB = env("UMBRELLA_CMDB_AUTO", "true") == "true"
	pd.SetResult(eng.PDResult)
	pd.SetInbound(eng.PDInbound)
	rt := connector.New(st, eng, vault, publish)
	integrations := integration.NewManager(st, vault, publicURL)
	integrations.SetFeed(rt.Webhook)
	ruleEngine := rules.New(st, integrations, eng)

	var tokens []auth.ServiceToken
	if t := secret("UMBRELLA_GRAFANA_TOKEN"); t != "" {
		tokens = append(tokens, auth.ServiceToken{User: "grafana", Role: auth.RoleReader, Token: t})
	}
	auth.Bootstrap(st, auth.BootstrapConfig{AdminUser: env("UMBRELLA_ADMIN_USER", "admin"),
		AdminPassword: secret("UMBRELLA_ADMIN_PASSWORD"), ServiceTokens: tokens})

	setupToken := ""
	st.Read(func(d *store.Data) {
		if d.Settings.SetupCompleted {
			return
		}
		for _, u := range d.Users {
			if !u.Service {
				return
			}
		}
		setupToken = auth.RandomToken("", 9)
	})
	if setupToken != "" {
		if *dataDir != "" {
			_ = os.MkdirAll(*dataDir, 0o700)
			_ = os.WriteFile(filepath.Join(*dataDir, api.SetupTokenFile), []byte(setupToken+"\n"), 0o600)
		}
		slog.Warn("first start: open the web UI and finish the setup wizard", "setup_code", setupToken,
			"file", filepath.Join(*dataDir, api.SetupTokenFile))
	}

	webDir := *web
	if fi, err := os.Stat(webDir); err != nil || !fi.IsDir() {
		slog.Warn("web UI not found, serving API only", "dir", webDir)
		webDir = ""
	}
	server := api.New(api.Config{WebDir: webDir, PublicURL: publicURL, Version: version, DataDir: *dataDir, SetupToken: setupToken,
		SecureCookies:     env("UMBRELLA_SECURE_COOKIES", "false") == "true",
		MetricsToken:      secret("UMBRELLA_METRICS_TOKEN"),
		AllowHTTPWebhooks: env("UMBRELLA_ALLOW_HTTP_WEBHOOKS", "false") == "true"},
		api.Deps{Store: st, Engine: eng, Runtime: rt, PagerDuty: pd, Hub: hub, Vault: vault, Integrations: integrations, Rules: ruleEngine})
	server.SetNotifier(notifier)
	sessions := server.Sessions()
	if *dataDir != "" {
		if err := sessions.Load(*dataDir); err != nil {
			slog.Warn("sessions not restored", "err", err)
		}
	}

	bg, cancelBg := context.WithCancel(context.Background())
	go pd.Run(bg)
	go pd.RunSync(bg)
	go notifier.Run(bg)
	go rt.Run(bg)
	go ruleEngine.Run(bg)
	go integrations.Run(bg)
	go func() {
		tk := time.NewTicker(5 * time.Second)
		defer tk.Stop()
		for {
			select {
			case <-bg.Done():
				return
			case <-tk.C:
				eng.Tick()
			}
		}
	}()
	saved := make(chan struct{})
	go func() {
		st.Run(bg, 2*time.Second, func() error { return sessions.Save(*dataDir) })
		close(saved)
	}()

	srv := &http.Server{Addr: *addr, Handler: server.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	slog.Info("umbrella started", "version", version, "addr", *addr, "data_dir", *dataDir, "openbao", vault.Enabled(),
		"pagerduty", pd.Status().Enabled)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		cancelBg()
		<-saved
		fail("server failed", err)
	}
	cancelBg()
	<-saved
	slog.Info("umbrella stopped, state saved")
}

func probe(addr string) int {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 1
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func openStorage(ctx context.Context, cfg store.StorageConfig, dir string, vault *secrets.Client) (store.Backend, error) {
	if cfg.Kind != "postgres" {
		return store.FileBackend{Dir: dir}, nil
	}
	deadline := time.Now().Add(2 * time.Minute)
	for {
		b, err := cfg.Open(ctx, dir, vault.Resolve)
		if err == nil {
			slog.Info("state storage connected", "kind", "postgres", "where", b.Where())
			return b, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		slog.Info("waiting for the state database", "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}
