package alert

import (
	"cmp"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// EventKeys are the forms of the ci field of an event a configuration item may be known by:
// the value itself, without a port (Prometheus instances look like host:9100) and the short
// host name.
func EventKeys(v string) []string {
	v = strings.ToLower(strings.TrimSpace(v))
	keys := []string{v}
	if host, port, err := net.SplitHostPort(v); err == nil {
		if _, err := strconv.Atoi(port); err == nil {
			v = host
			keys = append(keys, v)
		}
	}
	if net.ParseIP(v) == nil {
		if short, _, ok := strings.Cut(v, "."); ok && short != "" {
			keys = append(keys, short)
		}
	}
	return keys
}

// CIKeys are the names a configuration item is known by: its name, the short host name, its
// IP addresses and the DNS name the domain controller has for it. Events and the hosts of
// monitoring systems are matched against them with EventKeys.
func CIKeys(ci *model.ConfigItem) []string {
	keys := []string{ci.Name}
	if net.ParseIP(ci.Name) == nil {
		if short, _, ok := strings.Cut(ci.Name, "."); ok {
			keys = append(keys, short)
		}
	}
	keys = append(keys, ci.IPs...)
	if ci.Directory != nil && ci.Directory.DNSName != "" {
		keys = append(keys, ci.Directory.DNSName)
	}
	return keys
}

// world is what the engine needs from the catalog for one batch of events: configuration
// items and how events name them, services, teams, people and maintenance windows. It is a
// copy, so the engine does not hold the store lock while it talks to the database.
type world struct {
	cis         map[string]model.ConfigItem
	index       map[string][]string
	services    []model.Service
	teams       map[string]model.Team
	users       map[string]model.User
	members     map[string][]string
	maintenance []model.Maintenance
}

func snapshot(st *store.Store) *world {
	w := &world{cis: map[string]model.ConfigItem{}, index: map[string][]string{}, teams: map[string]model.Team{},
		users: map[string]model.User{}, members: map[string][]string{}}
	add := func(key, id string) {
		if key = strings.ToLower(strings.TrimSpace(key)); key != "" && !slices.Contains(w.index[key], id) {
			w.index[key] = append(w.index[key], id)
		}
	}
	st.Read(func(d *store.Data) {
		for id, ci := range d.ConfigItems {
			c := *ci
			c.Owners = slices.Clone(ci.Owners)
			w.cis[id] = c
			add(ci.ID, id)
			for _, k := range CIKeys(ci) {
				add(k, id)
			}
		}
		for _, s := range d.Services {
			c := *s
			c.CIIDs = slices.Clone(s.CIIDs)
			w.services = append(w.services, c)
		}
		for id, t := range d.Teams {
			w.teams[id] = *t
		}
		for id, u := range d.Users {
			c := *u
			c.Avatar = nil
			w.users[id] = c
			if u.TeamID != "" && !u.Disabled {
				w.members[u.TeamID] = append(w.members[u.TeamID], id)
			}
		}
		for _, m := range d.Maintenance {
			w.maintenance = append(w.maintenance, *m)
		}
	})
	for _, ids := range w.index {
		slices.Sort(ids)
	}
	for _, ids := range w.members {
		slices.Sort(ids)
	}
	slices.SortFunc(w.services, func(a, b model.Service) int { return cmp.Compare(a.ID, b.ID) })
	return w
}

// resolve finds the configuration item an event is about: a ci label wins over the ci field;
// the item is matched by ID, name, short name, IP address or the DNS name of its computer
// object. An ambiguous name matches nothing.
func (w *world) resolve(name string, labels map[string]string) *model.ConfigItem {
	if v := strings.TrimSpace(labels["ci"]); v != "" {
		name = v
	}
	if strings.TrimSpace(name) == "" {
		return nil
	}
	for _, k := range EventKeys(name) {
		ids := w.index[k]
		if len(ids) == 1 {
			ci := w.cis[ids[0]]
			return &ci
		}
		if len(ids) > 1 {
			return nil
		}
	}
	return nil
}

var criticalityRank = map[string]int{model.CriticalityCritical: 4, model.CriticalityHigh: 3, model.CriticalityMedium: 2, model.CriticalityLow: 1}

// route: the item belongs to business services; the owning team of the most critical active
// one gets the alert, with its lead and members. A team with nobody hands it to its parent.
// The people responsible for the item in NetBox are kept as owners.
func (w *world) route(ci *model.ConfigItem, now time.Time) Route {
	r := Route{Services: []Ref{}, People: []Person{}, Owners: []Person{}, Via: ViaNone, At: now}
	if ci == nil {
		return r
	}
	var svcs []model.Service
	for _, s := range w.services {
		if s.Status != model.ServiceRetired && slices.Contains(s.CIIDs, ci.ID) {
			svcs = append(svcs, s)
		}
	}
	slices.SortStableFunc(svcs, func(a, b model.Service) int {
		if c := cmp.Compare(criticalityRank[b.Criticality], criticalityRank[a.Criticality]); c != 0 {
			return c
		}
		if a.Status != b.Status {
			if a.Status == model.ServiceActive {
				return -1
			}
			if b.Status == model.ServiceActive {
				return 1
			}
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	for _, s := range svcs {
		r.Services = append(r.Services, Ref{ID: s.ID, Name: s.Name})
	}
	for _, o := range ci.Owners {
		if u, ok := w.users[o.UserID]; ok && !u.Disabled {
			r.Owners = append(r.Owners, w.person(u, o.Role))
		}
	}
	for _, s := range svcs {
		t, ok := w.teams[s.OwnerTeamID]
		if !ok {
			continue
		}
		r.Team = &Ref{ID: t.ID, Name: t.Name}
		seen := map[string]bool{}
		for cur, depth := t, 0; depth < 16 && len(r.People) == 0; depth++ {
			if seen[cur.ID] {
				break
			}
			seen[cur.ID] = true
			r.People = w.teamPeople(cur)
			p, ok := w.teams[cur.ParentID]
			if !ok {
				break
			}
			cur = p
		}
		break
	}
	switch {
	case len(r.People) > 0:
		r.Via = ViaService
	case len(r.Owners) > 0:
		r.Via = ViaCIOwners
	}
	return r
}

func (w *world) teamPeople(t model.Team) []Person {
	out := []Person{}
	if u, ok := w.users[t.LeadID]; ok && !u.Disabled {
		out = append(out, w.person(u, "lead"))
	}
	for _, id := range w.members[t.ID] {
		if id != t.LeadID {
			out = append(out, w.person(w.users[id], "member"))
		}
	}
	return out
}

func (w *world) person(u model.User, role string) Person {
	name := u.Name
	if name == "" {
		name = u.DisplayName(u.Username)
	}
	return Person{UserID: u.ID, Name: name, Email: u.Email, Telegram: u.Telegram, Role: role}
}

// maintenanceFor returns the active window that covers an alert.
func (w *world) maintenanceFor(a *Alert, now time.Time) *model.Maintenance {
	for i := range w.maintenance {
		m := &w.maintenance[i]
		if m.State(now) == model.MaintenanceActive && m.Covers(a.CIID, a.Route.ServiceIDs()) {
			return m
		}
	}
	return nil
}
