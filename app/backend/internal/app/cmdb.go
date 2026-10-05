package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/ingest"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// Health levels, from best to worst; unknown means nothing tells the state.
const (
	HealthUnknown  = "unknown"
	HealthOK       = "ok"
	HealthWarning  = "warning"
	HealthCritical = "critical"

	// eventWindow: firing events seen within it count toward health.
	eventWindow     = 24 * time.Hour
	maxRecentEvents = 5
)

var healthRank = map[string]int{HealthUnknown: 0, HealthOK: 1, HealthWarning: 2, HealthCritical: 3}

func worse(a, b string) string {
	if healthRank[b] > healthRank[a] {
		return b
	}
	return a
}

// HealthReason explains a health level: a code the interface translates, the level it leads
// to and, where it applies, a count or the item or service behind it.
type HealthReason struct {
	Code   string `json:"code"`
	Level  string `json:"level"`
	Count  int    `json:"count,omitempty"`
	Ref    string `json:"ref,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type Health struct {
	Level   string         `json:"level"`
	Reasons []HealthReason `json:"reasons"`
}

func (h *Health) add(r HealthReason) {
	h.Reasons = append(h.Reasons, r)
	h.Level = worse(h.Level, r.Level)
}

type MapEvent struct {
	Title       string    `json:"title"`
	Severity    string    `json:"severity"`
	ConnectorID string    `json:"connector_id"`
	LastSeen    time.Time `json:"last_seen"`
}

type MapEvents struct {
	Critical int        `json:"critical"`
	Error    int        `json:"error"`
	Warning  int        `json:"warning"`
	Info     int        `json:"info"`
	Recent   []MapEvent `json:"recent"`
}

type MapCI struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Source    string    `json:"source"`
	IPs       []string  `json:"ips"`
	NetBoxURL string    `json:"netbox_url,omitempty"`
	Directory string    `json:"directory,omitempty"`
	Services  []string  `json:"services"`
	Events    MapEvents `json:"events"`
	Health    Health    `json:"health"`
}

type MapService struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Criticality string   `json:"criticality"`
	Status      string   `json:"status"`
	Owner       string   `json:"owner"`
	CIIDs       []string `json:"ci_ids"`
	DependsOn   []string `json:"depends_on"`
	NetBoxURL   string   `json:"netbox_url,omitempty"`
	Health      Health   `json:"health"`
}

// MapEventsInfo says whether connector events were taken into account.
type MapEventsInfo struct {
	Available   bool   `json:"available"`
	WindowHours int    `json:"window_hours"`
	Error       string `json:"error,omitempty"`
}

type CMDBMap struct {
	Services    []MapService  `json:"services"`
	CIs         []MapCI       `json:"cis"`
	Events      MapEventsInfo `json:"events"`
	GeneratedAt time.Time     `json:"generated_at"`
}

var errEventsNotReady = errors.New("the event tables are not ready yet")

type firingSource func(ctx context.Context, since time.Time) ([]ingest.FiringEvent, error)

// CMDBService builds the map of business services, their configuration items and dependencies
// with the health of each.
type CMDBService struct {
	st     *store.Store
	firing firingSource
	now    func() time.Time
}

func NewCMDBService(st *store.Store, firing firingSource) *CMDBService {
	return &CMDBService{st: st, firing: firing, now: func() time.Time { return time.Now().UTC() }}
}

func (a *App) registerCMDB(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/cmdb", a.authed(a.can("cmdb:view", a.cmdbMap)))
}

func (a *App) cmdbMap(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, a.cmdb.Map(r.Context()))
}

func (s *CMDBService) Map(ctx context.Context) CMDBMap {
	now := s.now()
	out := CMDBMap{Services: []MapService{}, CIs: []MapCI{}, GeneratedAt: now, Events: MapEventsInfo{WindowHours: int(eventWindow / time.Hour)}}
	var events []ingest.FiringEvent
	if s.firing != nil {
		var err error
		if events, err = s.firing(ctx, now.Add(-eventWindow)); err != nil {
			out.Events.Error = err.Error()
		} else {
			out.Events.Available = true
		}
	}
	s.st.Read(func(d *store.Data) {
		g := newServiceGraph(d)
		bound := map[string][]string{}
		for _, svc := range d.Services {
			for _, id := range svc.CIIDs {
				if d.ConfigItems[id] != nil {
					bound[id] = append(bound[id], svc.ID)
				}
			}
		}
		cis := map[string]*MapCI{}
		for id, svcs := range bound {
			ci := d.ConfigItems[id]
			slices.Sort(svcs)
			m := &MapCI{ID: ci.ID, Name: ci.Name, Kind: ci.Kind, Status: ci.Status, Source: ci.Source, IPs: serviceStrings(ci.IPs), Services: svcs,
				Events: MapEvents{Recent: []MapEvent{}}}
			if ci.NetBox != nil {
				m.NetBoxURL = ci.NetBox.URL
			}
			if ci.Directory != nil {
				m.Directory = ci.Directory.Status
				if ci.Directory.Status == model.DirectoryMatched && ci.Directory.Disabled {
					m.Directory = "disabled"
				}
			}
			cis[id] = m
		}
		attachEvents(d, cis, events)
		for _, m := range cis {
			m.Health = ciHealth(m)
			out.CIs = append(out.CIs, *m)
		}
		health := serviceHealth(d, cis)
		for _, svc := range d.Services {
			m := MapService{ID: svc.ID, Name: svc.Name, Criticality: svc.Criticality, Status: svc.Status, Owner: strings.Join(g.team(svc.OwnerTeamID).Path, " / "),
				CIIDs: []string{}, DependsOn: []string{}, Health: health[svc.ID]}
			for _, id := range svc.CIIDs {
				if cis[id] != nil {
					m.CIIDs = append(m.CIIDs, id)
				}
			}
			for _, id := range svc.DependsOn {
				if d.Services[id] != nil {
					m.DependsOn = append(m.DependsOn, id)
				}
			}
			if svc.NetBox != nil {
				m.NetBoxURL = svc.NetBox.URL
			}
			out.Services = append(out.Services, m)
		}
	})
	slices.SortFunc(out.Services, func(a, b MapService) int { return byName(a.Name, b.Name) })
	slices.SortFunc(out.CIs, func(a, b MapCI) int { return byName(a.Name, b.Name) })
	return out
}

// eventKeys are the forms of the ci field of an event a configuration item may be known by:
// the value itself, without a port (Prometheus instances look like host:9100) and the short
// host name.
func eventKeys(v string) []string {
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

// attachEvents matches firing events to configuration items by name, short name, IP address
// or the DNS name the domain controller has for the item.
func attachEvents(d *store.Data, cis map[string]*MapCI, events []ingest.FiringEvent) {
	index := map[string][]string{}
	add := func(key, id string) {
		if key = strings.ToLower(strings.TrimSpace(key)); key != "" && !slices.Contains(index[key], id) {
			index[key] = append(index[key], id)
		}
	}
	for id := range cis {
		ci := d.ConfigItems[id]
		add(ci.Name, id)
		if net.ParseIP(ci.Name) == nil {
			if short, _, ok := strings.Cut(ci.Name, "."); ok {
				add(short, id)
			}
		}
		for _, ip := range ci.IPs {
			add(ip, id)
		}
		if ci.Directory != nil {
			add(ci.Directory.DNSName, id)
		}
	}
	for _, e := range events {
		var ids []string
		for _, k := range eventKeys(e.CI) {
			if ids = index[k]; len(ids) > 0 {
				break
			}
		}
		for _, id := range ids {
			m := cis[id]
			switch e.Severity {
			case "critical":
				m.Events.Critical++
			case "error":
				m.Events.Error++
			case "warning":
				m.Events.Warning++
			default:
				m.Events.Info++
			}
			if len(m.Events.Recent) < maxRecentEvents {
				m.Events.Recent = append(m.Events.Recent, MapEvent{Title: e.Title, Severity: e.Severity, ConnectorID: e.ConnectorID, LastSeen: e.LastSeen})
			}
		}
	}
}

// ciHealth: the status of the item, the computer object of the domain controller and the
// events that fire for the item.
func ciHealth(m *MapCI) Health {
	h := Health{Level: HealthUnknown, Reasons: []HealthReason{}}
	switch m.Status {
	case model.CIStatusActive:
		h.Level = HealthOK
	case model.CIStatusFailed, model.CIStatusOffline:
		h.add(HealthReason{Code: "status_" + m.Status, Level: HealthCritical})
	case model.CIStatusDecommissioning:
		h.add(HealthReason{Code: "status_" + m.Status, Level: HealthWarning})
	default:
		h.add(HealthReason{Code: "status_" + m.Status, Level: HealthUnknown})
	}
	switch m.Directory {
	case model.DirectoryMissing:
		h.add(HealthReason{Code: "directory_missing", Level: HealthWarning})
	case "disabled":
		h.add(HealthReason{Code: "directory_disabled", Level: HealthWarning})
	}
	if n := m.Events.Critical + m.Events.Error; n > 0 {
		h.add(HealthReason{Code: "events_critical", Level: HealthCritical, Count: n})
	}
	if m.Events.Warning > 0 {
		h.add(HealthReason{Code: "events_warning", Level: HealthWarning, Count: m.Events.Warning})
	}
	return h
}

// serviceHealth: a service is as healthy as the worst of its items; a service it depends on
// that is in trouble makes it a warning at most. Planned and retired services are not judged.
func serviceHealth(d *store.Data, cis map[string]*MapCI) map[string]Health {
	out := map[string]Health{}
	visiting := map[string]bool{}
	var eval func(id string) Health
	eval = func(id string) Health {
		if h, ok := out[id]; ok {
			return h
		}
		svc := d.Services[id]
		h := Health{Level: HealthUnknown, Reasons: []HealthReason{}}
		if svc == nil || visiting[id] {
			return h
		}
		visiting[id] = true
		defer delete(visiting, id)
		if svc.Status != model.ServiceActive {
			h.add(HealthReason{Code: "service_" + svc.Status, Level: HealthUnknown})
			out[id] = h
			return h
		}
		judged := false
		for _, ciID := range svc.CIIDs {
			m := cis[ciID]
			if m == nil {
				continue
			}
			if m.Health.Level != HealthUnknown {
				judged = true
				h.Level = worse(h.Level, HealthOK)
			}
			if m.Health.Level == HealthWarning || m.Health.Level == HealthCritical {
				h.add(HealthReason{Code: "ci_" + m.Health.Level, Level: m.Health.Level, Ref: m.ID, Detail: m.Name})
			}
		}
		for _, dep := range svc.DependsOn {
			dh := eval(dep)
			if dh.Level == HealthUnknown {
				continue
			}
			judged = true
			h.Level = worse(h.Level, HealthOK)
			if dh.Level == HealthWarning || dh.Level == HealthCritical {
				h.add(HealthReason{Code: "dependency_" + dh.Level, Level: HealthWarning, Ref: dep, Detail: d.Services[dep].Name})
			}
		}
		if !judged && len(h.Reasons) == 0 {
			code := "no_cis"
			if len(svc.CIIDs) > 0 {
				code = "cis_unknown"
			}
			h.add(HealthReason{Code: code, Level: HealthUnknown})
		}
		out[id] = h
		return h
	}
	for id := range d.Services {
		eval(id)
	}
	return out
}
