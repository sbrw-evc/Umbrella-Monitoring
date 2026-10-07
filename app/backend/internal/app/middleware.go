package app

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/access"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type session struct {
	user model.User
	ss   *auth.Session
}

type ctxKey struct{}

func current(r *http.Request) session {
	s, _ := r.Context().Value(ctxKey{}).(session)
	return s
}

var allowedWhenExpired = map[string]bool{
	"GET /api/auth/me":           true,
	"POST /api/auth/logout":      true,
	"PUT /api/auth/me/password":  true,
	"GET /api/users/{id}/avatar": true,
}

func (a *App) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil {
			writeError(w, ErrUnauthenticated)
			return
		}
		ss := a.deps.Sessions.Get(c.Value)
		if ss == nil {
			writeError(w, ErrUnauthenticated)
			return
		}
		var u *model.User
		a.deps.Store.Read(func(d *store.Data) {
			if x := d.Users[ss.UserID]; x != nil && !x.Disabled {
				c := *x
				u = &c
			}
		})
		if u == nil {
			a.deps.Sessions.Delete(c.Value)
			writeError(w, ErrUnauthenticated)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead &&
			subtle.ConstantTimeCompare([]byte(r.Header.Get(CSRFHeader)), []byte(ss.CSRF)) != 1 {
			writeError(w, invalidCSRF)
			return
		}
		if !allowedWhenExpired[r.Pattern] && a.policy.Age(*u).Expired {
			writeError(w, ErrPasswordExpired)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, session{user: *u, ss: ss})))
	}
}

// can guards next with perm. An unknown permission is a programming error: it panics when the
// route is registered, so a typo never ships as a route nobody can open.
func (a *App) can(perm string, next http.HandlerFunc) http.HandlerFunc {
	if !access.Valid(perm) {
		panic("unknown permission " + perm)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.access.Permissions(current(r).user).Has(perm) {
			writeError(w, forbidden)
			return
		}
		next(w, r)
	}
}

// clientIP is the address of the client for security decisions and the audit: the peer of the
// connection, or the forwarded address when the peer is a trusted reverse proxy
// (UMBRELLA_TRUSTED_PROXIES or the interface).
func (a *App) clientIP(r *http.Request) string {
	if ip := a.trustedProxies().ClientIP(r); ip.IsValid() {
		return ip.String()
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
