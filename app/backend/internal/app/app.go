package app

import (
	"net/http"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/config"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/credentials"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
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
	Runtime  Runtime
}

type App struct {
	opt       Options
	deps      Deps
	auth      *AuthService
	users     *UserService
	settings  *SettingsService
	status    *StatusService
	limiter   *Limiter
	policy    *PolicyService
	directory *DirectoryService
	postgres  *PostgresService
	openbao   *OpenBaoService
}

func New(opt Options, deps Deps) *App {
	if opt.Web == nil {
		opt.Web = http.NotFoundHandler()
	}
	passwords := credentials.NewPasswords(deps.Vault)
	dir := ldapDirectory{}
	users := NewUserService(deps.Store, passwords)
	return &App{
		opt:       opt,
		deps:      deps,
		users:     users,
		auth:      NewAuthService(deps.Store, passwords, deps.Vault, dir, users),
		settings:  NewSettingsService(deps.Store),
		status:    NewStatusService(deps.Store, deps.Vault, deps.Backend, dir, opt.Version),
		limiter:   NewLimiter(maxFailures, failWindow, lockout),
		policy:    NewPolicyService(deps.Store),
		directory: NewDirectoryService(deps.Store, deps.Vault, dir, deps.Sessions),
		postgres:  NewPostgresService(deps.Store, deps.Backend, deps.Vault, deps.Runtime, deps.Config),
		openbao:   NewOpenBaoService(deps.Store, deps.Vault, deps.Runtime, deps.Config),
	}
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
	mux.HandleFunc("PUT /api/auth/me/preferences", a.authed(a.preferences))
	mux.HandleFunc("PUT /api/auth/me/profile", a.authed(a.updateProfile))
	mux.HandleFunc("PUT /api/auth/me/password", a.authed(a.changePassword))
	mux.HandleFunc("PUT /api/auth/me/avatar", a.authed(a.uploadAvatar))
	mux.HandleFunc("DELETE /api/auth/me/avatar", a.authed(a.deleteAvatar))
	mux.HandleFunc("GET /api/users/{id}/avatar", a.authed(a.avatar))
	mux.HandleFunc("PUT /api/settings", a.authed(a.admin(a.updateSettings)))
	mux.HandleFunc("GET /api/system", a.authed(a.admin(a.system)))
	for _, register := range []func(*http.ServeMux){a.registerPostgres, a.registerOpenBao, a.registerDirectory, a.registerPolicy} {
		register(mux)
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeError(w, ErrNotFound) })
	mux.Handle("/", a.opt.Web)
	return httpx.Secure(mux)
}

type metaView struct {
	defaultsView
	Mode          string `json:"mode"`
	SetupRequired bool   `json:"setup_required"`
	Version       string `json:"version"`
	LDAPEnabled   bool   `json:"ldap_enabled"`
}

func (a *App) meta(w http.ResponseWriter, r *http.Request) {
	s := a.settings.Get()
	httpx.JSON(w, http.StatusOK, metaView{defaultsView: defaultsOf(s), Mode: "ready", Version: a.opt.Version, LDAPEnabled: s.LDAP.Enabled})
}
