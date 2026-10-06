// Package pagerduty delivers alerts to PagerDuty with the Events API v2 and takes incident
// status changes back from Webhooks v3. It also uses the REST API to find services and their
// integration keys and to subscribe to webhooks.
package pagerduty

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/textx"
)

var (
	ErrDisabled   = errors.New("PagerDuty is not enabled")
	ErrNoKey      = errors.New("no Events API v2 integration key is set")
	ErrNoAPIToken = errors.New("no PagerDuty REST API token is set")
	ErrQueueFull  = errors.New("the PagerDuty queue is full")
	ErrBreaker    = errors.New("the circuit breaker is open after a series of failures")
)

const (
	breakerThreshold = 5
	breakerPause     = 60 * time.Second
	DefaultRoute     = "default"
	queueSize        = 10000
)

// Resolver reads a secret by its openbao:// reference.
type Resolver interface {
	Resolve(ref string) (string, error)
}

// Results is where the gateway reports deliveries and incident changes: the alert engine.
type Results interface {
	PDResult(ctx context.Context, alertID string, action alert.Action, route, routeID string, err error)
	PDInbound(ctx context.Context, u alert.PDUpdate) error
	PDKeys(ctx context.Context, incidentKey, incidentID string) ([]string, error)
}

type Status struct {
	Enabled       bool       `json:"enabled"`
	Configured    bool       `json:"configured"`
	BreakerOpen   bool       `json:"breaker_open"`
	ConsecFails   int        `json:"consecutive_failures"`
	Sent          int        `json:"sent"`
	Failed        int        `json:"failed"`
	Queue         int        `json:"queue"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
	LastErrorAt   *time.Time `json:"last_error_at,omitempty"`
	LastWebhookAt *time.Time `json:"last_webhook_at,omitempty"`
}

type Gateway struct {
	st      *store.Store
	sec     Resolver
	results Results
	client  *http.Client
	queue   chan alert.Command
	Retries int
	Backoff time.Duration

	mu          sync.Mutex
	stat        Status
	breakerTill time.Time
}

func New(st *store.Store, sec Resolver) *Gateway {
	return &Gateway{st: st, sec: sec, client: &http.Client{Timeout: 15 * time.Second}, queue: make(chan alert.Command, queueSize),
		Retries: 3, Backoff: 500 * time.Millisecond}
}

func (g *Gateway) SetResults(r Results) { g.results = r }

func (g *Gateway) settings() (model.PagerDuty, model.Alerting) {
	var s model.Alerting
	g.st.Read(func(d *store.Data) {
		s = d.Settings.Alerting
		s.PagerDuty.Routes = slices.Clone(d.Settings.Alerting.PagerDuty.Routes)
	})
	return s.PagerDuty, s
}

// Send queues a command; Run delivers it.
func (g *Gateway) Send(cmd alert.Command) {
	select {
	case g.queue <- cmd:
	default:
		g.report(cmd, "", "", ErrQueueFull)
	}
}

func (g *Gateway) Status() Status {
	set, _ := g.settings()
	g.mu.Lock()
	defer g.mu.Unlock()
	s := g.stat
	s.Enabled = set.Enabled
	s.Configured = set.RoutingKeyRef != "" || len(set.Routes) > 0
	s.Queue = len(g.queue)
	s.BreakerOpen = time.Now().Before(g.breakerTill)
	return s
}

func (g *Gateway) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-g.queue:
			g.deliver(ctx, cmd)
		}
	}
}

type permanent struct{ error }

func (p permanent) Unwrap() error { return p.error }

func (g *Gateway) deliver(ctx context.Context, cmd alert.Command) {
	set, all := g.settings()
	if !set.Enabled {
		g.report(cmd, "", "", ErrDisabled)
		return
	}
	if min := set.MinSeverity; cmd.Action == alert.PDTrigger && min != "" && alert.SeverityRank(cmd.Alert.Severity) < alert.SeverityRank(min) {
		g.report(cmd, "", "", fmt.Errorf("%w %s", alert.ErrPDSkipped, min))
		return
	}
	routeName, routeID, ref := deliveryRoute(set, cmd.Alert)
	if ref == "" {
		g.report(cmd, routeName, routeID, ErrNoKey)
		return
	}
	key, err := g.sec.Resolve(ref)
	if err != nil {
		g.report(cmd, routeName, routeID, fmt.Errorf("integration key: %w", err))
		return
	}
	ev := Build(all.PublicURL, all.Grafana.DashboardURL != "", key, cmd)
	backoff := g.Backoff
	for attempt := 0; attempt < g.Retries; attempt++ {
		if g.breakerOpen() {
			err = ErrBreaker
			break
		}
		err = g.post(ctx, set.Events(), ev)
		var perm permanent
		if err == nil || errors.As(err, &perm) {
			break
		}
		g.fail(err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	g.mu.Lock()
	now := time.Now().UTC()
	if err == nil {
		g.stat.Sent++
		g.stat.ConsecFails = 0
		g.stat.LastSuccessAt = &now
	} else {
		g.stat.Failed++
		g.stat.LastError, g.stat.LastErrorAt = err.Error(), &now
	}
	g.mu.Unlock()
	g.report(cmd, routeName, routeID, err)
}

// Route picks the PagerDuty service of an alert: the first route from the top whose team and
// business service match the team and the primary service of the alert route (an empty field
// matches any), otherwise the default integration key.
func Route(set model.PagerDuty, a alert.Alert) (string, string) {
	name, _, ref := currentRoute(set, a)
	return name, ref
}

// RouteOf names the route a new trigger of the alert would take: its ID (DefaultRoute for the
// default integration) and name.
func RouteOf(set model.PagerDuty, a alert.Alert) (id, name string) {
	name, id, _ = currentRoute(set, a)
	return id, name
}

// deliveryRoute is the route of a command: the one the accepted trigger went by while it
// exists, because PagerDuty has the incident in that service and the alert may have been routed
// to another team since; otherwise the current route.
func deliveryRoute(set model.PagerDuty, a alert.Alert) (name, id, ref string) {
	switch id := a.PD.RouteID; {
	case id == "":
	case id == DefaultRoute:
		if set.RoutingKeyRef != "" {
			return DefaultRoute, DefaultRoute, set.RoutingKeyRef
		}
	default:
		for _, r := range set.Routes {
			if r.ID == id && r.RoutingKeyRef != "" {
				return r.Name, r.ID, r.RoutingKeyRef
			}
		}
	}
	return currentRoute(set, a)
}

func currentRoute(set model.PagerDuty, a alert.Alert) (name, id, ref string) {
	team := ""
	if a.Route.Team != nil {
		team = a.Route.Team.ID
	}
	// A route matches the primary service of the alert route, the one its team comes from, so
	// PagerDuty and backup notification agree on who owns the incident. Alerts routed before the
	// primary service was recorded match any of their services.
	services := a.Route.ServiceIDs()
	if a.Route.Service != nil {
		services = []string{a.Route.Service.ID}
	}
	for _, r := range set.Routes {
		if r.TeamID == "" && r.ServiceID == "" {
			continue
		}
		if (r.TeamID == "" || r.TeamID == team) && (r.ServiceID == "" || slices.Contains(services, r.ServiceID)) {
			return r.Name, r.ID, r.RoutingKeyRef
		}
	}
	return DefaultRoute, DefaultRoute, set.RoutingKeyRef
}

func (g *Gateway) report(cmd alert.Command, route, routeID string, err error) {
	if g.results != nil {
		g.results.PDResult(context.Background(), cmd.Alert.ID, cmd.Action, route, routeID, err)
	}
}

func (g *Gateway) breakerOpen() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return time.Now().Before(g.breakerTill)
}

func (g *Gateway) fail(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.stat.ConsecFails++
	now := time.Now().UTC()
	g.stat.LastError, g.stat.LastErrorAt = err.Error(), &now
	if g.stat.ConsecFails >= breakerThreshold {
		g.breakerTill = now.Add(breakerPause)
		g.stat.ConsecFails = 0
		slog.Warn("pagerduty circuit breaker opened", "err", err)
	}
}

type Event struct {
	RoutingKey  string   `json:"routing_key"`
	EventAction string   `json:"event_action"`
	DedupKey    string   `json:"dedup_key"`
	Payload     *Payload `json:"payload,omitempty"`
	Links       []Link   `json:"links,omitempty"`
	Client      string   `json:"client,omitempty"`
	ClientURL   string   `json:"client_url,omitempty"`
}

type Payload struct {
	Summary       string         `json:"summary"`
	Source        string         `json:"source"`
	Severity      string         `json:"severity"`
	Timestamp     string         `json:"timestamp,omitempty"`
	Component     string         `json:"component,omitempty"`
	Group         string         `json:"group,omitempty"`
	Class         string         `json:"class,omitempty"`
	CustomDetails map[string]any `json:"custom_details,omitempty"`
}

type Link struct {
	Href string `json:"href"`
	Text string `json:"text"`
}

func people(list []alert.Person) []string {
	out := []string{}
	for _, p := range list {
		v := p.Name
		if p.Email != "" {
			v += " <" + p.Email + ">"
		}
		if p.Role != "" {
			v += " (" + p.Role + ")"
		}
		out = append(out, strings.TrimSpace(v))
	}
	return out
}

// Build makes the Events API v2 event of a command. Only trigger carries the payload.
func Build(publicURL string, grafana bool, routingKey string, cmd alert.Command) Event {
	a := cmd.Alert
	ev := Event{RoutingKey: routingKey, EventAction: string(cmd.Action), DedupKey: a.PD.Key, Client: "Umbrella"}
	base := strings.TrimRight(publicURL, "/")
	if base != "" {
		ev.ClientURL = base + "/incidents?id=" + a.ID
	}
	if cmd.Action != alert.PDTrigger {
		return ev
	}
	sources := make([]string, 0, len(a.Sources))
	for _, s := range a.Sources {
		if !slices.Contains(sources, s.ConnectorID) {
			sources = append(sources, s.ConnectorID)
		}
	}
	slices.Sort(sources)
	source := a.CIName
	if source == "" {
		source = "umbrella"
	}
	services := []string{}
	for _, s := range a.Route.Services {
		services = append(services, s.Name)
	}
	details := map[string]any{
		"umbrella_id": a.ID,
		"method":      a.Method,
		"ci_id":       a.CIID,
		"ci_kind":     a.CIKind,
		"services":    services,
		"count":       a.Count,
		"sources":     sources,
		"people":      people(a.Route.People),
		"owners":      people(a.Route.Owners),
	}
	if a.Route.Team != nil {
		details["team"] = a.Route.Team.Name
	}
	if a.RelatedID != "" {
		details["related"] = a.RelatedID
	}
	if len(a.Labels) > 0 {
		details["labels"] = a.Labels
	}
	group := ""
	if len(services) > 0 {
		group = services[0]
	}
	ev.Payload = &Payload{
		Summary:       truncate(fmt.Sprintf("[%s] %s", strings.ToUpper(a.Severity), a.Title), 1024),
		Source:        truncate(source, 255),
		Severity:      a.Severity,
		Timestamp:     a.OpenedAt.UTC().Format(time.RFC3339),
		Component:     a.CIName,
		Group:         group,
		Class:         a.Signal,
		CustomDetails: details,
	}
	if base != "" {
		ev.Links = []Link{{Href: base + "/incidents?id=" + a.ID, Text: "Umbrella incident"}}
		if grafana {
			ev.Links = append(ev.Links, Link{Href: base + "/go/incidents/" + a.ID + "/grafana", Text: "Incident context in Grafana"})
		}
	}
	return ev
}

func (g *Gateway) post(ctx context.Context, url string, ev Event) error {
	body, _ := json.Marshal(ev)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return permanent{err}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	switch {
	case resp.StatusCode/100 == 2:
		return nil
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return fmt.Errorf("Events API answered %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	default:
		return permanent{fmt.Errorf("Events API rejected the event (%d): %s", resp.StatusCode, eventsError(msg))}
	}
}

func eventsError(msg []byte) string {
	var e struct {
		Message string   `json:"message"`
		Errors  []string `json:"errors"`
	}
	if json.Unmarshal(msg, &e) == nil && (e.Message != "" || len(e.Errors) > 0) {
		return strings.TrimSpace(e.Message + " " + strings.Join(e.Errors, "; "))
	}
	return strings.TrimSpace(string(msg))
}

// SendTest triggers and resolves a test event with the default integration key, or with the
// given key before it is saved.
func (g *Gateway) SendTest(ctx context.Context, key string) error {
	set, _ := g.settings()
	if key == "" {
		if set.RoutingKeyRef == "" {
			return ErrNoKey
		}
		var err error
		if key, err = g.sec.Resolve(set.RoutingKeyRef); err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	a := alert.Alert{ID: "TEST", PD: alert.PD{Key: fmt.Sprintf("umb-test-%d", now.Unix())}, Title: "Umbrella → PagerDuty connection test",
		CIName: "umbrella", Severity: "info", Signal: "umbrella.test", OpenedAt: now, Sources: map[string]*alert.Source{}}
	if err := g.post(ctx, set.Events(), Build("", false, key, alert.Command{Action: alert.PDTrigger, Alert: a})); err != nil {
		return err
	}
	return g.post(ctx, set.Events(), Build("", false, key, alert.Command{Action: alert.PDResolve, Alert: a}))
}

// truncate cuts to n characters, as the Events API counts its limits.
func truncate(s string, n int) string { return textx.Runes(s, n) }

var _ alert.Sender = (*Gateway)(nil)
