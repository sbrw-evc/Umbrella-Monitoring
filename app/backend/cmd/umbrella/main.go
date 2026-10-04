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
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/app"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/config"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/secrets"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/setup"
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

type switchHandler struct{ h atomic.Value }

func (s *switchHandler) Set(h http.Handler) { s.h.Store(&h) }

func (s *switchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	(*s.h.Load().(*http.Handler)).ServeHTTP(w, r)
}

type runtime struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	backend *store.PGBackend
}

func (rt *runtime) stop() {
	rt.mu.Lock()
	cancel, done, backend := rt.cancel, rt.done, rt.backend
	rt.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
	backend.Close()
}

func main() {
	addr := flag.String("addr", env("UMBRELLA_ADDR", ":8080"), "listen address")
	web := flag.String("web", env("UMBRELLA_WEB_DIR", "../web/dist"), "built web UI directory")
	dataDir := flag.String("data", env("UMBRELLA_DATA_DIR", "data"), "directory for umbrella.json and sessions")
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
	if *dataDir == "" {
		fail("data directory", errors.New("UMBRELLA_DATA_DIR must not be empty"))
	}
	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		fail("data directory", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	webDir := *web
	if fi, err := os.Stat(webDir); err != nil || !fi.IsDir() {
		slog.Warn("web UI not found, serving API only", "dir", webDir)
		webDir = ""
	}
	webHandler := httpx.Web(webDir)
	opts := app.Options{Version: version, SecureCookies: env("UMBRELLA_SECURE_COOKIES", "false") == "true", Web: webHandler}
	sessions := auth.NewSessions()
	if err := sessions.Load(*dataDir); err != nil {
		slog.Warn("sessions not restored", "err", err)
	}

	rt := &runtime{}
	handler := &switchHandler{}
	start := func(cfg config.File, vault *secrets.Client, backend *store.PGBackend, st *store.Store) {
		bg, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go vault.Run(bg)
		go func() {
			st.Run(bg, 2*time.Second, func() error { return sessions.Save(*dataDir) })
			close(done)
		}()
		rt.mu.Lock()
		rt.cancel, rt.done, rt.backend = cancel, done, backend
		rt.mu.Unlock()
		a := app.New(opts, app.Deps{Config: cfg, Vault: vault, Backend: backend, Store: st, Sessions: sessions})
		handler.Set(a.Handler())
	}

	cfg, found, err := config.Read(*dataDir)
	if err != nil {
		fail("configuration", err)
	}
	if found && cfg.CompletedAt != nil {
		vault, backend, st, err := open(ctx, cfg)
		if err != nil {
			fail("startup", err)
		}
		start(cfg, vault, backend, st)
		slog.Info("umbrella ready", "openbao", cfg.OpenBao.Addr, "storage", backend.Where())
	} else {
		token, err := setupToken(*dataDir)
		if err != nil {
			fail("setup code", err)
		}
		mod := setup.New(setup.Options{DataDir: *dataDir, Token: token, Version: version, Web: webHandler},
			func(r setup.Result) { start(r.Config, r.Vault, r.Backend, r.Store) })
		handler.Set(mod.Handler())
		slog.Warn("first start: open the web UI and finish the setup wizard", "setup_code", token,
			"file", filepath.Join(*dataDir, config.SetupTokenFile))
	}

	srv := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	slog.Info("umbrella started", "version", version, "addr", *addr, "data_dir", *dataDir)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		rt.stop()
		fail("server failed", err)
	}
	rt.stop()
	if err := sessions.Save(*dataDir); err != nil {
		slog.Warn("sessions not saved", "err", err)
	}
	slog.Info("umbrella stopped")
}

func open(ctx context.Context, cfg config.File) (*secrets.Client, *store.PGBackend, *store.Store, error) {
	vault, err := cfg.OpenBao.Client()
	if err != nil {
		return nil, nil, nil, err
	}
	wait, err := time.ParseDuration(env("UMBRELLA_STARTUP_WAIT", "2m"))
	if err != nil {
		wait = 2 * time.Minute
	}
	if err := vault.Ready(ctx, wait); err != nil {
		return nil, nil, nil, fmt.Errorf("OpenBao %s: %w", cfg.OpenBao.Addr, err)
	}
	pg := cfg.Postgres
	if cfg.PostgresPasswordRef != "" {
		if pg.Password, err = vault.Resolve(cfg.PostgresPasswordRef); err != nil {
			return nil, nil, nil, fmt.Errorf("PostgreSQL password: %w", err)
		}
	}
	deadline := time.Now().Add(wait)
	var backend *store.PGBackend
	for {
		backend, err = store.OpenPostgres(ctx, pg)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return nil, nil, nil, err
		}
		slog.Info("waiting for PostgreSQL", "err", err)
		select {
		case <-ctx.Done():
			return nil, nil, nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	st := store.New()
	restored, err := st.Attach(ctx, backend)
	if err != nil {
		backend.Close()
		return nil, nil, nil, err
	}
	if !restored {
		backend.Close()
		return nil, nil, nil, errors.New("the database has no Umbrella state: restore it or remove umbrella.json to run the setup wizard again")
	}
	return vault, backend, st, nil
}

func setupToken(dir string) (string, error) {
	if t := strings.TrimSpace(os.Getenv("UMBRELLA_SETUP_TOKEN")); t != "" {
		return t, nil
	}
	path := filepath.Join(dir, config.SetupTokenFile)
	if b, err := os.ReadFile(path); err == nil {
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, nil
		}
	}
	t := auth.RandomToken("", 12)
	if err := store.WriteFileAtomic(path, []byte(t+"\n")); err != nil {
		return "", err
	}
	return t, nil
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
