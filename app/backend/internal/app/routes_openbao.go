package app

import (
	"context"
	"net/http"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/config"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
)

func (a *App) registerOpenBao(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings/openbao", a.authed(a.can("settings.openbao:view", a.openbaoOverview)))
	mux.HandleFunc("POST /api/settings/openbao/test", a.authed(a.can("settings.openbao:test", a.openbaoTest)))
	mux.HandleFunc("POST /api/settings/openbao/probe", a.authed(a.can("settings.openbao:migrate", a.openbaoProbe)))
	mux.HandleFunc("POST /api/settings/openbao/migrate", a.authed(a.can("settings.openbao:migrate", a.openbaoMigrate)))
}

func (a *App) openbaoOverview(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
	defer cancel()
	httpx.JSON(w, http.StatusOK, a.openbao.Overview(ctx))
}

func (a *App) openbaoTest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
	defer cancel()
	httpx.JSON(w, http.StatusOK, a.openbao.Test(ctx))
}

func (a *App) openbaoProbe(w http.ResponseWriter, r *http.Request) {
	var in config.OpenBao
	if !httpx.Decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
	defer cancel()
	httpx.JSON(w, http.StatusOK, a.openbao.Probe(ctx, in))
}

func (a *App) openbaoMigrate(w http.ResponseWriter, r *http.Request) {
	var in OpenBaoMigration
	if !httpx.Decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), migrationTimeout)
	defer cancel()
	out, err := a.openbao.Migrate(ctx, current(r).user.Username, in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
