package setup

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
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
	TokenHeader = "X-Setup-Token"

	maxFailures = 5
	lockout     = time.Minute

	secretPostgres = "postgres"
	secretLDAP     = "ldap"
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@-]{1,63}$`)

type Options struct {
	DataDir string
	Token   string
	Version string
	Web     http.Handler
}

type Result struct {
	Config  config.File
	Vault   *secrets.Client
	Backend *store.PGBackend
	Store   *store.Store
}

type Module struct {
	opt  Options
	done func(Result)

	mu       sync.Mutex
	finished bool

	lmu      sync.Mutex
	failures int
	until    time.Time
}

func New(opt Options, done func(Result)) *Module {
	if opt.Web == nil {
		opt.Web = http.NotFoundHandler()
	}
	return &Module{opt: opt, done: done}
}

func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "setup"})
	})
	mux.HandleFunc("GET /api/meta", m.meta)
	mux.HandleFunc("GET /api/setup/status", m.meta)
	mux.HandleFunc("POST /api/setup/token", m.guard(func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("POST /api/setup/openbao/test", m.guard(m.testOpenBao))
	mux.HandleFunc("POST /api/setup/postgres/test", m.guard(m.testPostgres))
	mux.HandleFunc("POST /api/setup/ldap/test", m.guard(m.testLDAP))
	mux.HandleFunc("POST /api/setup/complete", m.guard(m.complete))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, http.StatusServiceUnavailable, "setup_required", nil)
	})
	mux.Handle("/", m.opt.Web)
	return httpx.Secure(mux)
}

func (m *Module) meta(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{"mode": "setup", "setup_required": true, "version": m.opt.Version,
		"default_theme": model.ThemeLight, "default_locale": model.LocaleEN})
}

func (m *Module) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		finished := m.finished
		m.mu.Unlock()
		if finished {
			httpx.Error(w, http.StatusConflict, "setup_completed", nil)
			return
		}
		m.lmu.Lock()
		locked := time.Now().Before(m.until)
		m.lmu.Unlock()
		if locked {
			httpx.Error(w, http.StatusTooManyRequests, "too_many_attempts", nil)
			return
		}
		given := r.Header.Get(TokenHeader)
		if subtle.ConstantTimeCompare([]byte(given), []byte(m.opt.Token)) != 1 {
			m.lmu.Lock()
			m.failures++
			if m.failures >= maxFailures {
				m.failures, m.until = 0, time.Now().Add(lockout)
			}
			m.lmu.Unlock()
			slog.Warn("setup: wrong setup code", "remote", r.RemoteAddr)
			httpx.Error(w, http.StatusUnauthorized, "invalid_setup_token", nil)
			return
		}
		m.lmu.Lock()
		m.failures = 0
		m.lmu.Unlock()
		next(w, r)
	}
}

type openbaoReply struct {
	OK         bool           `json:"ok"`
	Status     secrets.Status `json:"status"`
	WriteOK    bool           `json:"write_ok"`
	WriteError string         `json:"write_error,omitempty"`
	Error      string         `json:"error,omitempty"`
}

func (m *Module) testOpenBao(w http.ResponseWriter, r *http.Request) {
	var in config.OpenBao
	if !httpx.Decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	_, rep := checkOpenBao(ctx, in)
	httpx.JSON(w, http.StatusOK, rep)
}

func checkOpenBao(ctx context.Context, in config.OpenBao) (*secrets.Client, openbaoReply) {
	var rep openbaoReply
	in, err := in.Normalize()
	if err != nil {
		rep.Error = err.Error()
		return nil, rep
	}
	c, err := in.Client()
	if err != nil {
		rep.Error = err.Error()
		return nil, rep
	}
	rep.Status = c.Status(ctx)
	if !rep.Status.TokenOK || !rep.Status.MountOK {
		rep.Error = rep.Status.Error
		return nil, rep
	}
	probe := "setup-probe-" + auth.RandomToken("", 6)
	if err := c.Put(ctx, probe, map[string]string{"probe": time.Now().UTC().Format(time.RFC3339)}); err != nil {
		rep.WriteError = err.Error()
		rep.Error = "cannot write secrets: " + err.Error()
		return nil, rep
	}
	if err := c.Delete(ctx, probe); err != nil {
		rep.WriteError = err.Error()
		rep.Error = "cannot delete secrets: " + err.Error()
		return nil, rep
	}
	rep.WriteOK, rep.OK = true, true
	return c, rep
}

type pgInput struct {
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Database      string `json:"database"`
	User          string `json:"user"`
	Password      string `json:"password"`
	SSLMode       string `json:"sslmode"`
	ReuseExisting bool   `json:"reuse_existing"`
}

func (p pgInput) config() store.PGConfig {
	return store.PGConfig{Host: strings.TrimSpace(p.Host), Port: p.Port, Database: strings.TrimSpace(p.Database),
		User: strings.TrimSpace(p.User), SSLMode: p.SSLMode, Password: p.Password}
}

type pgReply struct {
	OK    bool          `json:"ok"`
	Probe store.PGProbe `json:"probe"`
	Error string        `json:"error,omitempty"`
}

func (m *Module) testPostgres(w http.ResponseWriter, r *http.Request) {
	var in pgInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	probe, err := store.ProbePostgres(r.Context(), in.config())
	rep := pgReply{OK: err == nil, Probe: probe}
	if err != nil {
		rep.Error = err.Error()
	}
	httpx.JSON(w, http.StatusOK, rep)
}

type ldapInput struct {
	Config       directory.Config `json:"config"`
	BindPassword string           `json:"bind_password"`
	TestUsername string           `json:"test_username,omitempty"`
	TestPassword string           `json:"test_password,omitempty"`
}

type ldapReply struct {
	OK    bool            `json:"ok"`
	Probe directory.Probe `json:"probe"`
	Error string          `json:"error,omitempty"`
}

func (m *Module) testLDAP(w http.ResponseWriter, r *http.Request) {
	var in ldapInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	probe, err := directory.Test(in.Config, in.BindPassword, in.TestUsername, in.TestPassword)
	rep := ldapReply{OK: err == nil, Probe: probe}
	if err != nil {
		rep.Error = err.Error()
	}
	httpx.JSON(w, http.StatusOK, rep)
}

type adminInput struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type completeInput struct {
	Locale   string         `json:"locale"`
	Theme    string         `json:"theme"`
	OpenBao  config.OpenBao `json:"openbao"`
	Postgres pgInput        `json:"postgres"`
	LDAP     ldapInput      `json:"ldap"`
	Admin    adminInput     `json:"admin"`
}

type stepError struct {
	status int
	code   string
	err    error
}

func (e *stepError) Error() string {
	if e.err == nil {
		return e.code
	}
	return e.code + ": " + e.err.Error()
}

func fail(status int, code string, err error) *stepError {
	return &stepError{status: status, code: code, err: err}
}

func (m *Module) complete(w http.ResponseWriter, r *http.Request) {
	var in completeInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.finished {
		httpx.Error(w, http.StatusConflict, "setup_completed", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	res, err := m.apply(ctx, in)
	if err != nil {
		var se *stepError
		if !errors.As(err, &se) {
			se = fail(http.StatusInternalServerError, "internal", err)
		}
		slog.Warn("setup: not completed", "step", se.code, "err", se.err)
		httpx.Error(w, se.status, se.code, se.err)
		return
	}
	m.finished = true
	_ = os.Remove(filepath.Join(m.opt.DataDir, config.SetupTokenFile))
	slog.Info("setup completed", "admin", in.Admin.Username, "storage", res.Backend.Where(), "ldap", in.LDAP.Config.Enabled)
	if m.done != nil {
		m.done(res)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (m *Module) apply(ctx context.Context, in completeInput) (Result, error) {
	var res Result
	if !model.ValidLocale(in.Locale) {
		return res, fail(http.StatusBadRequest, "invalid_locale", nil)
	}
	if !model.ValidTheme(in.Theme) {
		return res, fail(http.StatusBadRequest, "invalid_theme", nil)
	}
	admin, err := checkAdmin(in.Admin)
	if err != nil {
		return res, err
	}

	ob, err := in.OpenBao.Normalize()
	if err != nil {
		return res, fail(http.StatusBadRequest, "openbao_invalid", err)
	}
	vault, rep := checkOpenBao(ctx, ob)
	if !rep.OK {
		return res, fail(http.StatusBadRequest, "openbao_unavailable", errors.New(rep.Error))
	}

	ldapCfg := directory.Config{}
	if in.LDAP.Config.Enabled {
		ldapCfg, err = in.LDAP.Config.Normalize()
		if err != nil {
			return res, fail(http.StatusBadRequest, "ldap_invalid", err)
		}
		if _, err := directory.Test(ldapCfg, in.LDAP.BindPassword, "", ""); err != nil {
			return res, fail(http.StatusBadRequest, "ldap_unavailable", err)
		}
		ldapCfg.Enabled = true
	}

	pg, err := in.Postgres.config().Normalize()
	if err != nil {
		return res, fail(http.StatusBadRequest, "postgres_invalid", err)
	}
	probe, err := store.ProbePostgres(ctx, pg)
	if err != nil {
		return res, fail(http.StatusBadRequest, "postgres_unavailable", err)
	}
	if probe.HasState && !in.Postgres.ReuseExisting {
		return res, fail(http.StatusConflict, "postgres_has_state", nil)
	}
	if !probe.CanCreate && !probe.HasState {
		return res, fail(http.StatusBadRequest, "postgres_no_create", nil)
	}

	pgRef := ""
	if pg.Password != "" {
		if pgRef, err = vault.PutRef(ctx, secretPostgres, "password", pg.Password); err != nil {
			return res, fail(http.StatusBadGateway, "openbao_write_failed", err)
		}
	} else if err := vault.Delete(ctx, secretPostgres); err != nil {
		return res, fail(http.StatusBadGateway, "openbao_write_failed", err)
	}
	if ldapCfg.Enabled {
		ref, err := vault.PutRef(ctx, secretLDAP, "bind_password", in.LDAP.BindPassword)
		if err != nil {
			return res, fail(http.StatusBadGateway, "openbao_write_failed", err)
		}
		ldapCfg.BindPasswordRef = ref
	} else if err := vault.Delete(ctx, secretLDAP); err != nil && !errors.Is(err, secrets.ErrNotFound) {
		slog.Warn("setup: old directory secret not removed", "err", err)
	}

	backend, err := store.OpenPostgres(ctx, pg)
	if err != nil {
		return res, fail(http.StatusBadGateway, "postgres_unavailable", err)
	}
	st := store.New()
	if _, err := st.Attach(ctx, backend); err != nil {
		backend.Close()
		return res, fail(http.StatusConflict, "postgres_state_unreadable", err)
	}
	now := time.Now().UTC()
	st.Write(func(d *store.Data) {
		d.Settings.DefaultTheme = in.Theme
		d.Settings.DefaultLocale = in.Locale
		d.Settings.LDAP = ldapCfg
		d.Settings.SetupAt = now
		d.Settings.SetupBy = admin.Username
		u := d.UserByName(admin.Username)
		if u == nil {
			u = &model.User{ID: d.NextID("USR"), CreatedAt: now}
			d.Users[u.ID] = u
		}
		u.Username, u.Name, u.Email = admin.Username, admin.Name, admin.Email
		u.Source, u.Role, u.Disabled, u.PasswordHash = model.SourceLocal, model.RoleAdmin, false, admin.PasswordHash
		d.AddAudit(store.AuditEntry{At: now, Actor: admin.Username, Action: "setup.complete",
			Detail: "storage " + pg.Where() + ", directory " + onOff(ldapCfg.Enabled)})
	})
	if err := st.Flush(); err != nil {
		backend.Close()
		return res, fail(http.StatusBadGateway, "postgres_write_failed", err)
	}

	stored := pg
	stored.Password = ""
	file := config.File{OpenBao: ob, Postgres: stored, PostgresPasswordRef: pgRef, CompletedAt: &now}
	if err := config.Write(m.opt.DataDir, file); err != nil {
		backend.Close()
		return res, fail(http.StatusInternalServerError, "config_write_failed", err)
	}
	return Result{Config: file, Vault: vault, Backend: backend, Store: st}, nil
}

type admin struct {
	Username, Name, Email, PasswordHash string
}

func checkAdmin(in adminInput) (admin, error) {
	a := admin{Username: strings.TrimSpace(in.Username), Name: strings.TrimSpace(in.Name), Email: strings.TrimSpace(in.Email)}
	if !usernameRe.MatchString(a.Username) {
		return a, fail(http.StatusBadRequest, "invalid_username", nil)
	}
	if a.Name == "" {
		a.Name = a.Username
	}
	if len(a.Name) > 200 {
		return a, fail(http.StatusBadRequest, "invalid_name", nil)
	}
	if a.Email != "" {
		if addr, err := mail.ParseAddress(a.Email); err != nil || addr.Address != a.Email {
			return a, fail(http.StatusBadRequest, "invalid_email", nil)
		}
	}
	if err := auth.CheckPolicy(in.Password, a.Username); err != nil {
		return a, fail(http.StatusBadRequest, "weak_password", err)
	}
	h, err := auth.HashPassword(in.Password)
	if err != nil {
		return a, err
	}
	a.PasswordHash = h
	return a, nil
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}
