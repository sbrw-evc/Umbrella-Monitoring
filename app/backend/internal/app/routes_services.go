package app

import (
	"errors"
	"net/http"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
)

func (a *App) registerServices(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/services", a.authed(a.can("services:view", a.listServices)))
	mux.HandleFunc("GET /api/services/{id}", a.authed(a.can("services:view", a.getService)))
	mux.HandleFunc("POST /api/services", a.authed(a.can("services:edit", a.createService)))
	mux.HandleFunc("PUT /api/services/{id}", a.authed(a.can("services:edit", a.updateService)))
	mux.HandleFunc("DELETE /api/services/{id}", a.authed(a.can("services:edit", a.deleteService)))
	mux.HandleFunc("GET /api/teams/{id}/services", a.authed(a.can("services:view", a.teamServices)))
}

func (a *App) listServices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	httpx.JSON(w, http.StatusOK, a.services.List(ServiceFilter{
		Query:       q.Get("q"),
		Team:        q.Get("team"),
		Owner:       q.Get("owner"),
		Descendants: q.Get("descendants") != "false",
		Criticality: q.Get("criticality"),
		Status:      q.Get("status"),
		Tag:         q.Get("tag"),
	}))
}

func (a *App) getService(w http.ResponseWriter, r *http.Request) {
	out, err := a.services.Get(r.PathValue("id"))
	respondService(w, http.StatusOK, out, err)
}

func (a *App) createService(w http.ResponseWriter, r *http.Request) {
	var in ServiceInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	out, err := a.services.Create(current(r).user.Username, in)
	respondService(w, http.StatusCreated, out, err)
}

func (a *App) updateService(w http.ResponseWriter, r *http.Request) {
	var in ServiceInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	out, err := a.services.Update(current(r).user.Username, r.PathValue("id"), in)
	respondService(w, http.StatusOK, out, err)
}

func (a *App) deleteService(w http.ResponseWriter, r *http.Request) {
	if err := a.services.Delete(current(r).user.Username, r.PathValue("id")); err != nil {
		serviceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) teamServices(w http.ResponseWriter, r *http.Request) {
	out, err := a.services.ForTeam(r.PathValue("id"), r.URL.Query().Get("descendants") == "true")
	respondService(w, http.StatusOK, out, err)
}

func respondService(w http.ResponseWriter, status int, out any, err error) {
	if err != nil {
		serviceError(w, err)
		return
	}
	httpx.JSON(w, status, out)
}

func serviceError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrServiceNameTaken) {
		httpx.Error(w, http.StatusConflict, "service_name_taken", nil)
		return
	}
	writeError(w, err)
}
