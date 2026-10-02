package auth

import (
	"log/slog"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// BootstrapConfig seeds access on an empty store.
type BootstrapConfig struct {
	AdminUser     string // default "admin"
	AdminPassword string // empty: a random one is generated and logged once
	// ServiceTokens are API tokens for service accounts known in advance,
	// e.g. Grafana: name -> role id + token.
	ServiceTokens []ServiceToken
}

// ServiceToken describes a service account created at start.
type ServiceToken struct {
	User  string
	Role  string
	Token string
}

// Bootstrap adds missing built-in roles, the first administrator and the
// configured service accounts. The MVP store is in memory, so this runs on
// every start; existing records are kept.
func Bootstrap(st *store.Store, cfg BootstrapConfig) {
	now := time.Now()
	if cfg.AdminUser == "" {
		cfg.AdminUser = "admin"
	}
	st.Write(func(d *store.Data) {
		for _, r := range BuiltInRoles(now) {
			if d.Roles[r.ID] == nil {
				rr := r
				rr.BuiltIn = true
				rr.UpdatedAt = now
				d.Roles[rr.ID] = &rr
			}
		}
		if d.UserByName(cfg.AdminUser) == nil {
			pw, must := cfg.AdminPassword, false
			if pw == "" {
				pw, must = RandomToken("", 12), true
				slog.Warn("created the first administrator with a one-time password; change it after signing in",
					"user", cfg.AdminUser, "password", pw)
			} else if err := CheckPolicy(pw, cfg.AdminUser); err != nil {
				slog.Warn("UMBRELLA_ADMIN_PASSWORD does not meet the password policy; it is used anyway", "err", err)
			}
			h, err := HashPassword(pw)
			if err != nil {
				panic(err)
			}
			u := &model.User{ID: d.NextID("USR"), Username: cfg.AdminUser, Name: "Администратор", Roles: []string{RoleAdmin},
				BusinessServices: []string{}, MustChangePassword: must, CreatedAt: now, PasswordHash: h, PasswordChangedAt: &now}
			d.Users[u.ID] = u
			d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: "system", Action: "user.bootstrap", Object: u.Username})
		}
		for _, stk := range cfg.ServiceTokens {
			if stk.Token == "" || stk.User == "" {
				continue
			}
			u := d.UserByName(stk.User)
			if u == nil {
				role := stk.Role
				if d.Roles[role] == nil {
					role = RoleReader
				}
				u = &model.User{ID: d.NextID("USR"), Username: stk.User, Name: stk.User, Roles: []string{role}, BusinessServices: []string{},
					Service: true, CreatedAt: now}
				d.Users[u.ID] = u
			}
			hash := TokenHash(stk.Token)
			found := false
			for _, t := range d.Tokens {
				if t.Hash == hash {
					found = true
				}
			}
			if !found {
				t := &model.APIToken{ID: d.NextID("TOK"), UserID: u.ID, Name: "bootstrap", Prefix: prefix(stk.Token), CreatedAt: now, Hash: hash}
				d.Tokens[t.ID] = t
			}
		}
	})
}

func prefix(token string) string {
	if len(token) > 10 {
		return token[:10]
	}
	return strings.Repeat("•", 4)
}

// Prefix is the visible part of a token.
func Prefix(token string) string { return prefix(token) }
