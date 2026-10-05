package store

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

const (
	MaxEvents      = 20000
	MaxParseErrors = 2000
	MaxDeliveries  = 1000
	MaxAudit       = 5000
)

type Data struct {
	CIs          map[string]*model.CI
	Relations    []model.Relation
	Connectors   map[string]*model.Connector
	Events       []*model.Event
	Alerts       map[string]*model.Alert
	ParseErrors  []*model.ParseError
	Maintenance  map[string]*model.Maintenance
	Audit        []AuditEntry
	Users        map[string]*model.User
	Roles        map[string]*model.Role
	Tokens       map[string]*model.APIToken
	Channels     map[string]*model.Channel
	Deliveries   []*model.Delivery
	Integrations map[string]*model.Integration
	Rules        map[string]*model.Rule
	Teams        map[string]*model.Team
	PagerDuty    model.PDSettings
	OnCall       model.OnCall
	Settings     model.Settings
	Seq          map[string]int
}

type AuditEntry struct {
	At     string `json:"at"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Object string `json:"object"`
}

type Store struct {
	mu      sync.RWMutex
	d       Data
	version uint64

	persistMu sync.Mutex
	backend   Backend
	saved     uint64
	lastErr   error
}

func New() *Store {
	s := &Store{}
	s.d.init()
	return s
}

func (d *Data) init() {
	if d.CIs == nil {
		d.CIs = map[string]*model.CI{}
	}
	if d.Connectors == nil {
		d.Connectors = map[string]*model.Connector{}
	}
	if d.Alerts == nil {
		d.Alerts = map[string]*model.Alert{}
	}
	if d.Maintenance == nil {
		d.Maintenance = map[string]*model.Maintenance{}
	}
	if d.Users == nil {
		d.Users = map[string]*model.User{}
	}
	if d.Roles == nil {
		d.Roles = map[string]*model.Role{}
	}
	if d.Tokens == nil {
		d.Tokens = map[string]*model.APIToken{}
	}
	if d.Channels == nil {
		d.Channels = map[string]*model.Channel{}
	}
	if d.Integrations == nil {
		d.Integrations = map[string]*model.Integration{}
	}
	if d.Teams == nil {
		d.Teams = map[string]*model.Team{}
	}
	if d.Rules == nil {
		d.Rules = map[string]*model.Rule{}
	}
	if d.Seq == nil {
		d.Seq = map[string]int{}
	}
	if d.PagerDuty.Region == "" {
		d.PagerDuty.Region = model.PDRegionUS
	}
	if d.PagerDuty.MinSeverity == "" {
		d.PagerDuty.MinSeverity = model.SevInfo
	}
}

func (s *Store) Read(f func(d *Data)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f(&s.d)
}

func (s *Store) Write(f func(d *Data)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.version++
	f(&s.d)
}

func (d *Data) NextID(prefix string) string {
	d.Seq[prefix]++
	return fmt.Sprintf("%s-%d", prefix, d.Seq[prefix])
}

func (d *Data) UseID(id string) {
	i := strings.LastIndex(id, "-")
	if i < 0 {
		return
	}
	if n, err := strconv.Atoi(id[i+1:]); err == nil && n > d.Seq[id[:i]] {
		d.Seq[id[:i]] = n
	}
}

func ring[T any](list []T, v T, max int) []T {
	list = append(list, v)
	if len(list) > max {
		list = append([]T(nil), list[len(list)-max:]...)
	}
	return list
}

func (d *Data) AddEvent(e *model.Event) { d.Events = ring(d.Events, e, MaxEvents) }

func (d *Data) AddParseError(p *model.ParseError) {
	d.ParseErrors = ring(d.ParseErrors, p, MaxParseErrors)
}

func (d *Data) AddDelivery(x *model.Delivery) { d.Deliveries = ring(d.Deliveries, x, MaxDeliveries) }

func (d *Data) AddAudit(a AuditEntry) { d.Audit = ring(d.Audit, a, MaxAudit) }

func (d *Data) UserByName(username string) *model.User {
	for _, u := range d.Users {
		if strings.EqualFold(u.Username, username) {
			return u
		}
	}
	return nil
}

func (d *Data) DeleteCI(id string) {
	delete(d.CIs, id)
	kept := d.Relations[:0]
	for _, rel := range d.Relations {
		if rel.From != id && rel.To != id {
			kept = append(kept, rel)
		}
	}
	d.Relations = kept
	for mid, m := range d.Maintenance {
		if m.CIID == id {
			delete(d.Maintenance, mid)
		}
	}
	for _, a := range d.Alerts {
		if a.CIID == id {
			a.CIID = ""
		}
	}
	for _, c := range d.Channels {
		c.Services = without(c.Services, id)
	}
	for _, u := range d.Users {
		u.BusinessServices = without(u.BusinessServices, id)
	}
}

func (d *Data) TeamUsage() map[string]int {
	use := map[string]int{}
	for _, ci := range d.CIs {
		if ci.Team != "" {
			use[ci.Team]++
		}
	}
	for _, it := range d.Integrations {
		if it.Team != "" {
			use[it.Team]++
		}
	}
	for _, r := range d.Rules {
		if r.Team != "" {
			use[r.Team]++
		}
	}
	for _, c := range d.Connectors {
		if c.Team != "" {
			use[c.Team]++
		}
	}
	return use
}

func without(list []string, v string) []string {
	out := list[:0]
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
