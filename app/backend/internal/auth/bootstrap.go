package auth

import (
	"log/slog"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type BootstrapConfig struct {
	AdminUser     string
	AdminPassword string

	ServiceTokens []ServiceToken
}

type ServiceToken struct {
	User  string
	Role  string
	Token string
}

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
		for _, r := range d.Roles {
			if r.ID == RoleAdmin {
				r.Permissions = append([]string(nil), model.AllPermissions...)
				continue
			}
			kept := r.Permissions[:0]
			for _, p := range r.Permissions {
				if ValidPermission(p) {
					kept = append(kept, p)
				}
			}
			r.Permissions = kept
		}
		if d.UserByName(cfg.AdminUser) == nil && cfg.AdminPassword != "" {
			if err := CheckPolicy(cfg.AdminPassword, cfg.AdminUser); err != nil {
				slog.Warn("UMBRELLA_ADMIN_PASSWORD does not meet the password policy; it is used anyway", "err", err)
			}
			h, err := HashPassword(cfg.AdminPassword)
			if err != nil {
				panic(err)
			}
			u := &model.User{ID: d.NextID("USR"), Username: cfg.AdminUser, Name: "Администратор", Roles: []string{RoleAdmin},
				BusinessServices: []string{}, CreatedAt: now, PasswordHash: h, PasswordChangedAt: &now}
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

func Prefix(token string) string { return prefix(token) }
