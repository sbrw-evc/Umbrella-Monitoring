package app

import (
	"context"
	"net/http"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/directory"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
)

func (a *App) registerDirectory(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings/ldap", a.authed(a.admin(a.directorySettings)))
	mux.HandleFunc("PUT /api/settings/ldap", a.authed(a.admin(a.saveDirectory)))
	mux.HandleFunc("POST /api/settings/ldap/test", a.authed(a.admin(a.testDirectory)))
}

func (a *App) directorySettings(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, a.directory.View())
}

func (a *App) testDirectory(w http.ResponseWriter, r *http.Request) {
	var in directory.TestRequest
	if !httpx.Decode(w, r, &in) {
		return
	}
	rep, err := a.directory.Test(in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, rep)
}

func (a *App) saveDirectory(w http.ResponseWriter, r *http.Request) {
	var in directory.TestRequest
	if !httpx.Decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	out, err := a.directory.Save(ctx, current(r).user.Username, in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
