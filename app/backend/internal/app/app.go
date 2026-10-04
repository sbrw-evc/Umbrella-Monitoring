package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/config"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/directory"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/secrets"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

const (
	CookieName = "umbrella_session"
	CSRFHeader = "X-CSRF-Token"

	maxFailures = 10
	failWindow  = 15 * time.Minute
	lockout     = 5 * time.Minute
)

type Options struct {
	Version       string
	SecureCookies bool
	Web           http.Handler
}

type Deps struct {
	Config   config.File
	Vault    *secrets.Client
	Backend  *store.PGBackend
	Store    *store.Store
	Sessions *auth.Sessions
}

type App struct {
	opt Options
	Deps

	lmu      sync.Mutex
	failures map[string]*attempts
}

type attempts struct {
	count int
	first time.Time
	until time.Time
}

func New(opt Options, deps Deps) *App {
	if opt.Web == nil {
		opt.Web = http.NotFoundHandler()
	}
	return &App{opt: opt, Deps: deps, failures: map[string]*attempts{}}
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/meta", a.meta)
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("POST /api/auth/logout", a.authed(a.logout))
	mux.HandleFunc("GET /api/auth/me", a.authed(a.me))
	mux.HandleFunc("GET /api/system", a.authed(a.admin(a.system)))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, http.StatusNotFound, "not_found", nil)
	})
	mux.Handle("/", a.opt.Web)
	return httpx.Secure(mux)
}

func (a *App) meta(w http.ResponseWriter, r *http.Request) {
	var s model.Settings
	a.Store.Read(func(d *store.Data) { s = d.Settings })
	httpx.JSON(w, http.StatusOK, map[string]any{"mode": "ready", "setup_required": false, "version": a.opt.Version,
		"default_theme": s.DefaultTheme, "default_locale": s.DefaultLocale, "ldap_enabled": s.LDAP.Enabled})
}

type session struct {
	user model.User
	ss   *auth.Session
}

type ctxKey struct{}

func current(r *http.Request) session {
	s, _ := r.Context().Value(ctxKey{}).(session)
	return s
}

func (a *App) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil {
			httpx.Error(w, http.StatusUnauthorized, "unauthenticated", nil)
			return
		}
		ss := a.Sessions.Get(c.Value)
		if ss == nil {
			httpx.Error(w, http.StatusUnauthorized, "unauthenticated", nil)
			return
		}
		var u *model.User
		a.Store.Read(func(d *store.Data) {
			if x := d.Users[ss.UserID]; x != nil && !x.Disabled {
				c := *x
				u = &c
			}
		})
		if u == nil {
			a.Sessions.Delete(c.Value)
			httpx.Error(w, http.StatusUnauthorized, "unauthenticated", nil)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead &&
			subtle.ConstantTimeCompare([]byte(r.Header.Get(CSRFHeader)), []byte(ss.CSRF)) != 1 {
			httpx.Error(w, http.StatusForbidden, "csrf", nil)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, session{user: *u, ss: ss})))
	}
}

func (a *App) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if current(r).user.Role != model.RoleAdmin {
			httpx.Error(w, http.StatusForbidden, "forbidden", nil)
			return
		}
		next(w, r)
	}
}

type loginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userView struct {
	model.User
	CSRF string `json:"csrf,omitempty"`
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var in loginInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	name := strings.TrimSpace(in.Username)
	keys := []string{"user:" + strings.ToLower(name), "ip:" + clientIP(r)}
	if a.locked(keys) {
		httpx.Error(w, http.StatusTooManyRequests, "too_many_attempts", nil)
		return
	}
	u, err := a.verify(name, in.Password)
	if errors.Is(err, errUnavailable) {
		httpx.Error(w, http.StatusServiceUnavailable, "directory_unavailable", nil)
		return
	}
	if err != nil || u == nil {
		a.fail(keys)
		a.Store.Write(func(d *store.Data) {
			d.AddAudit(store.AuditEntry{Actor: name, Action: "auth.login_failed", Detail: clientIP(r)})
		})
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials", nil)
		return
	}
	a.reset(keys)
	ss := a.Sessions.Create(u.ID)
	now := time.Now().UTC()
	a.Store.Write(func(d *store.Data) {
		if x := d.Users[u.ID]; x != nil {
			x.LastLoginAt = &now
			c := *x
			u = &c
		}
		d.AddAudit(store.AuditEntry{At: now, Actor: u.Username, Action: "auth.login", Detail: u.Source + " " + clientIP(r)})
	})
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: ss.ID, Path: "/", HttpOnly: true, Secure: a.opt.SecureCookies,
		SameSite: http.SameSiteStrictMode, MaxAge: int(auth.MaxLifetime / time.Second)})
	httpx.JSON(w, http.StatusOK, userView{User: *u, CSRF: ss.CSRF})
}

var errUnavailable = errors.New("directory unavailable")

func (a *App) verify(name, password string) (*model.User, error) {
	if name == "" || password == "" {
		auth.CheckPassword("", password)
		return nil, directory.ErrInvalidCredentials
	}
	var u *model.User
	var ldap directory.Config
	a.Store.Read(func(d *store.Data) {
		if x := d.UserByName(name); x != nil {
			c := *x
			u = &c
		}
		ldap = d.Settings.LDAP
	})
	if u != nil && u.Source == model.SourceLocal {
		if !auth.CheckPassword(u.PasswordHash, password) || u.Disabled {
			return nil, directory.ErrInvalidCredentials
		}
		return u, nil
	}
	if !ldap.Enabled || (u != nil && u.Disabled) {
		auth.CheckPassword("", password)
		return nil, directory.ErrInvalidCredentials
	}
	bindPassword, err := a.Vault.Resolve(ldap.BindPasswordRef)
	if err != nil {
		slog.Error("directory: service account password unavailable", "err", err)
		return nil, errUnavailable
	}
	id, err := directory.Authenticate(ldap, bindPassword, name, password)
	switch {
	case errors.Is(err, directory.ErrInvalidCredentials), errors.Is(err, directory.ErrUserNotFound), errors.Is(err, directory.ErrAmbiguousUser):
		return nil, directory.ErrInvalidCredentials
	case err != nil:
		slog.Error("directory: sign-in failed", "err", err)
		return nil, errUnavailable
	}
	role := model.RoleUser
	if id.Admin {
		role = model.RoleAdmin
	}
	var out model.User
	a.Store.Write(func(d *store.Data) {
		x := d.UserByName(id.Username)
		if x == nil {
			x = &model.User{ID: d.NextID("USR"), Username: id.Username, Source: model.SourceLDAP, CreatedAt: time.Now().UTC()}
			d.Users[x.ID] = x
		}
		x.Name, x.Email, x.Role = id.Name, id.Email, role
		out = *x
	})
	if out.Source != model.SourceLDAP {
		return nil, directory.ErrInvalidCredentials
	}
	return &out, nil
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	s := current(r)
	a.Sessions.Delete(s.ss.ID)
	a.Store.Write(func(d *store.Data) {
		d.AddAudit(store.AuditEntry{Actor: s.user.Username, Action: "auth.logout"})
	})
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", HttpOnly: true, Secure: a.opt.SecureCookies,
		SameSite: http.SameSiteStrictMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) me(w http.ResponseWriter, r *http.Request) {
	s := current(r)
	httpx.JSON(w, http.StatusOK, userView{User: s.user, CSRF: s.ss.CSRF})
}

type ldapStatus struct {
	Enabled    bool   `json:"enabled"`
	Kind       string `json:"kind,omitempty"`
	URL        string `json:"url,omitempty"`
	TLS        string `json:"tls,omitempty"`
	BaseDN     string `json:"base_dn,omitempty"`
	AdminGroup string `json:"admin_group,omitempty"`
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
}

type pgStatus struct {
	Where   string              `json:"where"`
	OK      bool                `json:"ok"`
	Info    store.PGInfo        `json:"info"`
	Persist store.PersistStatus `json:"persist"`
	Error   string              `json:"error,omitempty"`
}

func (a *App) system(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var s model.Settings
	users := map[string]int{}
	a.Store.Read(func(d *store.Data) {
		s = d.Settings
		for _, u := range d.Users {
			users[u.Source]++
		}
	})

	var wg sync.WaitGroup
	var ob secrets.Status
	pg := pgStatus{Where: a.Backend.Where(), Persist: a.Store.PersistStatus()}
	ld := ldapStatus{Enabled: s.LDAP.Enabled}
	wg.Add(2)
	go func() {
		defer wg.Done()
		ob = a.Vault.Status(ctx)
	}()
	go func() {
		defer wg.Done()
		info, err := a.Backend.Info(ctx)
		pg.Info, pg.OK = info, err == nil
		if err != nil {
			pg.Error = err.Error()
		}
	}()
	if s.LDAP.Enabled {
		ld.Kind, ld.URL, ld.TLS, ld.BaseDN, ld.AdminGroup = s.LDAP.Kind, s.LDAP.URL, s.LDAP.TLSMode(), s.LDAP.BaseDN, s.LDAP.AdminGroupDN
		wg.Add(1)
		go func() {
			defer wg.Done()
			pw, err := a.Vault.Resolve(s.LDAP.BindPasswordRef)
			if err == nil {
				_, err = directory.Test(s.LDAP, pw, "", "")
			}
			ld.OK = err == nil
			if err != nil {
				ld.Error = err.Error()
			}
		}()
	}
	wg.Wait()
	httpx.JSON(w, http.StatusOK, map[string]any{
		"version":  a.opt.Version,
		"openbao":  ob,
		"postgres": pg,
		"ldap":     ld,
		"settings": map[string]any{"default_theme": s.DefaultTheme, "default_locale": s.DefaultLocale, "setup_at": s.SetupAt, "setup_by": s.SetupBy},
		"users":    users,
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (a *App) locked(keys []string) bool {
	a.lmu.Lock()
	defer a.lmu.Unlock()
	now := time.Now()
	for _, k := range keys {
		if at := a.failures[k]; at != nil && now.Before(at.until) {
			return true
		}
	}
	return false
}

func (a *App) fail(keys []string) {
	a.lmu.Lock()
	defer a.lmu.Unlock()
	now := time.Now()
	for _, k := range keys {
		at := a.failures[k]
		if at == nil || now.Sub(at.first) > failWindow {
			at = &attempts{first: now}
			a.failures[k] = at
		}
		at.count++
		if at.count >= maxFailures {
			at.until, at.count, at.first = now.Add(lockout), 0, now
		}
	}
	if len(a.failures) > 10000 {
		for k, at := range a.failures {
			if now.Sub(at.first) > failWindow && now.After(at.until) {
				delete(a.failures, k)
			}
		}
	}
}

func (a *App) reset(keys []string) {
	a.lmu.Lock()
	defer a.lmu.Unlock()
	delete(a.failures, keys[0])
}
