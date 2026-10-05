package app

import (
	"context"
	"net/http"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/config"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
)

const (
	checkTimeout     = 30 * time.Second
	migrationTimeout = 5 * time.Minute
)

func (a *App) registerPostgres(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings/postgres", a.authed(a.can("settings.postgres:view", a.postgresOverview)))
	mux.HandleFunc("POST /api/settings/postgres/test", a.authed(a.can("settings.postgres:test", a.postgresTest)))
	mux.HandleFunc("GET /api/settings/postgres/stats", a.authed(a.can("settings.postgres:view", a.postgresStats)))
	mux.HandleFunc("POST /api/settings/postgres/probe", a.authed(a.can("settings.postgres:migrate", a.postgresProbe)))
	mux.HandleFunc("POST /api/settings/postgres/migrate", a.authed(a.can("settings.postgres:migrate", a.postgresMigrate)))
}

func (a *App) postgresOverview(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
	defer cancel()
	httpx.JSON(w, http.StatusOK, a.postgres.Overview(ctx))
}

func (a *App) postgresTest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
	defer cancel()
	httpx.JSON(w, http.StatusOK, a.postgres.Test(ctx))
}

func (a *App) postgresStats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
	defer cancel()
	stats, err := a.postgres.Stats(ctx)
	if err != nil {
		writeError(w, invalid("postgres_stats_failed", err))
		return
	}
	httpx.JSON(w, http.StatusOK, stats)
}

func (a *App) postgresProbe(w http.ResponseWriter, r *http.Request) {
	var in config.PostgresTarget
	if !httpx.Decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
	defer cancel()
	httpx.JSON(w, http.StatusOK, a.postgres.Probe(ctx, in))
}

func (a *App) postgresMigrate(w http.ResponseWriter, r *http.Request) {
	var in PostgresMigration
	if !httpx.Decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), migrationTimeout)
	defer cancel()
	out, err := a.postgres.Migrate(ctx, current(r).user.Username, in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
