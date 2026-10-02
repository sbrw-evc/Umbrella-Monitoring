// Package api is the Core API: REST + WebSocket for the web UI, the ingest
// endpoint for push sources, the PagerDuty webhook and the Grafana link.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/connector"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pagerduty"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pipeline"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// Config of the API.
type Config struct {
	WebDir        string // built web UI (index.html); empty disables static
	GrafanaURL    string // base URL of Grafana for /go/incidents/{id}/grafana
	WebhookSecret string // PagerDuty Webhooks v3 signing secret
	SecureCookies bool   // always mark the session cookie Secure (behind TLS)
	MetricsToken  string // bearer token for /metrics; empty = open
	// AllowHTTPWebhooks accepts http:// notification webhooks (lab only).
	AllowHTTPWebhooks bool
}

// Server wires handlers.
type Server struct {
	cfg     Config
	st      *store.Store
	eng     *alert.Engine
	rt      *connector.Runtime
	pd      *pagerduty.Gateway
	hub      *Hub
	notify   Notifier
	sessions *auth.Sessions
	limiter  *ipLimiter
	started  time.Time
}

// Notifier sends test messages to notification channels.
type Notifier interface {
	Test(ch model.Channel, actor string) model.Delivery
}

// New creates the server.
func New(cfg Config, st *store.Store, eng *alert.Engine, rt *connector.Runtime, pd *pagerduty.Gateway, hub *Hub) *Server {
	return &Server{cfg: cfg, st: st, eng: eng, rt: rt, pd: pd, hub: hub, sessions: auth.NewSessions(),
		limiter: &ipLimiter{hits: map[string][]time.Time{}}, started: time.Now()}
}

// SetNotifier wires the notification sender (test messages).
func (s *Server) SetNotifier(n Notifier) { s.notify = n }

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	// p registers a handler that needs a signed-in user with perm ("" = any).
	p := func(pattern, perm string, h http.HandlerFunc) { m.HandleFunc(pattern, s.require(perm, h)) }

	m.HandleFunc("POST /api/auth/login", s.authLogin)
	m.HandleFunc("POST /api/auth/token", s.authToken)
	p("GET /api/auth/me", "", s.authMe)
	p("POST /api/auth/logout", "", s.authLogout)
	p("POST /api/auth/password", "", s.authPassword)

	p("GET /api/meta", "", s.meta)
	p("GET /api/incidents", model.PermIncidentsView, s.listIncidents)
	p("GET /api/incidents/{id}", model.PermIncidentsView, s.getIncident)
	p("POST /api/incidents/{id}/{action}", model.PermIncidentsAct, s.actIncident)
	p("POST /api/incidents/bulk", model.PermIncidentsAct, s.bulkIncidents)
	p("GET /api/events", model.PermEventsView, s.listEvents)
	p("GET /api/parse-errors", model.PermEventsView, s.listParseErrors)
	p("GET /api/cis", model.PermCMDBView, s.listCIs)
	p("POST /api/cis", model.PermCMDBEdit, s.createCI)
	p("GET /api/cis/{id}", model.PermCMDBView, s.getCI)
	p("GET /api/cmdb/graph", model.PermCMDBView, s.graph)
	p("GET /api/heatmap", model.PermIncidentsView, s.heatmap)
	p("GET /api/blocks", model.PermConnectorsView, s.blocks)
	p("GET /api/connectors", model.PermConnectorsView, s.listConnectors)
	p("POST /api/connectors", model.PermConnectorsEdit, s.createConnector)
	p("GET /api/connectors/{id}", model.PermConnectorsView, s.getConnector)
	p("PUT /api/connectors/{id}", model.PermConnectorsEdit, s.updateConnector)
	p("DELETE /api/connectors/{id}", model.PermConnectorsEdit, s.deleteConnector)
	p("POST /api/connectors/{id}/dry-run", model.PermConnectorsEdit, s.dryRun)
	p("POST /api/connectors/{id}/publish", model.PermConnectorsEdit, s.publish)
	p("POST /api/connectors/{id}/start", model.PermConnectorsEdit, s.setConnectorStatus(model.ConnectorRunning))
	p("POST /api/connectors/{id}/stop", model.PermConnectorsEdit, s.setConnectorStatus(model.ConnectorStopped))
	p("GET /api/maintenance", model.PermIncidentsView, s.listMaintenance)
	p("POST /api/maintenance", model.PermMaintenanceEdit, s.createMaintenance)
	p("DELETE /api/maintenance/{id}", model.PermMaintenanceEdit, s.deleteMaintenance)
	p("GET /api/rules", model.PermRulesView, s.listRules)
	p("GET /api/audit", model.PermAuditView, s.listAudit)
	p("GET /api/selfcheck", model.PermSelfcheckView, s.selfcheck)
	p("POST /api/selfcheck/pd-outage", model.PermSelfcheckAdmin, s.pdOutage)

	p("GET /api/users", model.PermUsersAdmin, s.listUsers)
	p("POST /api/users", model.PermUsersAdmin, s.createUser)
	p("PUT /api/users/{id}", model.PermUsersAdmin, s.updateUser)
	p("DELETE /api/users/{id}", model.PermUsersAdmin, s.deleteUser)
	p("POST /api/users/{id}/password", model.PermUsersAdmin, s.resetPassword)
	p("GET /api/users/{id}/tokens", "", s.listTokens)
	p("POST /api/users/{id}/tokens", "", s.createToken)
	p("DELETE /api/users/{id}/tokens/{tid}", "", s.deleteToken)
	p("GET /api/roles", model.PermUsersAdmin, s.listRoles)
	p("POST /api/roles", model.PermUsersAdmin, s.createRole)
	p("PUT /api/roles/{id}", model.PermUsersAdmin, s.updateRole)
	p("DELETE /api/roles/{id}", model.PermUsersAdmin, s.deleteRole)

	p("GET /api/channels", model.PermNotifyEdit, s.listChannels)
	p("POST /api/channels", model.PermNotifyEdit, s.createChannel)
	p("PUT /api/channels/{id}", model.PermNotifyEdit, s.updateChannel)
	p("DELETE /api/channels/{id}", model.PermNotifyEdit, s.deleteChannel)
	p("POST /api/channels/{id}/test", model.PermNotifyEdit, s.testChannel)
	p("GET /api/deliveries", model.PermNotifyEdit, s.listDeliveries)

	p("GET /api/ws", "", s.serveWS)
	p("GET /go/incidents/{id}/grafana", model.PermIncidentsView, s.grafana)

	// Machine endpoints with their own authentication.
	m.HandleFunc("POST /api/ingest/{id}", s.ingest)
	m.HandleFunc("POST /api/pagerduty/webhook", s.pdWebhook)
	m.HandleFunc("GET /metrics", s.metrics)
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	if s.cfg.WebDir != "" {
		m.Handle("/", spa(s.cfg.WebDir))
	}
	return m
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 5<<20))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("неверный JSON: %v", err)
	}
	return nil
}

// actor is the signed-in user name for timelines and the audit log.
func actor(r *http.Request) string { return me(r).Name() }

func splitList(v string) []string {
	if v == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// ---- meta ----

func (s *Server) meta(w http.ResponseWriter, _ *http.Request) {
	// Teams come from the seed and from the CMDB, so a clean install without
	// demo data still gets its team list from the CIs people create.
	teams := []model.Team{}
	s.st.Read(func(d *store.Data) {
		seen := map[string]bool{}
		for _, t := range d.Teams {
			seen[t.ID] = true
			teams = append(teams, t)
		}
		var extra []string
		for _, ci := range d.CIs {
			if ci.Team != "" && !seen[ci.Team] {
				seen[ci.Team] = true
				extra = append(extra, ci.Team)
			}
		}
		sort.Strings(extra)
		for _, id := range extra {
			teams = append(teams, model.Team{ID: id, Name: id})
		}
	})
	writeJSON(w, 200, map[string]any{
		"teams":      teams,
		"severities": []model.Severity{model.SevCritical, model.SevError, model.SevWarning, model.SevInfo},
		"grafana":    s.cfg.GrafanaURL != "",
		"pd_mode":    s.pd.Status().Mode,
		"version":    "0.2.0-mvp",
	})
}

// ---- incidents ----

// IncidentQuery filters the incident dashboard. The same fields go into the
// URL so a view can be shared by link.
type IncidentQuery struct {
	View     string // open, closed, all
	Severity []string
	Status   []string
	Team     string
	Service  string
	CI       string
	Method   []string
	PD       []string
	Fallback string
	Q        string
	Hours    int
	Sort     string
	Desc     bool
}

func parseQuery(v url.Values) IncidentQuery {
	q := IncidentQuery{
		View: v.Get("view"), Severity: splitList(v.Get("severity")), Status: splitList(v.Get("status")),
		Team: v.Get("team"), Service: v.Get("service"), CI: v.Get("ci"), Method: splitList(v.Get("method")),
		PD: splitList(v.Get("pd")), Fallback: v.Get("fallback"), Q: strings.ToLower(strings.TrimSpace(v.Get("q"))),
		Sort: v.Get("sort"), Desc: v.Get("order") != "asc",
	}
	q.Hours, _ = strconv.Atoi(v.Get("hours"))
	if q.View == "" {
		q.View = "open"
	}
	return q
}

func (q IncidentQuery) match(a *model.Alert, now time.Time, eventText map[string]string) bool {
	switch q.View {
	case "open":
		if !a.Status.Active() {
			return false
		}
	case "closed":
		if a.Status.Active() {
			return false
		}
	}
	if len(q.Severity) > 0 && !contains(q.Severity, string(a.Severity)) {
		return false
	}
	if len(q.Status) > 0 && !contains(q.Status, string(a.Status)) {
		return false
	}
	if q.Team != "" && q.Team != "all" && a.Team != q.Team {
		return false
	}
	if q.Service != "" && a.Service != q.Service {
		return false
	}
	if q.CI != "" && a.CIID != q.CI {
		return false
	}
	if len(q.Method) > 0 && !contains(q.Method, string(a.Method)) {
		return false
	}
	if len(q.PD) > 0 && !contains(q.PD, string(a.PDState)) {
		return false
	}
	if q.Fallback == "yes" && !a.Fallback || q.Fallback == "no" && a.Fallback {
		return false
	}
	if q.Hours > 0 && now.Sub(a.LastSeen) > time.Duration(q.Hours)*time.Hour {
		return false
	}
	if q.Q != "" {
		hay := strings.ToLower(strings.Join([]string{a.ID, a.Title, a.CIName, a.Service, a.Signal, a.Team, eventText[a.ID]}, " "))
		for _, word := range strings.Fields(q.Q) {
			if !strings.Contains(hay, word) {
				return false
			}
		}
	}
	return true
}

func sortAlerts(list []model.Alert, by string, desc bool) {
	less := func(i, j int) bool { return list[i].LastSeen.Before(list[j].LastSeen) }
	switch by {
	case "severity":
		less = func(i, j int) bool {
			if list[i].Severity.Rank() == list[j].Severity.Rank() {
				return list[i].LastSeen.Before(list[j].LastSeen)
			}
			return list[i].Severity.Rank() < list[j].Severity.Rank()
		}
	case "first_seen":
		less = func(i, j int) bool { return list[i].FirstSeen.Before(list[j].FirstSeen) }
	case "count":
		less = func(i, j int) bool { return list[i].Count < list[j].Count }
	case "ci":
		less = func(i, j int) bool { return list[i].CIName < list[j].CIName }
	case "service":
		less = func(i, j int) bool { return list[i].Service < list[j].Service }
	case "id":
		less = func(i, j int) bool { return idNum(list[i].ID) < idNum(list[j].ID) }
	}
	sort.SliceStable(list, func(i, j int) bool {
		if desc {
			return less(j, i)
		}
		return less(i, j)
	})
}

func idNum(id string) int {
	_, n, _ := strings.Cut(id, "-")
	v, _ := strconv.Atoi(n)
	return v
}

func (s *Server) listIncidents(w http.ResponseWriter, r *http.Request) {
	q := parseQuery(r.URL.Query())
	now := time.Now()
	var items []model.Alert
	counts := map[string]int{}
	hourly := make([]map[string]int, 24)
	for i := range hourly {
		hourly[i] = map[string]int{}
	}
	s.st.Read(func(d *store.Data) {
		var text map[string]string
		if q.Q != "" {
			// Full-text over event titles within 30 days (PostgreSQL FTS in
			// the database-backed store).
			text = map[string]string{}
			for _, ev := range d.Events {
				if ev.AlertID != "" && now.Sub(ev.ReceivedAt) <= 30*24*time.Hour {
					text[ev.AlertID] += " " + ev.Title + " " + ev.Source
				}
			}
		}
		p := me(r)
		for _, a := range d.Alerts {
			if !p.SeesCI(a.CIID) {
				continue
			}
			// Indicator tiles count active incidents in the team scope,
			// independent of the other filters (like operational tiles).
			if a.Status.Active() && (q.Team == "" || q.Team == "all" || a.Team == q.Team) {
				counts[string(a.Severity)]++
				counts["total"]++
				if a.PDState == model.PDFailed || a.PDState == model.PDPending {
					counts["pd_not_accepted"]++
				}
			}
			if (q.Team == "" || q.Team == "all" || a.Team == q.Team) && now.Sub(a.FirstSeen) < 24*time.Hour {
				h := 23 - int(now.Sub(a.FirstSeen)/time.Hour)
				hourly[h][string(a.Severity)]++
			}
			if q.match(a, now, text) {
				c := alert.Clone(a)
				c.Timeline = nil
				items = append(items, c)
			}
		}
	})
	sortAlerts(items, q.Sort, q.Desc)
	total := len(items)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	if len(items) > limit {
		items = items[:limit]
	}
	if items == nil {
		items = []model.Alert{}
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "counts": counts, "hourly": hourly})
}

func (s *Server) getIncident(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, ok := s.eng.Get(id)
	if !ok || !me(r).SeesCI(a.CIID) {
		writeErr(w, 404, alert.ErrNotFound)
		return
	}
	var events []model.Event
	var related *model.Alert
	s.st.Read(func(d *store.Data) {
		for i := len(d.Events) - 1; i >= 0 && len(events) < 200; i-- {
			if d.Events[i].AlertID == id {
				events = append(events, *d.Events[i])
			}
		}
		if a.RelatedID != "" {
			if o := d.Alerts[a.RelatedID]; o != nil && me(r).SeesCI(o.CIID) {
				c := alert.Clone(o)
				c.Timeline = nil
				related = &c
			}
		}
	})
	if events == nil {
		events = []model.Event{}
	}
	writeJSON(w, 200, map[string]any{"incident": a, "events": events, "related": related,
		"grafana_url": "/go/incidents/" + id + "/grafana"})
}

func (s *Server) actIncident(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if r.ContentLength > 0 {
		if err := readJSON(r, &body); err != nil {
			writeErr(w, 400, err)
			return
		}
	}
	if cur, ok := s.eng.Get(r.PathValue("id")); !ok || !me(r).SeesCI(cur.CIID) {
		writeErr(w, 404, alert.ErrNotFound)
		return
	}
	a, err := s.eng.Act(r.PathValue("id"), r.PathValue("action"), actor(r), body.Text)
	if errors.Is(err, alert.ErrNotFound) {
		writeErr(w, 404, err)
		return
	}
	if err != nil {
		writeErr(w, 409, err)
		return
	}
	writeJSON(w, 200, a)
}

func (s *Server) bulkIncidents(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if body.Action != "ack" && body.Action != "resolve" {
		writeErr(w, 400, errors.New("групповое действие: ack или resolve"))
		return
	}
	done, failed := 0, map[string]string{}
	for _, id := range body.IDs {
		if cur, ok := s.eng.Get(id); !ok || !me(r).SeesCI(cur.CIID) {
			failed[id] = alert.ErrNotFound.Error()
			continue
		}
		if _, err := s.eng.Act(id, body.Action, actor(r), ""); err != nil {
			failed[id] = err.Error()
		} else {
			done++
		}
	}
	writeJSON(w, 200, map[string]any{"done": done, "failed": failed})
}

// ---- events and parse errors ----

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 2000 {
		limit = 300
	}
	conn := r.URL.Query().Get("connector")
	q := strings.ToLower(r.URL.Query().Get("q"))
	sev := splitList(r.URL.Query().Get("severity"))
	out := []model.Event{}
	s.st.Read(func(d *store.Data) {
		for i := len(d.Events) - 1; i >= 0 && len(out) < limit; i-- {
			ev := d.Events[i]
			if !me(r).SeesCI(ev.CIID) {
				continue
			}
			if conn != "" && ev.ConnectorID != conn {
				continue
			}
			if len(sev) > 0 && !contains(sev, string(ev.Severity)) {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(ev.Title+" "+ev.CIName+" "+ev.Signal+" "+ev.Source+" "+ev.ID), q) {
				continue
			}
			out = append(out, *ev)
		}
	})
	writeJSON(w, 200, map[string]any{"items": out})
}

func (s *Server) listParseErrors(w http.ResponseWriter, r *http.Request) {
	conn := r.URL.Query().Get("connector")
	out := []model.ParseError{}
	if !me(r).AllServices {
		writeJSON(w, 200, map[string]any{"items": out})
		return
	}
	s.st.Read(func(d *store.Data) {
		for i := len(d.ParseErrors) - 1; i >= 0 && len(out) < 500; i-- {
			if conn == "" || d.ParseErrors[i].ConnectorID == conn {
				out = append(out, *d.ParseErrors[i])
			}
		}
	})
	writeJSON(w, 200, map[string]any{"items": out})
}

// ---- CMDB ----

// CIView is a CI with its computed status for tables and the graph.
type CIView struct {
	model.CI
	Status      model.Severity `json:"status"`        // worst active alert on the CI or below it; "" = ok
	OwnStatus   model.Severity `json:"own_status"`    // worst active alert on the CI itself
	OpenAlerts  int            `json:"open_alerts"`   // active alerts on the CI itself
	Maintenance bool           `json:"maintenance"`   // an active maintenance window
	Children    int            `json:"children"`
	Parents     int            `json:"parents"`
}

func ciViews(d *store.Data, now time.Time) map[string]*CIView {
	views := map[string]*CIView{}
	for id, ci := range d.CIs {
		c := *ci
		views[id] = &CIView{CI: c}
	}
	for _, a := range d.Alerts {
		if !a.Status.Active() || a.CIID == "" || a.Suppressed {
			continue
		}
		if v := views[a.CIID]; v != nil {
			v.OpenAlerts++
			v.OwnStatus = model.MaxSeverity(v.OwnStatus, a.Severity)
		}
	}
	down := map[string][]string{}
	for _, r := range d.Relations {
		down[r.From] = append(down[r.From], r.To)
		if v := views[r.From]; v != nil {
			v.Children++
		}
		if v := views[r.To]; v != nil {
			v.Parents++
		}
	}
	for _, m := range d.Maintenance {
		if v := views[m.CIID]; v != nil && m.State(now) == "active" {
			v.Maintenance = true
		}
	}
	// Status propagates up: a service is as bad as the worst CI under it.
	var walk func(id string, seen map[string]bool) model.Severity
	walk = func(id string, seen map[string]bool) model.Severity {
		if seen[id] {
			return ""
		}
		seen[id] = true
		v := views[id]
		if v == nil {
			return ""
		}
		st := v.OwnStatus
		for _, c := range down[id] {
			st = model.MaxSeverity(st, walk(c, seen))
		}
		return st
	}
	for id, v := range views {
		v.Status = walk(id, map[string]bool{})
	}
	return views
}

func (s *Server) listCIs(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	types := splitList(r.URL.Query().Get("type"))
	state := r.URL.Query().Get("state") // problem, ok
	team := r.URL.Query().Get("team")
	out := []CIView{}
	s.st.Read(func(d *store.Data) {
		for _, v := range ciViews(d, time.Now()) {
			if !me(r).SeesCI(v.ID) {
				continue
			}
			if len(types) > 0 && !contains(types, v.Type) {
				continue
			}
			if team != "" && team != "all" && v.Team != team {
				continue
			}
			if state == "problem" && v.Status == "" || state == "ok" && v.Status != "" {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(v.ID+" "+v.Name+" "+v.Description+" "+v.LogicalGroup), q) {
				continue
			}
			out = append(out, *v)
		}
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].Status.Rank() != out[j].Status.Rank() {
			return out[i].Status.Rank() > out[j].Status.Rank()
		}
		return out[i].Name < out[j].Name
	})
	writeJSON(w, 200, map[string]any{"items": out})
}

func (s *Server) createCI(w http.ResponseWriter, r *http.Request) {
	var body struct {
		model.CI
		Parent string `json:"parent"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if strings.TrimSpace(body.Name) == "" || body.Type == "" {
		writeErr(w, 400, errors.New("нужны название и тип КЕ"))
		return
	}
	var ci model.CI
	s.st.Write(func(d *store.Data) {
		ci = body.CI
		ci.ID = d.NextID("CI")
		ci.Origin = "manual"
		ci.CreatedAt = time.Now()
		if ci.Identities == nil {
			ci.Identities = []model.Identity{}
		}
		d.CIs[ci.ID] = &ci
		if body.Parent != "" && d.CIs[body.Parent] != nil {
			d.Relations = append(d.Relations, model.Relation{From: body.Parent, To: ci.ID, Type: "depends_on"})
		}
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: "ci.create", Object: ci.ID})
	})
	writeJSON(w, 201, ci)
}

func (s *Server) getCI(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var resp map[string]any
	s.st.Read(func(d *store.Data) {
		views := ciViews(d, time.Now())
		v := views[id]
		if v == nil || !me(r).SeesCI(id) {
			return
		}
		for k := range views {
			if !me(r).SeesCI(k) {
				delete(views, k)
			}
		}
		type rel struct {
			model.Relation
			Dir    string         `json:"dir"`
			Other  string         `json:"other"`
			Name   string         `json:"name"`
			Type   string         `json:"ci_type"`
			Status model.Severity `json:"status"`
		}
		rels := []rel{}
		for _, r := range d.Relations {
			if r.From == id && views[r.To] != nil {
				o := views[r.To]
				rels = append(rels, rel{Relation: r, Dir: "down", Other: o.ID, Name: o.Name, Type: o.Type, Status: o.Status})
			}
			if r.To == id && views[r.From] != nil {
				o := views[r.From]
				rels = append(rels, rel{Relation: r, Dir: "up", Other: o.ID, Name: o.Name, Type: o.Type, Status: o.Status})
			}
		}
		alerts := []model.Alert{}
		for _, a := range d.Alerts {
			if a.CIID == id {
				c := alert.Clone(a)
				c.Timeline = nil
				alerts = append(alerts, c)
			}
		}
		sortAlerts(alerts, "", true)
		maint := []map[string]any{}
		for _, m := range d.Maintenance {
			if m.CIID == id {
				maint = append(maint, map[string]any{"maintenance": m, "state": m.State(time.Now())})
			}
		}
		svc := alert.ServiceOf(d, id)
		var svcName string
		if svc != nil {
			svcName = svc.Name
		}
		resp = map[string]any{"ci": v, "relations": rels, "alerts": alerts, "maintenance": maint, "service": svcName}
	})
	if resp == nil {
		writeErr(w, 404, errors.New("КЕ не найдена"))
		return
	}
	writeJSON(w, 200, resp)
}

func (s *Server) graph(w http.ResponseWriter, r *http.Request) {
	root := r.URL.Query().Get("root")
	depth, _ := strconv.Atoi(r.URL.Query().Get("depth"))
	if depth <= 0 {
		depth = 3
	}
	var nodes []CIView
	var edges []model.Relation
	s.st.Read(func(d *store.Data) {
		views := ciViews(d, time.Now())
		include := map[string]bool{}
		if root == "" {
			for id := range views {
				include[id] = true
			}
		} else {
			// Up to services and down to resources, each to the depth.
			include[root] = true
			frontier := []string{root}
			for i := 0; i < depth; i++ {
				var next []string
				for _, id := range frontier {
					for _, rel := range d.Relations {
						if rel.From == id && !include[rel.To] {
							include[rel.To] = true
							next = append(next, rel.To)
						}
					}
				}
				frontier = next
			}
			frontier = []string{root}
			for i := 0; i < depth; i++ {
				var next []string
				for _, id := range frontier {
					for _, rel := range d.Relations {
						if rel.To == id && !include[rel.From] {
							include[rel.From] = true
							next = append(next, rel.From)
						}
					}
				}
				frontier = next
			}
		}
		for id := range include {
			if !me(r).SeesCI(id) {
				delete(include, id)
			}
		}
		for id := range include {
			if v := views[id]; v != nil {
				nodes = append(nodes, *v)
			}
		}
		for _, rel := range d.Relations {
			if include[rel.From] && include[rel.To] {
				edges = append(edges, rel)
			}
		}
	})
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	if nodes == nil {
		nodes = []CIView{}
	}
	if edges == nil {
		edges = []model.Relation{}
	}
	writeJSON(w, 200, map[string]any{"nodes": nodes, "edges": edges})
}

// ---- connectors ----

func (s *Server) blocks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"items": pipeline.Blocks, "categories": []map[string]string{
		{"id": "trigger", "title": "Триггеры"}, {"id": "fetch", "title": "Получение"}, {"id": "parse", "title": "Парсинг"},
		{"id": "transform", "title": "Преобразование"}, {"id": "ack", "title": "Подтверждение"}, {"id": "output", "title": "Выход"},
	}})
}

func (s *Server) listConnectors(w http.ResponseWriter, r *http.Request) {
	team := r.URL.Query().Get("team")
	out := []model.Connector{}
	s.st.Read(func(d *store.Data) {
		for _, c := range d.Connectors {
			if team != "" && team != "all" && c.Team != team {
				continue
			}
			cc := *c
			cc.Draft = model.Graph{}
			cc.Published = nil
			out = append(out, cc)
		}
	})
	sort.Slice(out, func(i, j int) bool { return idNum(out[i].ID) < idNum(out[j].ID) })
	writeJSON(w, 200, map[string]any{"items": out})
}

func (s *Server) getConnector(w http.ResponseWriter, r *http.Request) {
	var c *model.Connector
	s.st.Read(func(d *store.Data) {
		if p := d.Connectors[r.PathValue("id")]; p != nil {
			cc := *p
			c = &cc
		}
	})
	if c == nil {
		writeErr(w, 404, connector.ErrNotFound)
		return
	}
	writeJSON(w, 200, map[string]any{"connector": c, "ingest_url": "/api/ingest/" + c.ID})
}

// Templates offered when creating a connector.
func starterGraph(kind string) model.Graph {
	switch kind {
	case "pull-http":
		return model.Graph{
			Nodes: []model.Node{
				{ID: "n1", Kind: "trigger.schedule", X: 40, Y: 140, Config: map[string]string{"interval": "60s"}},
				{ID: "n2", Kind: "fetch.http", X: 280, Y: 140, Config: map[string]string{"method": "GET"}},
				{ID: "n3", Kind: "parse.json", X: 520, Y: 140, Config: map[string]string{}},
				{ID: "n4", Kind: "map.event", X: 40, Y: 320, Config: map[string]string{"title": "${title}", "ci": "${host}", "signal": "${signal|generic}", "severity": "${severity}", "status": "${status}", "external_id": "${id}", "method": "other"}},
				{ID: "n5", Kind: "out.event", X: 300, Y: 320, Config: map[string]string{}},
				{ID: "n6", Kind: "ack.response", X: 560, Y: 320, Config: map[string]string{"mode": "cursor"}},
			},
			Edges: []model.Edge{{ID: "e1", Source: "n1", Target: "n2"}, {ID: "e2", Source: "n2", Target: "n3"}, {ID: "e3", Source: "n3", Target: "n4"}, {ID: "e4", Source: "n4", Target: "n5"}, {ID: "e5", Source: "n5", Target: "n6"}},
		}
	default:
		return model.Graph{
			Nodes: []model.Node{
				{ID: "n1", Kind: "trigger.webhook", X: 40, Y: 140, Config: map[string]string{"auth": "token"}},
				{ID: "n2", Kind: "parse.json", X: 280, Y: 140, Config: map[string]string{}},
				{ID: "n3", Kind: "map.event", X: 520, Y: 140, Config: map[string]string{"title": "${title}", "ci": "${host}", "signal": "${signal|generic}", "severity": "${severity}", "status": "${status}", "external_id": "${id}", "method": "other"}},
				{ID: "n4", Kind: "out.event", X: 40, Y: 320, Config: map[string]string{}},
				{ID: "n5", Kind: "ack.response", X: 300, Y: 320, Config: map[string]string{"mode": "http_2xx"}},
			},
			Edges: []model.Edge{{ID: "e1", Source: "n1", Target: "n2"}, {ID: "e2", Source: "n2", Target: "n3"}, {ID: "e3", Source: "n3", Target: "n4"}, {ID: "e4", Source: "n4", Target: "n5"}},
		}
	}
}

func (s *Server) createConnector(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		Team     string `json:"team"`
		Template string `json:"template"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeErr(w, 400, errors.New("нужно название коннектора"))
		return
	}
	var c model.Connector
	var err error
	s.st.Write(func(d *store.Data) {
		c = model.Connector{Name: body.Name, Team: body.Team, Status: model.ConnectorStopped,
			Draft: starterGraph(body.Template), DraftDirty: true, UpdatedAt: time.Now(), UpdatedBy: actor(r)}
		if err = setSlug(d, &c, body.Slug); err != nil {
			return
		}
		c.ID = d.NextID("CON")
		d.Connectors[c.ID] = &c
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: "connector.create", Object: c.ID})
	})
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 201, c)
}

func (s *Server) updateConnector(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        *string      `json:"name"`
		Slug        *string      `json:"slug"`
		Description *string      `json:"description"`
		Draft       *model.Graph `json:"draft"`
		SampleInput *string      `json:"sample_input"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	var c *model.Connector
	var slugErr error
	s.st.Write(func(d *store.Data) {
		p := d.Connectors[r.PathValue("id")]
		if p == nil {
			return
		}
		if body.Name != nil {
			p.Name = *body.Name
		}
		if body.Slug != nil {
			if slugErr = setSlug(d, p, *body.Slug); slugErr != nil {
				return
			}
		}
		if body.Description != nil {
			p.Description = *body.Description
		}
		if body.Draft != nil {
			p.Draft = *body.Draft
			p.DraftDirty = true
		}
		if body.SampleInput != nil {
			p.SampleInput = *body.SampleInput
		}
		p.UpdatedAt = time.Now()
		p.UpdatedBy = actor(r)
		cc := *p
		c = &cc
	})
	if slugErr != nil {
		writeErr(w, 400, slugErr)
		return
	}
	if c == nil {
		writeErr(w, 404, connector.ErrNotFound)
		return
	}
	writeJSON(w, 200, c)
}

func (s *Server) deleteConnector(w http.ResponseWriter, r *http.Request) {
	found := false
	s.st.Write(func(d *store.Data) {
		if _, ok := d.Connectors[r.PathValue("id")]; ok {
			delete(d.Connectors, r.PathValue("id"))
			found = true
			d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: "connector.delete", Object: r.PathValue("id")})
		}
	})
	if !found {
		writeErr(w, 404, connector.ErrNotFound)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) dryRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Graph  *model.Graph `json:"graph"`
		Sample string       `json:"sample"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	g := body.Graph
	if g == nil {
		s.st.Read(func(d *store.Data) {
			if p := d.Connectors[r.PathValue("id")]; p != nil {
				gg := p.Draft
				g = &gg
			}
		})
	}
	if g == nil {
		writeErr(w, 404, connector.ErrNotFound)
		return
	}
	writeJSON(w, 200, s.rt.DryRun(*g, body.Sample))
}

func (s *Server) publish(w http.ResponseWriter, r *http.Request) {
	var c *model.Connector
	var err error
	s.st.Write(func(d *store.Data) {
		p := d.Connectors[r.PathValue("id")]
		if p == nil {
			err = connector.ErrNotFound
			return
		}
		if _, err = pipeline.Validate(p.Draft); err != nil {
			return
		}
		g := p.Draft
		g.Nodes = append([]model.Node(nil), p.Draft.Nodes...)
		g.Edges = append([]model.Edge(nil), p.Draft.Edges...)
		p.Published = &g
		p.Version++
		p.DraftDirty = false
		p.UpdatedAt = time.Now()
		p.UpdatedBy = actor(r)
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: fmt.Sprintf("connector.publish v%d", p.Version), Object: p.ID})
		cc := *p
		c = &cc
	})
	if errors.Is(err, connector.ErrNotFound) {
		writeErr(w, 404, err)
		return
	}
	if err != nil {
		writeErr(w, 422, err)
		return
	}
	writeJSON(w, 200, c)
}

func (s *Server) setConnectorStatus(st model.ConnectorStatus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var err error
		s.st.Write(func(d *store.Data) {
			p := d.Connectors[r.PathValue("id")]
			if p == nil {
				err = connector.ErrNotFound
				return
			}
			if st == model.ConnectorRunning && p.Published == nil {
				err = connector.ErrNotPublished
				return
			}
			p.Status = st
			d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: "connector." + string(st), Object: p.ID})
		})
		if errors.Is(err, connector.ErrNotFound) {
			writeErr(w, 404, err)
			return
		}
		if err != nil {
			writeErr(w, 409, err)
			return
		}
		writeJSON(w, 200, map[string]string{"status": string(st)})
	}
}

// ---- maintenance, rules, audit ----

func (s *Server) listMaintenance(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	out := []map[string]any{}
	s.st.Read(func(d *store.Data) {
		for _, m := range d.Maintenance {
			if !me(r).SeesCI(m.CIID) {
				continue
			}
			out = append(out, map[string]any{"maintenance": m, "state": m.State(now)})
		}
	})
	sort.Slice(out, func(i, j int) bool {
		return out[i]["maintenance"].(*model.Maintenance).Start.After(out[j]["maintenance"].(*model.Maintenance).Start)
	})
	writeJSON(w, 200, map[string]any{"items": out})
}

func (s *Server) createMaintenance(w http.ResponseWriter, r *http.Request) {
	var body model.Maintenance
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if body.Title == "" || body.CIID == "" || !body.End.After(body.Start) {
		writeErr(w, 400, errors.New("нужны название, КЕ и окончание позже начала"))
		return
	}
	var err error
	s.st.Write(func(d *store.Data) {
		ci := d.CIs[body.CIID]
		if ci == nil || !me(r).SeesCI(ci.ID) {
			err = errors.New("КЕ не найдена")
			return
		}
		body.ID = d.NextID("MW")
		body.CIName = ci.Name
		body.Author = actor(r)
		body.CreatedAt = time.Now()
		m := body
		d.Maintenance[m.ID] = &m
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: "maintenance.create", Object: m.ID})
	})
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 201, body)
}

func (s *Server) deleteMaintenance(w http.ResponseWriter, r *http.Request) {
	s.st.Write(func(d *store.Data) {
		if m := d.Maintenance[r.PathValue("id")]; m == nil || !me(r).SeesCI(m.CIID) {
			return
		}
		delete(d.Maintenance, r.PathValue("id"))
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: "maintenance.delete", Object: r.PathValue("id")})
	})
	w.WriteHeader(204)
}

func (s *Server) listRules(w http.ResponseWriter, _ *http.Request) {
	var out []model.Rule
	s.st.Read(func(d *store.Data) { out = append(out, d.Rules...) })
	writeJSON(w, 200, map[string]any{"items": out})
}

func (s *Server) listAudit(w http.ResponseWriter, _ *http.Request) {
	out := []store.AuditEntry{}
	s.st.Read(func(d *store.Data) {
		for i := len(d.Audit) - 1; i >= 0 && len(out) < 300; i-- {
			out = append(out, d.Audit[i])
		}
	})
	writeJSON(w, 200, map[string]any{"items": out})
}

// ---- self-check ----

func (s *Server) selfcheck(w http.ResponseWriter, _ *http.Request) {
	var events, alerts, active, errs, conns, running int
	var lastEvent *time.Time
	s.st.Read(func(d *store.Data) {
		events = len(d.Events)
		alerts = len(d.Alerts)
		errs = len(d.ParseErrors)
		for _, a := range d.Alerts {
			if a.Status.Active() {
				active++
			}
		}
		for _, c := range d.Connectors {
			conns++
			if c.Status == model.ConnectorRunning {
				running++
			}
		}
		if n := len(d.Events); n > 0 {
			t := d.Events[n-1].ReceivedAt
			lastEvent = &t
		}
	})
	writeJSON(w, 200, map[string]any{
		"uptime_s": int(time.Since(s.started).Seconds()), "events": events, "alerts": alerts, "active_alerts": active,
		"parse_errors": errs, "connectors": conns, "connectors_running": running, "last_event_at": lastEvent,
		"pagerduty": s.pd.Status(), "store": "memory", "bus": "in-process",
	})
}

func (s *Server) pdOutage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On bool `json:"on"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	s.pd.SetOutage(body.On)
	s.st.Write(func(d *store.Data) {
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: fmt.Sprintf("selfcheck.pd_outage=%v", body.On), Object: "pagerduty"})
	})
	writeJSON(w, 200, s.pd.Status())
}

// ---- ingest, PagerDuty webhook, Grafana ----

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20))
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	token := r.Header.Get("X-Umbrella-Token")
	if v, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		token = v
	}
	n, err := s.rt.Webhook(s.connectorRef(r.PathValue("id")), string(body), token)
	switch {
	case errors.Is(err, connector.ErrNotFound):
		writeErr(w, 404, err)
	case errors.Is(err, connector.ErrUnauthorized):
		writeErr(w, 401, err)
	case errors.Is(err, connector.ErrStopped), errors.Is(err, connector.ErrNotPublished), errors.Is(err, connector.ErrNotWebhook):
		writeErr(w, 409, err)
	case err != nil:
		// Not acknowledged: the source keeps the event and retries.
		writeErr(w, 503, err)
	default:
		writeJSON(w, 202, map[string]int{"accepted": n})
	}
}

// connectorRef maps a slug (/api/ingest/zabbix) to the connector id.
func (s *Server) connectorRef(ref string) string {
	id := ref
	s.st.Read(func(d *store.Data) {
		if d.Connectors[ref] != nil {
			return
		}
		for _, c := range d.Connectors {
			if c.Slug != "" && c.Slug == ref {
				id = c.ID
			}
		}
	})
	return id
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,40}$`)

// setSlug validates a slug and checks it is free. Call inside a store write.
func setSlug(d *store.Data, c *model.Connector, slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		c.Slug = ""
		return nil
	}
	if !slugRe.MatchString(slug) || strings.HasPrefix(strings.ToUpper(slug), "CON-") {
		return errors.New("короткое имя: латиница в нижнем регистре, цифры и дефис")
	}
	for _, o := range d.Connectors {
		if o.ID != c.ID && o.Slug == slug {
			return errors.New("короткое имя уже занято коннектором " + o.ID)
		}
	}
	c.Slug = slug
	return nil
}

func (s *Server) pdWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	if !pagerduty.VerifySignature(s.cfg.WebhookSecret, body, r.Header.Get("X-PagerDuty-Signature")) {
		writeErr(w, 401, errors.New("подпись не прошла проверку"))
		return
	}
	var ev pagerduty.WebhookEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		writeErr(w, 400, err)
		return
	}
	applied := 0
	for _, key := range ev.DedupKeys() {
		if err := s.eng.PDInbound(key, ev.Event.EventType, ev.Event.Agent.Summary); err == nil {
			applied++
		}
	}
	writeJSON(w, 200, map[string]int{"applied": applied})
}

var grafanaStub = template.Must(template.New("g").Parse(`<!doctype html><meta charset="utf-8"><title>Контекст {{.ID}}</title>
<body style="font-family:system-ui;padding:32px;max-width:640px">
<h2>Контекст инцидента {{.ID}}</h2>
<p>Grafana не настроена (переменная UMBRELLA_GRAFANA_URL). Когда она задана, эта ссылка перенаправляет на дашборд <code>umb-{{.ID}}</code> с окном
<b>{{.From}}</b> — <b>{{.To}}</b>.</p>
<p>КЕ: <b>{{.CI}}</b>, сервис: <b>{{.Service}}</b>, сигнал: <b>{{.Signal}}</b>.</p>
<p><a href="/incidents?id={{.ID}}">Вернуться к инциденту</a></p></body>`))

func (s *Server) grafana(w http.ResponseWriter, r *http.Request) {
	a, ok := s.eng.Get(r.PathValue("id"))
	if !ok || !me(r).SeesCI(a.CIID) {
		writeErr(w, 404, alert.ErrNotFound)
		return
	}
	from := a.FirstSeen.Add(-10 * time.Minute)
	to := time.Now()
	if a.ResolvedAt != nil {
		to = *a.ResolvedAt
	}
	to = to.Add(10 * time.Minute)
	if s.cfg.GrafanaURL == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = grafanaStub.Execute(w, map[string]string{"ID": a.ID, "From": from.Format("02.01 15:04"), "To": to.Format("02.01 15:04"), "CI": a.CIName, "Service": a.Service, "Signal": a.Signal})
		return
	}
	// Context Builder (next step) creates the dashboard with uid umb-<id>;
	// here the link carries the window and variables.
	v := url.Values{}
	v.Set("from", strconv.FormatInt(from.UnixMilli(), 10))
	v.Set("to", strconv.FormatInt(to.UnixMilli(), 10))
	v.Set("var-ci", a.CIName)
	v.Set("var-service", a.Service)
	v.Set("var-incident", a.ID)
	// A URL that already names a dashboard (.../d/<uid>) is used as is, e.g.
	// the provisioned incidents dashboard of the lab.
	target := strings.TrimRight(s.cfg.GrafanaURL, "/")
	if !strings.Contains(target, "/d/") {
		target += "/d/umb-" + strings.ToLower(a.ID)
	}
	http.Redirect(w, r, target+"?"+v.Encode(), http.StatusFound)
}

// ---- static ----

func spa(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})
}
