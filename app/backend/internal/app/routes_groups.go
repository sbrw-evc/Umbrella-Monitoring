package app

import (
	"net/http"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

func (a *App) registerGroups(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings/groups", a.authed(a.can("settings.ldap:view", a.groupSettings)))
	mux.HandleFunc("PUT /api/settings/groups", a.authed(a.can("settings.ldap:edit", a.saveGroups)))
	mux.HandleFunc("POST /api/settings/groups/sync", a.authed(a.can("settings.ldap:edit", a.syncGroups)))
}

func (a *App) groupSettings(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, a.groups.View())
}

func (a *App) saveGroups(w http.ResponseWriter, r *http.Request) {
	var in model.GroupMappings
	if !httpx.Decode(w, r, &in) {
		return
	}
	out, err := a.groups.Save(current(r).user.Username, in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *App) syncGroups(w http.ResponseWriter, r *http.Request) {
	out, err := a.groups.Sync(r.Context(), current(r).user.Username)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
