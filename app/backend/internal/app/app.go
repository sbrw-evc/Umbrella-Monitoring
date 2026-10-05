package app

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/config"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/credentials"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/ingest"
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
	accounts  *UsersService
	settings  *SettingsService
	status    *StatusService
	limiter   *Limiter
	policy    *PolicyService
	access    *AccessService
	directory *DirectoryService
	entra     *EntraService
	postgres  *PostgresService
	openbao   *OpenBaoService
	roles     *RolesService
	teams     *TeamsService
	services  *ServicesService

	creds      *CredentialsService
	connectors *ConnectorsService
	queue      *ingest.Queue
	ready      atomic.Bool
	rates      rates
}

func New(opt Options, deps Deps) *App {
	if opt.Web == nil {
		opt.Web = http.NotFoundHandler()
	}
	passwords := credentials.NewPasswords(deps.Vault)
	dir := ldapDirectory{}
	users := NewUserService(deps.Store, passwords)
	var vault Secrets
	if deps.Vault != nil {
		vault = deps.Vault
	}
	creds := NewCredentialsService(deps.Store, vault)
	var queue *ingest.Queue
	if deps.Backend != nil {
		queue = ingest.New(deps.Backend.Pool())
	}
	return &App{
		creds:      creds,
		connectors: NewConnectorsService(deps.Store, creds),
		queue:      queue,
		opt:        opt,
		deps:       deps,
		users:      users,
		accounts:   NewUsersService(deps.Store, passwords, deps.Vault, deps.Sessions),
		auth:       NewAuthService(deps.Store, passwords, deps.Vault, dir, users),
		settings:   NewSettingsService(deps.Store),
		status:     NewStatusService(deps.Store, deps.Vault, deps.Backend, dir, opt.Version),
		limiter:    NewLimiter(maxFailures, failWindow, lockout),
		policy:     NewPolicyService(deps.Store),
		access:     NewAccessService(deps.Store),
		directory:  NewDirectoryService(deps.Store, deps.Vault, dir, deps.Sessions),
		entra:      NewEntraService(deps.Store, deps.Vault, users, deps.Sessions),
		postgres:   NewPostgresService(deps.Store, deps.Backend, deps.Vault, deps.Runtime, deps.Config),
		openbao:    NewOpenBaoService(deps.Store, deps.Vault, deps.Runtime, deps.Config),
		roles:      NewRolesService(deps.Store),
		teams:      NewTeamsService(deps.Store),
		services:   NewServicesService(deps.Store),
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
	mux.HandleFunc("PUT /api/settings", a.authed(a.can("status:defaults", a.updateSettings)))
	mux.HandleFunc("GET /api/system", a.authed(a.can("status:view", a.system)))
	for _, register := range []func(*http.ServeMux){a.registerRefs, a.registerPostgres, a.registerOpenBao, a.registerDirectory, a.registerEntra, a.registerPolicy, a.registerUsers, a.registerRoles, a.registerTeams, a.registerServices, a.registerConnectors} {
		register(mux)
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeError(w, ErrNotFound) })
	mux.Handle("/", a.opt.Web)
	return httpx.Secure(mux)
}

// Run prepares the ingest tables and processes received requests until ctx ends. Without
// PostgreSQL (tests) it returns at once and the intake answers 503.
func (a *App) Run(ctx context.Context) {
	if a.queue == nil {
		return
	}
	for {
		err := ingest.EnsureSchema(ctx, a.deps.Backend.Pool())
		if err == nil {
			break
		}
		slog.Error("ingest tables are not ready", "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
	a.ready.Store(true)
	a.queue.Run(ctx, 2, a.process)
}

func (a *App) ingestReady() bool { return a.queue != nil && a.ready.Load() }

// IngestReady reports whether the intake tables exist and the workers run.
func (a *App) IngestReady() bool { return a.ingestReady() }

type metaView struct {
	defaultsView
	Mode          string `json:"mode"`
	SetupRequired bool   `json:"setup_required"`
	Version       string `json:"version"`
	LDAPEnabled   bool   `json:"ldap_enabled"`
	EntraEnabled  bool   `json:"entra_enabled"`
}

func (a *App) meta(w http.ResponseWriter, r *http.Request) {
	s := a.settings.Get()
	httpx.JSON(w, http.StatusOK, metaView{defaultsView: defaultsOf(s), Mode: "ready", Version: a.opt.Version, LDAPEnabled: s.LDAP.Enabled, EntraEnabled: s.Entra.Enabled})
}
