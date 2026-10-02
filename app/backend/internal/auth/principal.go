package auth

import (
	"context"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// Principal is the signed-in user of one request.
type Principal struct {
	User        model.User
	Perms       map[string]bool
	AllServices bool
	// Scope holds the CI ids the user may see when AllServices is false:
	// the bound business services and everything they depend on.
	Scope map[string]bool
	// Via is "session" or "token"; only sessions need the CSRF header.
	Via       string
	SessionID string
	CSRF      string
}

// Can reports whether the user has a permission.
func (p *Principal) Can(perm string) bool { return p != nil && p.Perms[perm] }

// SeesCI reports whether the user may see things bound to the CI. Incidents
// without a CI are visible only with access to all services.
func (p *Principal) SeesCI(ciID string) bool {
	if p == nil {
		return false
	}
	if p.AllServices {
		return true
	}
	return ciID != "" && p.Scope[ciID]
}

// Name is how the user appears in timelines and the audit log.
func (p *Principal) Name() string {
	if p == nil {
		return "system"
	}
	return p.User.Username
}

// PermList returns the granted permissions in UI order.
func (p *Principal) PermList() []string {
	out := []string{}
	for _, x := range model.AllPermissions {
		if p.Perms[x] {
			out = append(out, x)
		}
	}
	return out
}

// Build computes permissions and scope for a user. Call inside a store read.
func Build(d *store.Data, u *model.User) *Principal {
	p := &Principal{User: *u, Perms: map[string]bool{}}
	p.User.PasswordHash = ""
	for _, rid := range u.Roles {
		r := d.Roles[rid]
		if r == nil {
			continue
		}
		for _, perm := range r.Permissions {
			p.Perms[perm] = true
		}
		if r.AllServices {
			p.AllServices = true
		}
	}
	if !p.AllServices {
		p.Scope = ServiceScope(d, u.BusinessServices)
	}
	return p
}

// ServiceScope returns the given CIs and everything they depend on
// (relations go From -> To, "From depends on To").
func ServiceScope(d *store.Data, roots []string) map[string]bool {
	down := map[string][]string{}
	for _, r := range d.Relations {
		down[r.From] = append(down[r.From], r.To)
	}
	seen := map[string]bool{}
	stack := append([]string(nil), roots...)
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[id] || d.CIs[id] == nil {
			continue
		}
		seen[id] = true
		stack = append(stack, down[id]...)
	}
	return seen
}

type ctxKey struct{}

// With stores the principal in a context.
func With(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// From returns the principal of a request context, or nil.
func From(ctx context.Context) *Principal {
	p, _ := ctx.Value(ctxKey{}).(*Principal)
	return p
}
