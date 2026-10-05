package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

var (
	builtInThemes  = []string{"light", "dark"}
	builtInLocales = []string{"ru", "en"}
)

const SetupTokenFile = "setup-token"

func (s *Server) setupRequired() bool {
	done := false
	s.st.Read(func(d *store.Data) { done = d.Settings.SetupCompleted })
	return !done
}

func hasPeople(d *store.Data) bool {
	for _, u := range d.Users {
		if !u.Service {
			return true
		}
	}
	return false
}

func (s *Server) uiDefaults() (string, string) {
	theme, locale := "", ""
	s.st.Read(func(d *store.Data) { theme, locale = d.Settings.DefaultTheme, d.Settings.DefaultLocale })
	return theme, locale
}

func (s *Server) getUIDefaults(w http.ResponseWriter, _ *http.Request) {
	theme, locale := s.uiDefaults()
	writeJSON(w, 200, map[string]string{"theme": theme, "locale": locale})
}

func (s *Server) storageView() map[string]any {
	st := s.st.PersistStatus()
	cfg, _ := store.ReadStorageConfig(s.cfg.DataDir)
	out := map[string]any{"kind": st.Kind, "where": st.Where, "enabled": st.Enabled, "data_dir": s.cfg.DataDir}
	if cfg.Kind == "postgres" {
		pg := cfg.Postgres
		out["postgres"] = map[string]any{"host": pg.Host, "port": pg.Port, "database": pg.Database, "user": pg.User, "sslmode": pg.SSLMode}
		out["password_set"] = cfg.PasswordRef != ""
	}
	return out
}

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	people := false
	s.st.Read(func(d *store.Data) { people = hasPeople(d) })
	theme, locale := s.uiDefaults()
	resp := map[string]any{
		"required":       s.setupRequired(),
		"admin_exists":   people,
		"token_required": s.setupRequired() && !people,
		"openbao":        s.vault.Enabled(),
		"themes":         builtInThemes,
		"locales":        builtInLocales,
		"defaults":       map[string]string{"theme": theme, "locale": locale},
		"version":        s.cfg.Version,
	}
	if s.setupRequired() {
		resp["storage"] = s.storageView()
	}
	writeJSON(w, 200, resp)
}

func (s *Server) setupGuard(w http.ResponseWriter, r *http.Request, afterSetup bool) (string, bool) {
	if !afterSetup && !s.setupRequired() {
		writeErr(w, 409, errors.New("первоначальная настройка уже выполнена"))
		return "", false
	}
	people := false
	s.st.Read(func(d *store.Data) { people = hasPeople(d) })
	if !people && !afterSetup {
		tok := strings.TrimSpace(r.Header.Get("X-Umbrella-Setup-Token"))
		want := s.setupToken()
		if want == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(want)) != 1 {
			writeJSON(w, 401, map[string]string{"error": "неверный код установки: он записан в журнале сервера и в файле setup-token каталога данных", "code": "setup_token"})
			return "", false
		}
		return "setup", true
	}
	p := s.authenticate(r)
	if p == nil {
		writeJSON(w, 401, map[string]string{"error": "войдите как администратор", "code": "unauthorized"})
		return "", false
	}
	if p.Via == "session" && (p.CSRF == "" || r.Header.Get("X-Umbrella-CSRF") != p.CSRF) {
		writeJSON(w, 403, map[string]string{"error": "запрос без CSRF-токена", "code": "csrf"})
		return "", false
	}
	if !p.Can(model.PermUsersAdmin) {
		writeJSON(w, 403, map[string]string{"error": "первоначальную настройку выполняет администратор", "code": "forbidden"})
		return "", false
	}
	return p.Name(), true
}

type storageInput struct {
	Kind     string `json:"kind"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	User     string `json:"user"`
	SSLMode  string `json:"sslmode"`
	Password string `json:"password"`
	Adopt    bool   `json:"adopt"`
}

func (in storageInput) pg() store.PGConfig {
	return store.PGConfig{Host: strings.TrimSpace(in.Host), Port: in.Port, Database: strings.TrimSpace(in.Database),
		User: strings.TrimSpace(in.User), SSLMode: in.SSLMode, Password: in.Password}
}

func (s *Server) pgPassword(in storageInput) (string, error) {
	if in.Password != "" {
		return in.Password, nil
	}
	cfg, err := store.ReadStorageConfig(s.cfg.DataDir)
	if err != nil || cfg.PasswordRef == "" {
		return "", nil
	}
	return s.vault.Resolve(cfg.PasswordRef)
}

func (s *Server) setupTestDB(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.setupGuard(w, r, !s.setupRequired()); !ok {
		return
	}
	var in storageInput
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err)
		return
	}
	pg := in.pg()
	var err error
	if pg.Password, err = s.pgPassword(in); err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	b, err := store.OpenPostgres(ctx, pg)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	defer b.Close()
	info, err := b.Info(ctx)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "message": "PostgreSQL " + info.Version, "version": info.Version,
		"has_state": info.HasState, "saved_at": info.SavedAt})
}

func (s *Server) applyStorage(ctx context.Context, in storageInput) (bool, error) {
	switch in.Kind {
	case "", "keep":
		return false, nil
	case "file":
		if s.cfg.DataDir == "" {
			return false, errors.New("каталог данных не задан (UMBRELLA_DATA_DIR)")
		}
		if _, err := s.st.Switch(ctx, store.FileBackend{Dir: s.cfg.DataDir}, false); err != nil {
			return false, err
		}
		return false, store.WriteStorageConfig(s.cfg.DataDir, store.StorageConfig{Kind: "file"})
	case "postgres":
		if s.cfg.DataDir == "" {
			return false, errors.New("каталог данных не задан (UMBRELLA_DATA_DIR): подключение к БД негде запомнить")
		}
		if !s.vault.Enabled() {
			return false, errors.New("пароль БД хранится в OpenBao, а OpenBao не подключён")
		}
		pg := in.pg()
		ref := ""
		if in.Password != "" {
			var err error
			if ref, err = s.vault.PutRef(ctx, "database", "password", in.Password); err != nil {
				return false, err
			}
		} else if cfg, err := store.ReadStorageConfig(s.cfg.DataDir); err == nil && cfg.PasswordRef != "" {
			ref = cfg.PasswordRef
			if pg.Password, err = s.vault.Resolve(ref); err != nil {
				return false, err
			}
		}
		b, err := store.OpenPostgres(ctx, pg)
		if err != nil {
			return false, err
		}
		adopted, err := s.st.Switch(ctx, b, in.Adopt)
		if err != nil {
			b.Close()
			return false, err
		}
		norm, _ := pg.Normalize()
		norm.Password = ""
		return adopted, store.WriteStorageConfig(s.cfg.DataDir, store.StorageConfig{Kind: "postgres", Postgres: norm, PasswordRef: ref})
	}
	return false, errors.New("хранилище: file или postgres")
}

func validDefaults(theme, locale string) error {
	if theme != "" && !contains(builtInThemes, theme) {
		return errors.New("тема по умолчанию: light или dark")
	}
	if locale != "" && !contains(builtInLocales, locale) {
		return errors.New("язык по умолчанию: ru или en")
	}
	return nil
}

func (s *Server) setupComplete(w http.ResponseWriter, r *http.Request) {
	who, ok := s.setupGuard(w, r, false)
	if !ok {
		return
	}
	var in struct {
		Theme   string       `json:"theme"`
		Locale  string       `json:"locale"`
		Storage storageInput `json:"storage"`
		Admin   *struct {
			Username string `json:"username"`
			Name     string `json:"name"`
			Email    string `json:"email"`
			Password string `json:"password"`
		} `json:"admin"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err)
		return
	}
	if in.Theme == "" {
		in.Theme = "light"
	}
	if in.Locale == "" {
		in.Locale = "ru"
	}
	if err := validDefaults(in.Theme, in.Locale); err != nil {
		writeErr(w, 400, err)
		return
	}
	people := false
	s.st.Read(func(d *store.Data) { people = hasPeople(d) })
	if !people {
		if in.Admin == nil || strings.TrimSpace(in.Admin.Username) == "" {
			writeErr(w, 400, errors.New("создайте локального администратора"))
			return
		}
		if err := auth.CheckPolicy(in.Admin.Password, in.Admin.Username); err != nil {
			writeErr(w, 400, err)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	adopted, err := s.applyStorage(ctx, in.Storage)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	now := time.Now()
	created := ""
	kind := s.st.PersistStatus().Kind
	var cerr error
	s.st.Write(func(d *store.Data) {
		if in.Admin != nil && !hasPeople(d) {
			h, err := auth.HashPassword(in.Admin.Password)
			if err != nil {
				cerr = err
				return
			}
			name := strings.TrimSpace(in.Admin.Name)
			if name == "" {
				name = "Администратор"
			}
			u := &model.User{ID: d.NextID("USR"), Username: strings.TrimSpace(in.Admin.Username), Name: name, Email: strings.TrimSpace(in.Admin.Email),
				Roles: []string{auth.RoleAdmin}, BusinessServices: []string{}, CreatedAt: now, PasswordHash: h, PasswordChangedAt: &now}
			d.Users[u.ID] = u
			created = u.Username
			d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: who, Action: "setup.admin", Object: u.Username})
		}
		d.Settings.DefaultTheme, d.Settings.DefaultLocale = in.Theme, in.Locale
		d.Settings.SetupCompleted, d.Settings.SetupAt, d.Settings.SetupBy = true, &now, who
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: who, Action: "setup.complete", Object: kind})
	})
	if cerr != nil {
		writeErr(w, 500, cerr)
		return
	}
	if err := s.st.Flush(); err != nil {
		writeErr(w, 500, err)
		return
	}
	if s.cfg.DataDir != "" {
		_ = os.Remove(filepath.Join(s.cfg.DataDir, SetupTokenFile))
	}
	s.setSetupToken("")
	writeJSON(w, 200, map[string]any{"ok": true, "adopted": adopted, "admin_created": created, "storage": s.storageView()})
}

func (s *Server) getStorage(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, s.storageView())
}

func (s *Server) putStorage(w http.ResponseWriter, r *http.Request) {
	var in storageInput
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	adopted, err := s.applyStorage(ctx, in)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	s.audit(r, "settings.storage "+in.Kind, "storage")
	out := s.storageView()
	out["adopted"] = adopted
	writeJSON(w, 200, out)
}

func (s *Server) setupToken() string {
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	return s.cfg.SetupToken
}

func (s *Server) setSetupToken(v string) {
	s.setupMu.Lock()
	s.cfg.SetupToken = v
	s.setupMu.Unlock()
}
