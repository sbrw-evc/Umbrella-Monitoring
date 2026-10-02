// Command umbrella runs the Umbrella MVP as one process: Core API, Ingest
// Gateway, Connector Runtime, Alert Engine and PagerDuty Gateway. The target
// deployment splits them into containers that talk over NATS JetStream.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/api"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/connector"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/demo"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/notify"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pagerduty"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	addr := flag.String("addr", env("UMBRELLA_ADDR", ":8080"), "listen address")
	web := flag.String("web", env("UMBRELLA_WEB_DIR", "../web/dist"), "built web UI directory")
	demoMode := flag.Bool("demo", env("UMBRELLA_DEMO", "false") == "true", "seed demo data and generate traffic")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hub := api.NewHub()
	if o := env("UMBRELLA_WS_ORIGINS", "localhost:*,127.0.0.1:*"); o != "" {
		hub.Origins = strings.Split(o, ",")
	}
	st := store.New()
	pd := pagerduty.New(pagerduty.Config{
		EventsURL:     os.Getenv("UMBRELLA_PD_EVENTS_URL"),
		RoutingKey:    os.Getenv("UMBRELLA_PD_ROUTING_KEY"),
		WebhookSecret: os.Getenv("UMBRELLA_PD_WEBHOOK_SECRET"),
		PublicURL:     env("UMBRELLA_PUBLIC_URL", "http://localhost:8080"),
	})
	notifier := notify.New(notify.Config{PublicURL: env("UMBRELLA_PUBLIC_URL", "http://localhost:8080"),
		AllowHTTP: env("UMBRELLA_ALLOW_HTTP_WEBHOOKS", "false") == "true"}, st, connector.EnvSecrets{})
	// Every live update also goes to the notifier, which turns alert changes
	// into Teams and Zoom messages.
	publish := func(kind string, v any) {
		hub.Publish(kind, v)
		notifier.Observe(kind, v)
	}
	eng := alert.New(st, pd, publish)
	pd.SetResult(eng.PDResult)
	rt := connector.New(st, eng, connector.EnvSecrets{}, publish)

	var gen *demo.Generator
	if *demoMode {
		demo.Seed(st)
		gen = demo.NewGenerator(rt, eng)
		gen.Backfill(st)
	} else {
		// Clean start: no CMDB, connectors or incidents; only the rule
		// catalog. Unknown CIs from events build the CMDB as they arrive.
		st.Write(func(d *store.Data) { d.Rules = alert.DefaultRules() })
	}
	eng.AutoCMDB = env("UMBRELLA_CMDB_AUTO", strconv.FormatBool(!*demoMode)) == "true"

	var tokens []auth.ServiceToken
	if t := os.Getenv("UMBRELLA_GRAFANA_TOKEN"); t != "" {
		tokens = append(tokens, auth.ServiceToken{User: "grafana", Role: auth.RoleReader, Token: t})
	}
	auth.Bootstrap(st, auth.BootstrapConfig{AdminUser: env("UMBRELLA_ADMIN_USER", "admin"),
		AdminPassword: os.Getenv("UMBRELLA_ADMIN_PASSWORD"), ServiceTokens: tokens})

	go pd.Run(ctx)
	go notifier.Run(ctx)
	go rt.Run(ctx)
	go func() {
		tk := time.NewTicker(5 * time.Second)
		defer tk.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tk.C:
				eng.Tick()
			}
		}
	}()
	if gen != nil {
		go gen.Run(ctx, 4*time.Second)
	}

	webDir := *web
	if st, err := os.Stat(webDir); err != nil || !st.IsDir() {
		slog.Warn("web UI not found, serving API only", "dir", webDir)
		webDir = ""
	}
	server := api.New(api.Config{WebDir: webDir, GrafanaURL: os.Getenv("UMBRELLA_GRAFANA_URL"),
		WebhookSecret:     os.Getenv("UMBRELLA_PD_WEBHOOK_SECRET"),
		SecureCookies:     env("UMBRELLA_SECURE_COOKIES", "false") == "true",
		MetricsToken:      os.Getenv("UMBRELLA_METRICS_TOKEN"),
		AllowHTTPWebhooks: env("UMBRELLA_ALLOW_HTTP_WEBHOOKS", "false") == "true"}, st, eng, rt, pd, hub)
	server.SetNotifier(notifier)
	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	slog.Info("umbrella started", "addr", *addr, "demo", *demoMode, "pagerduty", pd.Status().Mode)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}
