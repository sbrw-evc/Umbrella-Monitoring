package app

import "net/http"

func (a *App) registerTeams(mux *http.ServeMux) {
	a.route(mux, "GET /api/teams", "teams:view", show(a.teams.List))
	a.route(mux, "GET /api/teams/{id}", "teams:view", fetch(func(r *http.Request) (TeamView, error) {
		return a.teams.Get(r.PathValue("id"))
	}))
	a.route(mux, "POST /api/teams", "teams:edit", submit(http.StatusCreated, func(r *http.Request, in TeamInput) (TeamView, error) {
		return a.teams.Create(actorName(r), in)
	}))
	a.route(mux, "PUT /api/teams/{id}", "teams:edit", submit(http.StatusOK, func(r *http.Request, in TeamInput) (TeamView, error) {
		return a.teams.Update(actorName(r), r.PathValue("id"), in)
	}))
	a.route(mux, "DELETE /api/teams/{id}", "teams:edit", remove(func(r *http.Request) error {
		return a.teams.Delete(actorName(r), r.PathValue("id"))
	}))
	a.route(mux, "PUT /api/teams/{id}/members", "teams:edit", submit(http.StatusOK, func(r *http.Request, in membersInput) (TeamView, error) {
		return a.teams.SetMembers(actorName(r), r.PathValue("id"), in.UserIDs)
	}))
}
