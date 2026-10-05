package api

import (
	"errors"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

const sessionCookie = "umb_session"

// Login protection: a user is locked after maxFailures wrong passwords in a
// row; an address is slowed down after ipFailures failures in ipWindow.
const (
	maxFailures = 5
	lockFor     = 5 * time.Minute
	ipFailures  = 20
	ipWindow    = 10 * time.Minute
)

var errBadLogin = errors.New("неверный логин или пароль")

type ipLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func (l *ipLimiter) blocked(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	keep := l.hits[ip][:0]
	for _, t := range l.hits[ip] {
		if now.Sub(t) < ipWindow {
			keep = append(keep, t)
		}
	}
	l.hits[ip] = keep
	return len(keep) >= ipFailures
}

func (l *ipLimiter) fail(ip string, now time.Time) {
	l.mu.Lock()
	l.hits[ip] = append(l.hits[ip], now)
	l.mu.Unlock()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// authenticate resolves the session cookie or a bearer token.
func (s *Server) authenticate(r *http.Request) *auth.Principal {
	via, sid, tokenID := "", "", ""
	var userID string
	if v, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, "umb_") {
			hash := auth.TokenHash(v)
			now := time.Now()
			s.st.Read(func(d *store.Data) {
				for _, t := range d.Tokens {
					if t.Hash == hash && (t.ExpiresAt == nil || now.Before(*t.ExpiresAt)) {
						userID, tokenID = t.UserID, t.ID
					}
				}
			})
			via = "token"
		} else if ss := s.sessions.Get(v); ss != nil {
			userID, sid, via = ss.UserID, ss.ID, "bearer"
		}
	} else if c, err := r.Cookie(sessionCookie); err == nil {
		if ss := s.sessions.Get(c.Value); ss != nil {
			userID, sid, via = ss.UserID, ss.ID, "session"
		}
	}
	if userID == "" {
		return nil
	}
	var p *auth.Principal
	s.st.Read(func(d *store.Data) {
		u := d.Users[userID]
		if u == nil || u.Disabled {
			return
		}
		p = auth.Build(d, u)
	})
	if p == nil {
		return nil
	}
	p.Via, p.SessionID = via, sid
	if sid != "" {
		if ss := s.sessions.Get(sid); ss != nil {
			p.CSRF = ss.CSRF
		}
	}
	if tokenID != "" {
		s.touchToken(tokenID)
	}
	return p
}

// touchToken records token use at most once a minute.
func (s *Server) touchToken(id string) {
	now := time.Now()
	var stale bool
	s.st.Read(func(d *store.Data) {
		if t := d.Tokens[id]; t != nil {
			stale = t.LastUsedAt == nil || now.Sub(*t.LastUsedAt) > time.Minute
		}
	})
	if stale {
		s.st.Write(func(d *store.Data) {
			if t := d.Tokens[id]; t != nil {
				t.LastUsedAt = &now
			}
		})
	}
}

// require wraps a handler: the caller must be signed in and hold perm
// (empty perm: any signed-in user). Cookie sessions also need the CSRF
// header on changing requests.
func (s *Server) require(perm string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := s.authenticate(r)
		if p == nil {
			writeJSON(w, 401, map[string]string{"error": "нужно войти", "code": "unauthorized"})
			return
		}
		if p.Via == "session" && r.Method != http.MethodGet && r.Method != http.MethodHead {
			if p.CSRF == "" || r.Header.Get("X-Umbrella-CSRF") != p.CSRF {
				writeJSON(w, 403, map[string]string{"error": "запрос без CSRF-токена", "code": "csrf"})
				return
			}
		}
		if p.User.MustChangePassword && !strings.HasPrefix(r.URL.Path, "/api/auth/") {
			writeJSON(w, 403, map[string]string{"error": "смените пароль", "code": "password_change_required"})
			return
		}
		if perm != "" && !p.Can(perm) {
			writeJSON(w, 403, map[string]string{"error": "недостаточно прав: " + perm, "code": "forbidden"})
			return
		}
		h(w, r.WithContext(auth.With(r.Context(), p)))
	}
}

// me returns the principal of the request (set by require).
func me(r *http.Request) *auth.Principal { return auth.From(r.Context()) }

// ---- sign-in ----

type loginBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// login checks the password and returns the user, or an error for the client.
func (s *Server) login(r *http.Request, body loginBody) (*model.User, int, error) {
	now := time.Now()
	ip := clientIP(r)
	if s.limiter.blocked(ip, now) {
		return nil, 429, errors.New("слишком много попыток входа, попробуйте позже")
	}
	var hash string
	var u *model.User
	s.st.Read(func(d *store.Data) {
		if x := d.UserByName(strings.TrimSpace(body.Username)); x != nil {
			c := *x
			u, hash = &c, x.PasswordHash
		}
	})
	if u != nil && u.LockedUntil != nil && now.Before(*u.LockedUntil) {
		auth.CheckPassword("", body.Password)
		s.limiter.fail(ip, now)
		return nil, 423, errors.New("учётная запись временно заблокирована после неудачных попыток входа")
	}
	ok := auth.CheckPassword(hash, body.Password)
	if u == nil || !ok || u.Disabled || u.Service {
		s.limiter.fail(ip, now)
		if u != nil {
			s.st.Write(func(d *store.Data) {
				x := d.Users[u.ID]
				if x == nil {
					return
				}
				x.FailedLogins++
				if x.FailedLogins >= maxFailures {
					t := now.Add(lockFor)
					x.LockedUntil = &t
					x.FailedLogins = 0
					d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: "system", Action: "user.locked", Object: x.Username})
				}
			})
		}
		return nil, 401, errBadLogin
	}
	s.st.Write(func(d *store.Data) {
		if x := d.Users[u.ID]; x != nil {
			x.FailedLogins = 0
			x.LockedUntil = nil
			x.LastLoginAt = &now
			d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: x.Username, Action: "auth.login", Object: ip})
		}
	})
	return u, 200, nil
}

func (s *Server) secureCookies(r *http.Request) bool {
	return s.cfg.SecureCookies || r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// POST /api/auth/login: browser sign-in with a session cookie.
func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	var body loginBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	u, code, err := s.login(r, body)
	if err != nil {
		writeErr(w, code, err)
		return
	}
	ss := s.sessions.Create(u.ID)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: ss.ID, Path: "/", HttpOnly: true, Secure: s.secureCookies(r),
		SameSite: http.SameSiteStrictMode, MaxAge: int(auth.MaxLifetime.Seconds())})
	r = r.WithContext(auth.With(r.Context(), s.principalOf(u.ID, ss)))
	s.authMe(w, r)
}

// POST /api/auth/token: API sign-in for scripts; returns a bearer session
// token instead of a cookie.
func (s *Server) authToken(w http.ResponseWriter, r *http.Request) {
	var body loginBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	u, code, err := s.login(r, body)
	if err != nil {
		writeErr(w, code, err)
		return
	}
	ss := s.sessions.Create(u.ID)
	writeJSON(w, 200, map[string]any{"token": ss.ID, "expires_in": int(auth.IdleTimeout.Seconds()), "must_change_password": u.MustChangePassword})
}

func (s *Server) principalOf(userID string, ss *auth.Session) *auth.Principal {
	var p *auth.Principal
	s.st.Read(func(d *store.Data) {
		if u := d.Users[userID]; u != nil {
			p = auth.Build(d, u)
		}
	})
	if p != nil && ss != nil {
		p.Via, p.SessionID, p.CSRF = "session", ss.ID, ss.CSRF
	}
	return p
}

// GET /api/auth/me
func (s *Server) authMe(w http.ResponseWriter, r *http.Request) {
	p := me(r)
	var services []map[string]string
	s.st.Read(func(d *store.Data) {
		for _, id := range p.User.BusinessServices {
			if ci := d.CIs[id]; ci != nil {
				services = append(services, map[string]string{"id": ci.ID, "name": ci.Name})
			}
		}
	})
	if services == nil {
		services = []map[string]string{}
	}
	writeJSON(w, 200, map[string]any{"user": p.User, "permissions": p.PermList(), "all_services": p.AllServices,
		"business_services": services, "csrf": p.CSRF})
}

// POST /api/auth/logout
func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	if p := me(r); p != nil && p.SessionID != "" {
		s.sessions.Delete(p.SessionID)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: s.secureCookies(r),
		SameSite: http.SameSiteStrictMode, MaxAge: -1})
	w.WriteHeader(204)
}

// POST /api/auth/password {current, new}
func (s *Server) authPassword(w http.ResponseWriter, r *http.Request) {
	p := me(r)
	var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	var hash string
	s.st.Read(func(d *store.Data) {
		if u := d.Users[p.User.ID]; u != nil {
			hash = u.PasswordHash
		}
	})
	if !auth.CheckPassword(hash, body.Current) {
		writeErr(w, 403, errors.New("текущий пароль не подходит"))
		return
	}
	if body.New == body.Current {
		writeErr(w, 400, errors.New("новый пароль совпадает с текущим"))
		return
	}
	if err := auth.CheckPolicy(body.New, p.User.Username); err != nil {
		writeErr(w, 400, err)
		return
	}
	h, err := auth.HashPassword(body.New)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	now := time.Now()
	s.st.Write(func(d *store.Data) {
		if u := d.Users[p.User.ID]; u != nil {
			u.PasswordHash, u.MustChangePassword, u.PasswordChangedAt = h, false, &now
		}
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: p.Name(), Action: "auth.password", Object: p.User.Username})
	})
	s.sessions.DeleteUser(p.User.ID, p.SessionID)
	w.WriteHeader(204)
}

// ---- users ----

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]{2,64}$`)

type userView struct {
	model.User
	Sessions int `json:"sessions"`
	Tokens   int `json:"tokens"`
}

// GET /api/users
func (s *Server) listUsers(w http.ResponseWriter, _ *http.Request) {
	counts := s.sessions.Count()
	out := []userView{}
	s.st.Read(func(d *store.Data) {
		tok := map[string]int{}
		for _, t := range d.Tokens {
			tok[t.UserID]++
		}
		for _, u := range d.Users {
			c := *u
			out = append(out, userView{User: c, Sessions: counts[u.ID], Tokens: tok[u.ID]})
		}
	})
	sort.Slice(out, func(i, j int) bool { return idNum(out[i].ID) < idNum(out[j].ID) })
	writeJSON(w, 200, map[string]any{"items": out})
}

type userBody struct {
	Username         *string   `json:"username"`
	Name             *string   `json:"name"`
	Email            *string   `json:"email"`
	Roles            *[]string `json:"roles"`
	BusinessServices *[]string `json:"business_services"`
	Disabled         *bool     `json:"disabled"`
	Service          *bool     `json:"service"`
	Password         string    `json:"password"`
	MustChange       *bool     `json:"must_change_password"`
}

// validate checks references; call inside a store read or write.
func (b *userBody) validate(d *store.Data) error {
	if b.Roles != nil {
		for _, r := range *b.Roles {
			if d.Roles[r] == nil {
				return errors.New("нет роли " + r)
			}
		}
	}
	if b.BusinessServices != nil {
		for _, id := range *b.BusinessServices {
			ci := d.CIs[id]
			if ci == nil || (ci.Type != model.CIBusinessService && ci.Type != model.CIITService) {
				return errors.New("КЕ " + id + " не бизнес-услуга и не ИТ-сервис")
			}
		}
	}
	return nil
}

// adminCount counts enabled users with users.admin, treating u as replaced by next.
func adminCount(d *store.Data, skipID string, next *model.User) int {
	n := 0
	check := func(u *model.User) {
		if u.Disabled {
			return
		}
		for _, rid := range u.Roles {
			if r := d.Roles[rid]; r != nil {
				for _, p := range r.Permissions {
					if p == model.PermUsersAdmin {
						n++
						return
					}
				}
			}
		}
	}
	for _, u := range d.Users {
		if u.ID != skipID {
			check(u)
		}
	}
	if next != nil {
		check(next)
	}
	return n
}

// POST /api/users
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var b userBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	if b.Username == nil || !usernameRe.MatchString(*b.Username) {
		writeErr(w, 400, errors.New("логин: 2–64 символа, латиница, цифры, точка, дефис, подчёркивание"))
		return
	}
	service := b.Service != nil && *b.Service
	var hash string
	if !service {
		if err := auth.CheckPolicy(b.Password, *b.Username); err != nil {
			writeErr(w, 400, err)
			return
		}
		h, err := auth.HashPassword(b.Password)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		hash = h
	}
	now := time.Now()
	var u model.User
	var err error
	s.st.Write(func(d *store.Data) {
		if d.UserByName(*b.Username) != nil {
			err = errors.New("логин уже занят")
			return
		}
		if err = b.validate(d); err != nil {
			return
		}
		u = model.User{ID: d.NextID("USR"), Username: *b.Username, Roles: []string{}, BusinessServices: []string{}, Service: service,
			CreatedAt: now, PasswordHash: hash, MustChangePassword: !service && (b.MustChange == nil || *b.MustChange)}
		applyUser(&u, &b)
		if hash != "" {
			u.PasswordChangedAt = &now
		}
		d.Users[u.ID] = &u
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: me(r).Name(), Action: "user.create", Object: u.Username})
	})
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 201, u)
}

func applyUser(u *model.User, b *userBody) {
	if b.Name != nil {
		u.Name = strings.TrimSpace(*b.Name)
	}
	if b.Email != nil {
		u.Email = strings.TrimSpace(*b.Email)
	}
	if b.Roles != nil {
		u.Roles = dedupe(*b.Roles)
	}
	if b.BusinessServices != nil {
		u.BusinessServices = dedupe(*b.BusinessServices)
	}
	if b.Disabled != nil {
		u.Disabled = *b.Disabled
	}
	if u.Name == "" {
		u.Name = u.Username
	}
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, x := range in {
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// PUT /api/users/{id}
func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	var b userBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	id := r.PathValue("id")
	now := time.Now()
	var out *model.User
	var err error
	code := 400
	s.st.Write(func(d *store.Data) {
		u := d.Users[id]
		if u == nil {
			err, code = errors.New("пользователь не найден"), 404
			return
		}
		if err = b.validate(d); err != nil {
			return
		}
		next := *u
		applyUser(&next, &b)
		if b.MustChange != nil && !next.Service {
			next.MustChangePassword = *b.MustChange
		}
		if adminCount(d, u.ID, &next) == 0 {
			err, code = errors.New("нельзя оставить систему без активного администратора"), 409
			return
		}
		*u = next
		c := *u
		out = &c
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: me(r).Name(), Action: "user.update", Object: u.Username})
	})
	if err != nil {
		writeErr(w, code, err)
		return
	}
	if out.Disabled {
		s.sessions.DeleteUser(out.ID, "")
	}
	writeJSON(w, 200, out)
}

// DELETE /api/users/{id}
func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var err error
	code := 409
	s.st.Write(func(d *store.Data) {
		u := d.Users[id]
		if u == nil {
			err, code = errors.New("пользователь не найден"), 404
			return
		}
		if u.ID == me(r).User.ID {
			err = errors.New("нельзя удалить свою учётную запись")
			return
		}
		if adminCount(d, u.ID, nil) == 0 {
			err = errors.New("нельзя оставить систему без активного администратора")
			return
		}
		delete(d.Users, id)
		for tid, t := range d.Tokens {
			if t.UserID == id {
				delete(d.Tokens, tid)
			}
		}
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: me(r).Name(), Action: "user.delete", Object: u.Username})
	})
	if err != nil {
		writeErr(w, code, err)
		return
	}
	s.sessions.DeleteUser(id, "")
	w.WriteHeader(204)
}

// POST /api/users/{id}/password {password}: administrator reset. The user
// must change it at the next sign-in.
func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	id := r.PathValue("id")
	var username string
	s.st.Read(func(d *store.Data) {
		if u := d.Users[id]; u != nil && !u.Service {
			username = u.Username
		}
	})
	if username == "" {
		writeErr(w, 404, errors.New("пользователь не найден или сервисный"))
		return
	}
	if err := auth.CheckPolicy(body.Password, username); err != nil {
		writeErr(w, 400, err)
		return
	}
	h, err := auth.HashPassword(body.Password)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	now := time.Now()
	s.st.Write(func(d *store.Data) {
		if u := d.Users[id]; u != nil {
			u.PasswordHash, u.MustChangePassword, u.PasswordChangedAt, u.LockedUntil, u.FailedLogins = h, id != me(r).User.ID, &now, nil, 0
		}
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: me(r).Name(), Action: "user.password_reset", Object: username})
	})
	s.sessions.DeleteUser(id, me(r).SessionID)
	w.WriteHeader(204)
}

// ---- API tokens ----

// canManageTokens: own tokens, or anybody's with users.admin.
func canManageTokens(p *auth.Principal, userID string) bool {
	return p.User.ID == userID || p.Can(model.PermUsersAdmin)
}

// GET /api/users/{id}/tokens
func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !canManageTokens(me(r), id) {
		writeErr(w, 403, errors.New("недостаточно прав"))
		return
	}
	out := []model.APIToken{}
	s.st.Read(func(d *store.Data) {
		for _, t := range d.Tokens {
			if t.UserID == id {
				out = append(out, *t)
			}
		}
	})
	sort.Slice(out, func(i, j int) bool { return idNum(out[i].ID) < idNum(out[j].ID) })
	writeJSON(w, 200, map[string]any{"items": out})
}

// POST /api/users/{id}/tokens {name, days}: the token is returned once.
func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !canManageTokens(me(r), id) {
		writeErr(w, 403, errors.New("недостаточно прав"))
		return
	}
	var body struct {
		Name string `json:"name"`
		Days int    `json:"days"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeErr(w, 400, errors.New("нужно название токена"))
		return
	}
	token := auth.RandomToken("umb_", 32)
	now := time.Now()
	var t model.APIToken
	ok := false
	s.st.Write(func(d *store.Data) {
		if d.Users[id] == nil {
			return
		}
		ok = true
		t = model.APIToken{ID: d.NextID("TOK"), UserID: id, Name: strings.TrimSpace(body.Name), Prefix: auth.Prefix(token), CreatedAt: now, Hash: auth.TokenHash(token)}
		if body.Days > 0 {
			exp := now.Add(time.Duration(body.Days) * 24 * time.Hour)
			t.ExpiresAt = &exp
		}
		d.Tokens[t.ID] = &t
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: me(r).Name(), Action: "token.create", Object: d.Users[id].Username + "/" + t.Name})
	})
	if !ok {
		writeErr(w, 404, errors.New("пользователь не найден"))
		return
	}
	writeJSON(w, 201, map[string]any{"token": token, "item": t})
}

// DELETE /api/users/{id}/tokens/{tid}
func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request) {
	id, tid := r.PathValue("id"), r.PathValue("tid")
	if !canManageTokens(me(r), id) {
		writeErr(w, 403, errors.New("недостаточно прав"))
		return
	}
	s.st.Write(func(d *store.Data) {
		if t := d.Tokens[tid]; t != nil && t.UserID == id {
			delete(d.Tokens, tid)
			d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: me(r).Name(), Action: "token.delete", Object: tid})
		}
	})
	w.WriteHeader(204)
}

// ---- roles ----

// GET /api/roles
func (s *Server) listRoles(w http.ResponseWriter, _ *http.Request) {
	out := []model.Role{}
	users := map[string]int{}
	s.st.Read(func(d *store.Data) {
		for _, r := range d.Roles {
			out = append(out, *r)
		}
		for _, u := range d.Users {
			for _, rid := range u.Roles {
				users[rid]++
			}
		}
	})
	order := map[string]int{}
	for i, r := range auth.BuiltInRoles(time.Now()) {
		order[r.ID] = i + 1
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := order[out[i].ID], order[out[j].ID]
		if a == 0 {
			a = 1000
		}
		if b == 0 {
			b = 1000
		}
		if a != b {
			return a < b
		}
		return out[i].Name < out[j].Name
	})
	writeJSON(w, 200, map[string]any{"items": out, "users": users, "permissions": model.AllPermissions})
}

var roleIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,40}$`)

type roleBody struct {
	ID          string    `json:"id"`
	Name        *string   `json:"name"`
	Description *string   `json:"description"`
	Permissions *[]string `json:"permissions"`
	AllServices *bool     `json:"all_services"`
}

func (b *roleBody) apply(rl *model.Role) error {
	if b.Name != nil {
		if strings.TrimSpace(*b.Name) == "" {
			return errors.New("нужно название роли")
		}
		rl.Name = strings.TrimSpace(*b.Name)
	}
	if b.Description != nil {
		rl.Description = strings.TrimSpace(*b.Description)
	}
	if b.Permissions != nil {
		for _, p := range *b.Permissions {
			if !auth.ValidPermission(p) {
				return errors.New("неизвестное право " + p)
			}
		}
		rl.Permissions = dedupe(*b.Permissions)
	}
	if b.AllServices != nil {
		rl.AllServices = *b.AllServices
	}
	return nil
}

// POST /api/roles
func (s *Server) createRole(w http.ResponseWriter, r *http.Request) {
	var b roleBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	if !roleIDRe.MatchString(b.ID) {
		writeErr(w, 400, errors.New("код роли: латиница в нижнем регистре, цифры, дефис"))
		return
	}
	rl := model.Role{ID: b.ID, Permissions: []string{}, UpdatedAt: time.Now()}
	if b.Name == nil {
		writeErr(w, 400, errors.New("нужно название роли"))
		return
	}
	if err := b.apply(&rl); err != nil {
		writeErr(w, 400, err)
		return
	}
	var err error
	s.st.Write(func(d *store.Data) {
		if d.Roles[rl.ID] != nil {
			err = errors.New("роль с таким кодом уже есть")
			return
		}
		d.Roles[rl.ID] = &rl
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: me(r).Name(), Action: "role.create", Object: rl.ID})
	})
	if err != nil {
		writeErr(w, 409, err)
		return
	}
	writeJSON(w, 201, rl)
}

// PUT /api/roles/{id}
func (s *Server) updateRole(w http.ResponseWriter, r *http.Request) {
	var b roleBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	id := r.PathValue("id")
	var out model.Role
	var err error
	code := 400
	s.st.Write(func(d *store.Data) {
		rl := d.Roles[id]
		if rl == nil {
			err, code = errors.New("роль не найдена"), 404
			return
		}
		if id == auth.RoleAdmin && (b.Permissions != nil || b.AllServices != nil) {
			err, code = errors.New("права роли администратора не меняются"), 409
			return
		}
		next := *rl
		next.Permissions = append([]string(nil), rl.Permissions...)
		if err = b.apply(&next); err != nil {
			return
		}
		old := *rl
		*rl = next
		if adminCount(d, "", nil) == 0 {
			*rl = old
			err, code = errors.New("нельзя оставить систему без активного администратора"), 409
			return
		}
		rl.UpdatedAt = time.Now()
		out = *rl
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: me(r).Name(), Action: "role.update", Object: id})
	})
	if err != nil {
		writeErr(w, code, err)
		return
	}
	writeJSON(w, 200, out)
}

// DELETE /api/roles/{id}
func (s *Server) deleteRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var err error
	s.st.Write(func(d *store.Data) {
		rl := d.Roles[id]
		if rl == nil {
			return
		}
		if rl.BuiltIn {
			err = errors.New("встроенную роль удалить нельзя")
			return
		}
		for _, u := range d.Users {
			for _, x := range u.Roles {
				if x == id {
					err = errors.New("роль назначена пользователю " + u.Username)
					return
				}
			}
		}
		delete(d.Roles, id)
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: me(r).Name(), Action: "role.delete", Object: id})
	})
	if err != nil {
		writeErr(w, 409, err)
		return
	}
	w.WriteHeader(204)
}
