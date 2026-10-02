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
	"strings"
	"syscall"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/api"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/connector"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/demo"
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
	demoMode := flag.Bool("demo", env("UMBRELLA_DEMO", "true") == "true", "seed demo data and generate traffic")
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
	eng := alert.New(st, pd, hub.Publish)
	pd.SetResult(eng.PDResult)
	rt := connector.New(st, eng, connector.EnvSecrets{}, hub.Publish)

	var gen *demo.Generator
	if *demoMode {
		demo.Seed(st)
		gen = demo.NewGenerator(rt, eng)
		gen.Backfill(st)
	}

	go pd.Run(ctx)
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
	srv := &http.Server{
		Addr: *addr,
		Handler: api.New(api.Config{WebDir: webDir, GrafanaURL: os.Getenv("UMBRELLA_GRAFANA_URL"),
			WebhookSecret: os.Getenv("UMBRELLA_PD_WEBHOOK_SECRET")}, st, eng, rt, pd, hub).Handler(),
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
