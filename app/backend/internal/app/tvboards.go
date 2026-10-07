package app

import (
	"cmp"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/netacl"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

const (
	maxTVBoards       = 100
	maxTVName         = 100
	maxTVTargets      = 500
	maxTVSources      = 100
	minTVRefresh      = 5
	maxTVRefresh      = 600
	defaultTVRefresh  = 15
	maxTVIncidents    = 300
	tvSlugBytes       = 20
	maxTrustedProxies = 50
)

var slugEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

func newSlug() string {
	b := make([]byte, tvSlugBytes)
	_, _ = rand.Read(b)
	return strings.ToLower(slugEncoding.EncodeToString(b))
}

// TVBoardsService keeps the incident boards for wall screens and answers the screens.
type TVBoardsService struct {
	st  *store.Store
	now func() time.Time

	mu   sync.Mutex
	seen map[string]tvSeen
}

// tvSeen is when and from where a board was last opened; it is kept in memory only, so a
// screen polling every few seconds does not rewrite the saved state.
type tvSeen struct {
	At time.Time `json:"at"`
	IP string    `json:"ip"`
}

func NewTVBoardsService(st *store.Store) *TVBoardsService {
	return &TVBoardsService{st: st, now: func() time.Time { return time.Now().UTC() }, seen: map[string]tvSeen{}}
}

type TVBoardView struct {
	model.TVBoard
	CIs      []TargetRef `json:"cis"`
	Services []TargetRef `json:"services"`
	Teams    []TargetRef `json:"teams"`
	LastSeen *tvSeen     `json:"last_seen,omitempty"`
}

func (s *TVBoardsService) view(d *store.Data, b *model.TVBoard) TVBoardView {
	v := TVBoardView{TVBoard: *b, CIs: []TargetRef{}, Services: []TargetRef{}, Teams: []TargetRef{}}
	ref := func(id, name string) TargetRef {
		if name == "" {
			return TargetRef{ID: id, Name: id, Missing: true}
		}
		return TargetRef{ID: id, Name: name}
	}
	for _, id := range b.CIIDs {
		name := ""
		if ci := d.ConfigItems[id]; ci != nil {
			name = ci.Name
		}
		v.CIs = append(v.CIs, ref(id, name))
	}
	for _, id := range b.ServiceIDs {
		name := ""
		if svc := d.Services[id]; svc != nil {
			name = svc.Name
		}
		v.Services = append(v.Services, ref(id, name))
	}
	for _, id := range b.TeamIDs {
		name := ""
		if t := d.Teams[id]; t != nil {
			name = t.Name
		}
		v.Teams = append(v.Teams, ref(id, name))
	}
	s.mu.Lock()
	if seen, ok := s.seen[b.ID]; ok {
		v.LastSeen = &seen
	}
	s.mu.Unlock()
	return v
}

func (s *TVBoardsService) List() []TVBoardView {
	out := []TVBoardView{}
	s.st.Read(func(d *store.Data) {
		for _, b := range d.TVBoards {
			out = append(out, s.view(d, b))
		}
	})
	slices.SortFunc(out, func(a, b TVBoardView) int { return byName(a.Name, b.Name) })
	return out
}

type TVBoardInput struct {
	Name             string   `json:"name"`
	CIIDs            []string `json:"ci_ids"`
	ServiceIDs       []string `json:"service_ids"`
	TeamIDs          []string `json:"team_ids"`
	Severities       []string `json:"severities"`
	ShowAcknowledged bool     `json:"show_acknowledged"`
	ShowMaintenance  bool     `json:"show_maintenance"`
	AllowedSources   []string `json:"allowed_sources"`
	Refresh          int      `json:"refresh"`
	Locale           string   `json:"locale"`
	Timezone         string   `json:"timezone"`
	Theme            string   `json:"theme"`
	Disabled         bool     `json:"disabled"`
}

func (s *TVBoardsService) check(d *store.Data, in *TVBoardInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.CIIDs, in.ServiceIDs, in.TeamIDs = uniq(in.CIIDs), uniq(in.ServiceIDs), uniq(in.TeamIDs)
	in.Locale, in.Timezone, in.Theme = strings.TrimSpace(in.Locale), strings.TrimSpace(in.Timezone), strings.TrimSpace(in.Theme)
	if in.Refresh == 0 {
		in.Refresh = defaultTVRefresh
	}
	if in.Theme == "" {
		in.Theme = model.ThemeDark
	}
	sev := []string{}
	for _, v := range uniq(in.Severities) {
		if alert.SeverityRank(v) == 0 {
			return invalid("severity_invalid", fmt.Errorf("unknown severity %q", v))
		}
		sev = append(sev, v)
	}
	slices.SortFunc(sev, func(a, b string) int { return cmp.Compare(alert.SeverityRank(b), alert.SeverityRank(a)) })
	in.Severities = sev
	switch {
	case in.Name == "" || len([]rune(in.Name)) > maxTVName:
		return invalid("name_invalid", nil)
	case len(in.CIIDs)+len(in.ServiceIDs)+len(in.TeamIDs) > maxTVTargets:
		return invalid("too_many_targets", nil)
	case in.Refresh < minTVRefresh || in.Refresh > maxTVRefresh:
		return invalid("refresh_invalid", nil)
	case in.Locale != "" && !model.ValidLocale(in.Locale):
		return invalid("invalid_locale", nil)
	case in.Timezone != "" && !model.ValidTimezone(in.Timezone):
		return invalid("invalid_timezone", nil)
	case !model.ValidTheme(in.Theme):
		return invalid("invalid_theme", nil)
	}
	_, sources, err := netacl.Parse(in.AllowedSources)
	if err != nil {
		return invalid("source_invalid", err)
	}
	switch {
	case len(sources) == 0:
		return invalid("sources_required", nil)
	case len(sources) > maxTVSources:
		return invalid("too_many_sources", nil)
	}
	in.AllowedSources = sources
	for _, id := range in.CIIDs {
		if d.ConfigItems[id] == nil {
			return invalid("ci_not_found", fmt.Errorf("configuration item %s does not exist", id))
		}
	}
	for _, id := range in.ServiceIDs {
		if d.Services[id] == nil {
			return invalid("service_not_found", fmt.Errorf("service %s does not exist", id))
		}
	}
	for _, id := range in.TeamIDs {
		if d.Teams[id] == nil {
			return invalid("team_not_found", fmt.Errorf("team %s does not exist", id))
		}
	}
	return nil
}

func (in TVBoardInput) apply(b *model.TVBoard) {
	b.Name, b.CIIDs, b.ServiceIDs, b.TeamIDs, b.Severities = in.Name, in.CIIDs, in.ServiceIDs, in.TeamIDs, in.Severities
	b.ShowAcknowledged, b.ShowMaintenance, b.AllowedSources, b.Refresh = in.ShowAcknowledged, in.ShowMaintenance, in.AllowedSources, in.Refresh
	b.Locale, b.Timezone, b.Theme, b.Disabled = in.Locale, in.Timezone, in.Theme, in.Disabled
}

func (s *TVBoardsService) Create(actor string, in TVBoardInput) (TVBoardView, error) {
	now := s.now()
	var out TVBoardView
	var err error
	s.st.Write(func(d *store.Data) {
		if len(d.TVBoards) >= maxTVBoards {
			err = invalid("too_many_boards", nil)
			return
		}
		if err = s.check(d, &in); err != nil {
			return
		}
		b := &model.TVBoard{ID: d.NextID("TV"), Slug: newSlug(), CreatedBy: actor, CreatedAt: now, UpdatedBy: actor, UpdatedAt: now}
		in.apply(b)
		d.TVBoards[b.ID] = b
		d.AddAudit(store.AuditEntry{Actor: actor, Action: "tv.create", Object: b.ID, Detail: b.Name})
		out = s.view(d, b)
	})
	return out, err
}

func (s *TVBoardsService) Update(actor, id string, in TVBoardInput) (TVBoardView, error) {
	now := s.now()
	var out TVBoardView
	err := ErrNotFound
	s.st.Write(func(d *store.Data) {
		b := d.TVBoards[id]
		if b == nil {
			return
		}
		if err = s.check(d, &in); err != nil {
			return
		}
		in.apply(b)
		b.UpdatedBy, b.UpdatedAt = actor, now
		d.AddAudit(store.AuditEntry{Actor: actor, Action: "tv.update", Object: b.ID, Detail: b.Name})
		out = s.view(d, b)
	})
	return out, err
}

// Rotate gives the board a new address; screens on the old one stop getting data.
func (s *TVBoardsService) Rotate(actor, id string) (TVBoardView, error) {
	now := s.now()
	var out TVBoardView
	err := ErrNotFound
	s.st.Write(func(d *store.Data) {
		b := d.TVBoards[id]
		if b == nil {
			return
		}
		err = nil
		b.Slug, b.UpdatedBy, b.UpdatedAt = newSlug(), actor, now
		d.AddAudit(store.AuditEntry{Actor: actor, Action: "tv.rotate", Object: b.ID, Detail: b.Name})
		out = s.view(d, b)
	})
	return out, err
}

func (s *TVBoardsService) Delete(actor, id string) error {
	err := ErrNotFound
	s.st.Write(func(d *store.Data) {
		if b := d.TVBoards[id]; b != nil {
			delete(d.TVBoards, id)
			d.AddAudit(store.AuditEntry{Actor: actor, Action: "tv.delete", Object: id, Detail: b.Name})
			err = nil
		}
	})
	s.mu.Lock()
	delete(s.seen, id)
	s.mu.Unlock()
	return err
}

func (s *TVBoardsService) Settings() model.TVSettings {
	var out model.TVSettings
	s.st.Read(func(d *store.Data) { out = d.Settings.TV })
	if out.TrustedProxies == nil {
		out.TrustedProxies = []string{}
	}
	return out
}

func (s *TVBoardsService) SaveSettings(actor string, in model.TVSettings) (model.TVSettings, error) {
	_, proxies, err := netacl.Parse(in.TrustedProxies)
	if err != nil {
		return model.TVSettings{}, invalid("proxy_invalid", err)
	}
	if len(proxies) > maxTrustedProxies {
		return model.TVSettings{}, invalid("too_many_proxies", nil)
	}
	s.st.Write(func(d *store.Data) {
		d.Settings.TV.TrustedProxies = proxies
		d.AddAudit(store.AuditEntry{Actor: actor, Action: "tv.settings", Detail: strings.Join(proxies, ", ")})
	})
	return model.TVSettings{TrustedProxies: proxies}, nil
}

func (s *TVBoardsService) trusted() netacl.List {
	var l netacl.List
	s.st.Read(func(d *store.Data) { l = netacl.MustParse(d.Settings.TV.TrustedProxies) })
	return l
}

// Targets finds configuration items, services and teams by name for the board editor.
func (s *TVBoardsService) Targets(q string) map[string][]TargetRef {
	q = strings.ToLower(strings.TrimSpace(q))
	cis, svcs, teams := []TargetRef{}, []TargetRef{}, []TargetRef{}
	s.st.Read(func(d *store.Data) {
		for _, ci := range d.ConfigItems {
			if q == "" || strings.Contains(strings.ToLower(ci.Name), q) || slices.ContainsFunc(ci.IPs, func(ip string) bool { return strings.HasPrefix(ip, q) }) {
				cis = append(cis, TargetRef{ID: ci.ID, Name: ci.Name})
			}
		}
		for _, svc := range d.Services {
			if svc.Status != model.ServiceRetired && (q == "" || strings.Contains(strings.ToLower(svc.Name), q)) {
				svcs = append(svcs, TargetRef{ID: svc.ID, Name: svc.Name})
			}
		}
		for _, t := range d.Teams {
			if q == "" || strings.Contains(strings.ToLower(t.Name), q) {
				teams = append(teams, TargetRef{ID: t.ID, Name: t.Name})
			}
		}
	})
	sortRefs := func(r []TargetRef) []TargetRef {
		slices.SortFunc(r, func(a, b TargetRef) int { return byName(a.Name, b.Name) })
		if len(r) > maxTargetResults {
			r = r[:maxTargetResults]
		}
		return r
	}
	return map[string][]TargetRef{"cis": sortRefs(cis), "services": sortRefs(svcs), "teams": sortRefs(teams)}
}

// board finds a board by its address for a screen; disabled boards are not found.
func (s *TVBoardsService) board(slug string) (model.TVBoard, model.Settings, bool) {
	var b model.TVBoard
	var set model.Settings
	found := false
	s.st.Read(func(d *store.Data) {
		set = d.Settings
		for _, x := range d.TVBoards {
			if !x.Disabled && x.Slug != "" && x.Slug == slug {
				b, found = *x, true
				return
			}
		}
	})
	return b, set, found
}

func (s *TVBoardsService) markSeen(id, ip string) {
	s.mu.Lock()
	s.seen[id] = tvSeen{At: s.now(), IP: ip}
	s.mu.Unlock()
}

// tvIncident is what a screen shows of an incident: no people, contacts or event payloads.
type tvIncident struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	CI          string    `json:"ci"`
	Severity    string    `json:"severity"`
	Status      string    `json:"status"`
	Services    []string  `json:"services"`
	Team        string    `json:"team,omitempty"`
	Maintenance bool      `json:"maintenance"`
	OpenedAt    time.Time `json:"opened_at"`
	LastSeen    time.Time `json:"last_seen"`
	Count       int       `json:"count"`
}

type tvView struct {
	Name        string         `json:"name"`
	Locale      string         `json:"locale"`
	Timezone    string         `json:"timezone"`
	Theme       string         `json:"theme"`
	Refresh     int            `json:"refresh"`
	Incidents   []tvIncident   `json:"incidents"`
	BySeverity  map[string]int `json:"by_severity"`
	More        bool           `json:"more"`
	GeneratedAt time.Time      `json:"generated_at"`
}

func tvFilter(b model.TVBoard) alert.Filter {
	f := alert.Filter{Status: "active", Severities: b.Severities, HideSuppressed: !b.ShowMaintenance, Limit: maxTVIncidents,
		Scope: alert.Scope{CIIDs: b.CIIDs, ServiceIDs: b.ServiceIDs, TeamIDs: b.TeamIDs}}
	if !b.ShowAcknowledged {
		f.Status = alert.StatusOpen
	}
	return f
}

// sortTV puts the most severe incidents first and, within a severity, the newest first.
func sortTV(list []tvIncident) {
	slices.SortStableFunc(list, func(a, b tvIncident) int {
		if c := cmp.Compare(alert.SeverityRank(b.Severity), alert.SeverityRank(a.Severity)); c != 0 {
			return c
		}
		if c := b.OpenedAt.Compare(a.OpenedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
}

func tvIncidentOf(al alert.Alert) tvIncident {
	inc := tvIncident{ID: al.ID, Title: al.Title, CI: al.CIName, Severity: al.Severity, Status: al.Status, Services: []string{},
		Maintenance: al.Suppressed, OpenedAt: al.OpenedAt, LastSeen: al.LastSeen, Count: al.Count}
	for _, s := range al.Route.Services {
		inc.Services = append(inc.Services, s.Name)
	}
	if al.Route.Team != nil {
		inc.Team = al.Route.Team.Name
	}
	return inc
}

func (a *App) registerTVBoards(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tv-boards", a.authed(a.can("tv:view", a.listTVBoards)))
	mux.HandleFunc("POST /api/tv-boards", a.authed(a.can("tv:edit", a.createTVBoard)))
	mux.HandleFunc("GET /api/tv-boards/targets", a.authed(a.can("tv:edit", a.tvTargets)))
	mux.HandleFunc("GET /api/tv-boards/settings", a.authed(a.can("tv:view", a.tvSettings)))
	mux.HandleFunc("PUT /api/tv-boards/settings", a.authed(a.can("tv:edit", a.saveTVSettings)))
	mux.HandleFunc("PUT /api/tv-boards/{id}", a.authed(a.can("tv:edit", a.updateTVBoard)))
	mux.HandleFunc("POST /api/tv-boards/{id}/rotate", a.authed(a.can("tv:edit", a.rotateTVBoard)))
	mux.HandleFunc("DELETE /api/tv-boards/{id}", a.authed(a.can("tv:edit", a.deleteTVBoard)))
	// The screen itself: no sign-in, only from the networks of the board.
	mux.HandleFunc("GET /api/tv/{slug}", a.tvData)
}

type tvBoardsPage struct {
	Boards []TVBoardView `json:"boards"`
	// YourIP is the address this request came from as the boards see it, to help fill in the
	// allowed networks.
	YourIP string `json:"your_ip"`
}

func (a *App) listTVBoards(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, tvBoardsPage{Boards: a.tv.List(), YourIP: netacl.ClientIP(r, a.tv.trusted()).String()})
}

func (a *App) tvTargets(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, a.tv.Targets(r.URL.Query().Get("q")))
}

func (a *App) tvSettings(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, a.tv.Settings())
}

func (a *App) saveTVSettings(w http.ResponseWriter, r *http.Request) {
	var in model.TVSettings
	if !httpx.Decode(w, r, &in) {
		return
	}
	out, err := a.tv.SaveSettings(current(r).user.Username, in)
	settingsRespond(w, out, err)
}

func (a *App) createTVBoard(w http.ResponseWriter, r *http.Request) {
	var in TVBoardInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	out, err := a.tv.Create(current(r).user.Username, in)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (a *App) updateTVBoard(w http.ResponseWriter, r *http.Request) {
	var in TVBoardInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	out, err := a.tv.Update(current(r).user.Username, r.PathValue("id"), in)
	settingsRespond(w, out, err)
}

func (a *App) rotateTVBoard(w http.ResponseWriter, r *http.Request) {
	out, err := a.tv.Rotate(current(r).user.Username, r.PathValue("id"))
	settingsRespond(w, out, err)
}

func (a *App) deleteTVBoard(w http.ResponseWriter, r *http.Request) {
	if err := a.tv.Delete(current(r).user.Username, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) tvData(w http.ResponseWriter, r *http.Request) {
	b, set, ok := a.tv.board(r.PathValue("slug"))
	if !ok {
		httpx.Error(w, http.StatusNotFound, "board_not_found", nil)
		return
	}
	ip := netacl.ClientIP(r, netacl.MustParse(set.TV.TrustedProxies))
	if !netacl.MustParse(b.AllowedSources).Contains(ip) {
		// The address is the caller's own, so naming it helps whoever sets the screen up.
		httpx.JSON(w, http.StatusForbidden, map[string]string{"error": "address_not_allowed", "ip": ip.String()})
		return
	}
	a.tv.markSeen(b.ID, ip.String())
	if !a.alertsReady(w) {
		return
	}
	page, err := a.alerts.List(r.Context(), tvFilter(b))
	if err != nil {
		writeError(w, err)
		return
	}
	out := tvView{Name: b.Name, Locale: cmp.Or(b.Locale, set.DefaultLocale, "en"), Timezone: cmp.Or(b.Timezone, DefaultTimezone(set)),
		Theme: cmp.Or(b.Theme, model.ThemeDark), Refresh: cmp.Or(b.Refresh, defaultTVRefresh), Incidents: []tvIncident{},
		BySeverity: map[string]int{}, More: page.More, GeneratedAt: a.tv.now()}
	for _, al := range page.Alerts {
		inc := tvIncidentOf(al)
		out.Incidents = append(out.Incidents, inc)
		out.BySeverity[inc.Severity]++
	}
	sortTV(out.Incidents)
	httpx.JSON(w, http.StatusOK, out)
}
