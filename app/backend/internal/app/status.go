package app

import (
	"context"
	"sync"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/secrets"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type StatusService struct {
	st      *store.Store
	secrets Secrets
	db      Database
	dir     Directory
	version string
}

func NewStatusService(st *store.Store, secrets Secrets, db Database, dir Directory, version string) *StatusService {
	return &StatusService{st: st, secrets: secrets, db: db, dir: dir, version: version}
}

type LDAPStatus struct {
	Enabled    bool   `json:"enabled"`
	Kind       string `json:"kind,omitempty"`
	URL        string `json:"url,omitempty"`
	TLS        string `json:"tls,omitempty"`
	BaseDN     string `json:"base_dn,omitempty"`
	AdminGroup string `json:"admin_group,omitempty"`
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
}

type PostgresStatus struct {
	Where   string              `json:"where"`
	OK      bool                `json:"ok"`
	Info    store.PGInfo        `json:"info"`
	Persist store.PersistStatus `json:"persist"`
	Error   string              `json:"error,omitempty"`
}

type SettingsStatus struct {
	defaultsView
	SetupAt time.Time `json:"setup_at"`
	SetupBy string    `json:"setup_by"`
}

type SystemStatus struct {
	Version  string         `json:"version"`
	OpenBao  secrets.Status `json:"openbao"`
	Postgres PostgresStatus `json:"postgres"`
	LDAP     LDAPStatus     `json:"ldap"`
	Settings SettingsStatus `json:"settings"`
	Users    map[string]int `json:"users"`
}

func (s *StatusService) Collect(ctx context.Context) SystemStatus {
	var settings model.Settings
	out := SystemStatus{Version: s.version, Users: map[string]int{}}
	s.st.Read(func(d *store.Data) {
		settings = d.Settings
		for _, u := range d.Users {
			out.Users[u.Source]++
		}
	})
	out.Settings = SettingsStatus{defaultsView: defaultsOf(settings), SetupAt: settings.SetupAt, SetupBy: settings.SetupBy}
	out.Postgres = PostgresStatus{Where: s.db.Where(), Persist: s.st.PersistStatus()}
	out.LDAP = LDAPStatus{Enabled: settings.LDAP.Enabled}

	var wg sync.WaitGroup
	wg.Go(func() { out.OpenBao = s.secrets.Status(ctx) })
	wg.Go(func() {
		info, err := s.db.Info(ctx)
		out.Postgres.Info, out.Postgres.OK = info, err == nil
		if err != nil {
			out.Postgres.Error = err.Error()
		}
	})
	if settings.LDAP.Enabled {
		l := settings.LDAP
		out.LDAP.Kind, out.LDAP.URL, out.LDAP.TLS, out.LDAP.BaseDN, out.LDAP.AdminGroup = l.Kind, l.URL, l.TLSMode(), l.BaseDN, l.AdminGroupDN
		wg.Go(func() {
			pw, err := s.secrets.Resolve(l.BindPasswordRef)
			if err == nil {
				_, err = s.dir.Test(l, pw, "", "")
			}
			out.LDAP.OK = err == nil
			if err != nil {
				out.LDAP.Error = err.Error()
			}
		})
	}
	wg.Wait()
	return out
}
