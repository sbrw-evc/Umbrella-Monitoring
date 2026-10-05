package app

import (
	"context"
	"net/http"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
)

func (a *App) registerMonitoring(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/monitoring", a.authed(a.can("monitoring:view", a.monitoringView)))
	mux.HandleFunc("POST /api/monitoring/sources", a.authed(a.can("monitoring:edit", a.createMonitoringSource)))
	mux.HandleFunc("PUT /api/monitoring/sources/{id}", a.authed(a.can("monitoring:edit", a.updateMonitoringSource)))
	mux.HandleFunc("DELETE /api/monitoring/sources/{id}", a.authed(a.can("monitoring:edit", a.deleteMonitoringSource)))
	mux.HandleFunc("POST /api/monitoring/test", a.authed(a.can("monitoring:test", a.testMonitoringSource)))
	mux.HandleFunc("POST /api/monitoring/sources/{id}/sync", a.authed(a.can("monitoring:sync", a.syncMonitoringSource)))
	mux.HandleFunc("GET /api/monitoring/hosts", a.authed(a.can("monitoring:view", a.monitoringHosts)))
	mux.HandleFunc("POST /api/monitoring/link", a.authed(a.can("monitoring:link", a.linkHost)))
	mux.HandleFunc("POST /api/monitoring/ci", a.authed(a.can("monitoring:link", a.can("cis:edit", a.createCIFromHost))))
	mux.HandleFunc("POST /api/monitoring/ci/bulk", a.authed(a.can("monitoring:link", a.can("cis:edit", a.bulkCreateCIs))))
}

func (a *App) monitoringView(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, a.monitoring.View())
}

func (a *App) createMonitoringSource(w http.ResponseWriter, r *http.Request) {
	var in MonitoringSourceInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	out, err := a.monitoring.Create(current(r).user.Username, in)
	respond(w, http.StatusCreated, out, err)
}

func (a *App) updateMonitoringSource(w http.ResponseWriter, r *http.Request) {
	var in MonitoringSourceInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	out, err := a.monitoring.Update(current(r).user.Username, r.PathValue("id"), in)
	respond(w, http.StatusOK, out, err)
}

func (a *App) deleteMonitoringSource(w http.ResponseWriter, r *http.Request) {
	if err := a.monitoring.Delete(current(r).user.Username, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) testMonitoringSource(w http.ResponseWriter, r *http.Request) {
	var in MonitoringSourceInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	out, err := a.monitoring.Test(r.Context(), in)
	respond(w, http.StatusOK, out, err)
}

func (a *App) syncMonitoringSource(w http.ResponseWriter, r *http.Request) {
	// The reading finishes even if the browser stops waiting.
	out, err := a.monitoring.Sync(context.WithoutCancel(r.Context()), current(r).user.Username, r.PathValue("id"))
	respond(w, http.StatusOK, out, err)
}

func (a *App) monitoringHosts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	httpx.JSON(w, http.StatusOK, a.monitoring.Hosts(HostFilter{Source: q.Get("source"), Match: q.Get("match"), Query: q.Get("q")}))
}

func (a *App) linkHost(w http.ResponseWriter, r *http.Request) {
	var in LinkInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	out, err := a.monitoring.Link(current(r).user.Username, in)
	respond(w, http.StatusOK, out, err)
}

func (a *App) createCIFromHost(w http.ResponseWriter, r *http.Request) {
	var in CreateCIInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	out, err := a.monitoring.CreateCI(r.Context(), current(r).user.Username, in)
	respondNetBox(w, http.StatusCreated, out, err)
}

func (a *App) bulkCreateCIs(w http.ResponseWriter, r *http.Request) {
	var in BulkHostsInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	// The batch finishes even if the browser stops waiting.
	out, err := a.monitoring.BulkCreateCIs(context.WithoutCancel(r.Context()), current(r).user.Username, in)
	respond(w, http.StatusOK, out, err)
}
