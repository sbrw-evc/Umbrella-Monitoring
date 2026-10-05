package app

import (
	"net/http"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

func (a *App) registerPolicy(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings/password-policy", a.authed(a.admin(a.passwordPolicy)))
	mux.HandleFunc("PUT /api/settings/password-policy", a.authed(a.admin(a.updatePasswordPolicy)))
}

func (a *App) passwordPolicy(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, a.policy.Summary())
}

func (a *App) updatePasswordPolicy(w http.ResponseWriter, r *http.Request) {
	var in model.PasswordPolicy
	if !httpx.Decode(w, r, &in) {
		return
	}
	out, err := a.policy.Update(current(r).user.Username, in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
