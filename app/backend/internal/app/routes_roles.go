package app

import (
	"net/http"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
)

type membersInput struct {
	UserIDs []string `json:"user_ids"`
}

type roleDeleteInput struct {
	ReassignTo string `json:"reassign_to"`
}

func (a *App) registerRoles(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/roles", a.authed(a.can("roles:view", a.listRoles)))
	mux.HandleFunc("GET /api/roles/{id}", a.authed(a.can("roles:view", a.getRole)))
	mux.HandleFunc("POST /api/roles", a.authed(a.can("roles:edit", a.createRole)))
	mux.HandleFunc("PUT /api/roles/{id}", a.authed(a.can("roles:edit", a.updateRole)))
	mux.HandleFunc("DELETE /api/roles/{id}", a.authed(a.can("roles:edit", a.deleteRole)))
	mux.HandleFunc("PUT /api/roles/{id}/members", a.authed(a.can("roles:edit", a.addRoleMembers)))
	mux.HandleFunc("PUT /api/roles/{id}/new-users", a.authed(a.can("roles:edit", a.setNewUserRole)))
}

func (a *App) listRoles(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, a.roles.List())
}

func (a *App) getRole(w http.ResponseWriter, r *http.Request) {
	v, err := a.roles.Get(r.PathValue("id"))
	reply(w, http.StatusOK, v, err)
}

func (a *App) createRole(w http.ResponseWriter, r *http.Request) {
	var in RoleInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	v, err := a.roles.Create(current(r).user.Username, in)
	reply(w, http.StatusCreated, v, err)
}

func (a *App) updateRole(w http.ResponseWriter, r *http.Request) {
	var in RoleInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	v, err := a.roles.Update(current(r).user.Username, r.PathValue("id"), in)
	reply(w, http.StatusOK, v, err)
}

func (a *App) deleteRole(w http.ResponseWriter, r *http.Request) {
	in := roleDeleteInput{ReassignTo: r.URL.Query().Get("reassign_to")}
	if in.ReassignTo == "" && r.ContentLength > 0 && !httpx.Decode(w, r, &in) {
		return
	}
	if err := a.roles.Delete(current(r).user, r.PathValue("id"), in.ReassignTo); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) addRoleMembers(w http.ResponseWriter, r *http.Request) {
	var in membersInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	v, err := a.roles.AddMembers(current(r).user, r.PathValue("id"), in.UserIDs)
	reply(w, http.StatusOK, v, err)
}

func (a *App) setNewUserRole(w http.ResponseWriter, r *http.Request) {
	v, err := a.roles.SetNewUserRole(current(r).user.Username, r.PathValue("id"))
	reply(w, http.StatusOK, v, err)
}
