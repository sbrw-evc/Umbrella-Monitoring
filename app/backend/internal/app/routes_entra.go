package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/entra"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
)

const entraCookie = "umbrella_entra"

func (a *App) registerEntra(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings/entra", a.authed(a.can("settings.ldap:view", a.entraSettings)))
	mux.HandleFunc("PUT /api/settings/entra", a.authed(a.can("settings.ldap:edit", a.saveEntra)))
	mux.HandleFunc("POST /api/settings/entra/test", a.authed(a.can("settings.ldap:test", a.testEntra)))
	mux.HandleFunc("GET /api/auth/entra/start", a.entraStart)
	mux.HandleFunc("GET "+entra.CallbackPath, a.entraCallback)
}

func (a *App) entraSettings(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, a.entra.View())
}

func (a *App) testEntra(w http.ResponseWriter, r *http.Request) {
	var in entra.TestRequest
	if !httpx.Decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	rep, err := a.entra.Test(ctx, in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, rep)
}

func (a *App) saveEntra(w http.ResponseWriter, r *http.Request) {
	var in entra.TestRequest
	if !httpx.Decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	out, err := a.entra.Save(ctx, current(r).user.Username, in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// entraStart sends the browser to the Microsoft sign-in page. The state also goes into a
// cookie, so only the browser that started the sign-in can finish it.
func (a *App) entraStart(w http.ResponseWriter, r *http.Request) {
	ip := a.clientIP(r)
	if a.entraRate.Locked(ip) {
		a.signInFailed(w, r, "too_many_attempts")
		return
	}
	a.entraRate.Fail(ip)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	target, state, err := a.entra.Start(ctx)
	switch {
	case errors.Is(err, ErrNotFound):
		a.signInFailed(w, r, "entra_off")
		return
	case errors.Is(err, ErrTooManyAttempts):
		a.signInFailed(w, r, "too_many_attempts")
		return
	case err != nil:
		slog.Error("entra: sign-in cannot start", "err", err)
		a.signInFailed(w, r, "entra_unavailable")
		return
	}
	a.setEntraCookie(w, state, int(entraPendingTTL/time.Second))
	http.Redirect(w, r, target, http.StatusFound)
}

func (a *App) entraCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		slog.Warn("entra: sign-in refused by Microsoft", "error", e, "description", q.Get("error_description"))
		a.signInFailed(w, r, "entra_denied")
		return
	}
	state := q.Get("state")
	c, err := r.Cookie(entraCookie)
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(state)) != 1 {
		a.signInFailed(w, r, "entra_state")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	u, err := a.entra.Complete(ctx, state, q.Get("code"))
	switch {
	case errors.Is(err, errEntraState):
		a.signInFailed(w, r, "entra_state")
		return
	case errors.Is(err, errEntraNotAllowed):
		a.auth.Failed("entra", a.clientIP(r))
		a.signInFailed(w, r, "entra_not_allowed")
		return
	case errors.Is(err, ErrInvalidCredentials):
		a.auth.Failed("entra", a.clientIP(r))
		a.signInFailed(w, r, "entra_account")
		return
	case err != nil:
		slog.Error("entra: sign-in failed", "err", err)
		a.signInFailed(w, r, "entra_unavailable")
		return
	}
	a.setEntraCookie(w, "", -1)
	ss := a.deps.Sessions.Create(u.ID)
	a.auth.Signed(u.ID, a.clientIP(r))
	a.setCookie(w, ss.ID, int(auth.MaxLifetime/time.Second))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) signInFailed(w http.ResponseWriter, r *http.Request, code string) {
	a.setEntraCookie(w, "", -1)
	http.Redirect(w, r, "/?signin_error="+code, http.StatusSeeOther)
}

func (a *App) setEntraCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: entraCookie, Value: value, Path: "/api/auth/entra/", HttpOnly: true, Secure: a.opt.SecureCookies,
		SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}
