package app

import (
	"net/http"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type managedUserView struct {
	userView
	DisplayName string `json:"display_name"`
	TeamName    string `json:"team_name,omitempty"`
	// Services are the business services of the incident scope; Missing marks one deleted since.
	Services []userServiceRef `json:"services"`
}

type userServiceRef struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Missing bool   `json:"missing,omitempty"`
}

func (a *App) registerUsers(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/users", a.authed(a.can("users:view", a.listUsers)))
	mux.HandleFunc("GET /api/users/{id}", a.authed(a.can("users:view", a.getUser)))
	mux.HandleFunc("POST /api/users", a.authed(a.can("users:create", a.createUser)))
	mux.HandleFunc("PUT /api/users/{id}", a.authed(a.can("users:edit", a.editUser)))
	mux.HandleFunc("POST /api/users/{id}/lock", a.authed(a.can("users:lock", a.lockUser(true))))
	mux.HandleFunc("POST /api/users/{id}/unlock", a.authed(a.can("users:lock", a.lockUser(false))))
	mux.HandleFunc("PUT /api/users/{id}/password", a.authed(a.can("users:password", a.resetPassword)))
	mux.HandleFunc("DELETE /api/users/{id}", a.authed(a.can("users:delete", a.deleteUser)))
}

func actorOf(r *http.Request) Actor {
	s := current(r)
	return Actor{User: s.user, Session: s.ss.ID}
}

func (a *App) managedViews(users []model.User) []managedUserView {
	teams, services := map[string]string{}, map[string]string{}
	a.deps.Store.Read(func(d *store.Data) {
		for id, t := range d.Teams {
			teams[id] = t.Name
		}
		for id, s := range d.Services {
			services[id] = s.Name
		}
	})
	out := make([]managedUserView, 0, len(users))
	for _, u := range users {
		v := a.view(u, "")
		v.Permissions = nil
		refs := make([]userServiceRef, 0, len(u.ServiceIDs))
		for _, id := range u.ServiceIDs {
			name, ok := services[id]
			refs = append(refs, userServiceRef{ID: id, Name: userOr(name, id), Missing: !ok})
		}
		out = append(out, managedUserView{userView: v, DisplayName: u.Profile.DisplayName(u.Username), TeamName: teams[u.TeamID], Services: refs})
	}
	return out
}

func (a *App) respondManaged(w http.ResponseWriter, status int, u model.User, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, status, a.managedViews([]model.User{u})[0])
}

func (a *App) listUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	users := a.accounts.List(UserFilter{Query: q.Get("q"), Source: q.Get("source"), Role: q.Get("role"), Team: q.Get("team"), Status: q.Get("status")})
	httpx.JSON(w, http.StatusOK, a.managedViews(users))
}

func (a *App) getUser(w http.ResponseWriter, r *http.Request) {
	u, err := a.accounts.Get(r.PathValue("id"))
	a.respondManaged(w, http.StatusOK, u, err)
}

func (a *App) createUser(w http.ResponseWriter, r *http.Request) {
	var in NewUser
	if !httpx.Decode(w, r, &in) {
		return
	}
	u, err := a.accounts.Create(r.Context(), actorOf(r), in)
	a.respondManaged(w, http.StatusCreated, u, err)
}

func (a *App) editUser(w http.ResponseWriter, r *http.Request) {
	var in UserChanges
	if !httpx.Decode(w, r, &in) {
		return
	}
	u, err := a.accounts.Update(actorOf(r), r.PathValue("id"), in)
	a.respondManaged(w, http.StatusOK, u, err)
}

func (a *App) lockUser(locked bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := a.accounts.SetLocked(actorOf(r), r.PathValue("id"), locked)
		a.respondManaged(w, http.StatusOK, u, err)
	}
}

func (a *App) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in PasswordReset
	if !httpx.Decode(w, r, &in) {
		return
	}
	u, err := a.accounts.SetPassword(r.Context(), actorOf(r), r.PathValue("id"), in)
	a.respondManaged(w, http.StatusOK, u, err)
}

func (a *App) deleteUser(w http.ResponseWriter, r *http.Request) {
	if err := a.accounts.Delete(r.Context(), actorOf(r), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
