package app

import (
	"net/http"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
)

func (a *App) registerTeams(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/teams", a.authed(a.can("teams:view", a.listTeams)))
	mux.HandleFunc("GET /api/teams/{id}", a.authed(a.can("teams:view", a.getTeam)))
	mux.HandleFunc("POST /api/teams", a.authed(a.can("teams:edit", a.createTeam)))
	mux.HandleFunc("PUT /api/teams/{id}", a.authed(a.can("teams:edit", a.updateTeam)))
	mux.HandleFunc("DELETE /api/teams/{id}", a.authed(a.can("teams:edit", a.deleteTeam)))
	mux.HandleFunc("PUT /api/teams/{id}/members", a.authed(a.can("teams:edit", a.setTeamMembers)))
}

func (a *App) listTeams(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, a.teams.List())
}

func (a *App) getTeam(w http.ResponseWriter, r *http.Request) {
	v, err := a.teams.Get(r.PathValue("id"))
	reply(w, http.StatusOK, v, err)
}

func (a *App) createTeam(w http.ResponseWriter, r *http.Request) {
	var in TeamInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	v, err := a.teams.Create(current(r).user.Username, in)
	reply(w, http.StatusCreated, v, err)
}

func (a *App) updateTeam(w http.ResponseWriter, r *http.Request) {
	var in TeamInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	v, err := a.teams.Update(current(r).user.Username, r.PathValue("id"), in)
	reply(w, http.StatusOK, v, err)
}

func (a *App) deleteTeam(w http.ResponseWriter, r *http.Request) {
	if err := a.teams.Delete(current(r).user.Username, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) setTeamMembers(w http.ResponseWriter, r *http.Request) {
	var in membersInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	v, err := a.teams.SetMembers(current(r).user.Username, r.PathValue("id"), in.UserIDs)
	reply(w, http.StatusOK, v, err)
}
