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
	"strings"
	"sync"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/secrets"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

var (
	ErrDisabled   = errors.New("PagerDuty не подключён: включите интеграцию в разделе «Интеграции → PagerDuty»")
	ErrNoKey      = errors.New("не задан ключ интеграции Events API v2")
	ErrNoAPIToken = errors.New("не задан API-токен PagerDuty REST")
)

const (
	breakerThreshold = 5
	breakerPause     = 60 * time.Second
	defaultRoute     = "по умолчанию"
)

type Config struct {
	PublicURL string
	Retries   int
	Backoff   time.Duration
}

type Status struct {
	Enabled        bool       `json:"enabled"`
	Configured     bool       `json:"configured"`
	Region         string     `json:"region"`
	APIToken       bool       `json:"api_token"`
	WebhookSecret  bool       `json:"webhook_secret"`
	Routes         int        `json:"routes"`
	BreakerOpen    bool       `json:"breaker_open"`
	ConsecFails    int        `json:"consecutive_failures"`
	Sent           int        `json:"sent"`
	Failed         int        `json:"failed"`
	QueueLen       int        `json:"queue"`
	LastSuccessAt  *time.Time `json:"last_success_at,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	LastErrorAt    *time.Time `json:"last_error_at,omitempty"`
	LastWebhookAt  *time.Time `json:"last_webhook_at,omitempty"`
	OnCallSyncedAt *time.Time `json:"oncall_synced_at,omitempty"`
	OnCallError    string     `json:"oncall_error,omitempty"`
}

type Gateway struct {
	cfg     Config
	st      *store.Store
	sec     secrets.Resolver
	client  *http.Client
	queue   chan alert.PDCommand
	result  func(alertID string, action alert.PDAction, route string, err error)
	inbound func(alert.PDUpdate) error
	resync  chan struct{}

	mu          sync.Mutex
	stat        Status
	breakerTill time.Time
}

func New(cfg Config, st *store.Store, sec secrets.Resolver) *Gateway {
	if cfg.Retries == 0 {
		cfg.Retries = 3
	}
	if cfg.Backoff == 0 {
		cfg.Backoff = 500 * time.Millisecond
	}
	return &Gateway{cfg: cfg, st: st, sec: sec, client: &http.Client{Timeout: 15 * time.Second},
		queue: make(chan alert.PDCommand, 10000), resync: make(chan struct{}, 1)}
}

func (g *Gateway) SetResult(f func(alertID string, action alert.PDAction, route string, err error)) {
	g.result = f
}

func (g *Gateway) SetInbound(f func(alert.PDUpdate) error) { g.inbound = f }

func (g *Gateway) Settings() model.PDSettings {
	var s model.PDSettings
	g.st.Read(func(d *store.Data) {
		s = d.PagerDuty
		s.Routes = append([]model.PDRoute(nil), d.PagerDuty.Routes...)
		s.EscalationPolicies = append([]string(nil), d.PagerDuty.EscalationPolicies...)
	})
	return s
}

func (g *Gateway) Send(cmd alert.PDCommand) {
	select {
	case g.queue <- cmd:
	default:
		g.report(cmd, "", errors.New("очередь PagerDuty Gateway переполнена"))
	}
}

func (g *Gateway) Status() Status {
	set := g.Settings()
	var oc model.OnCall
	g.st.Read(func(d *store.Data) { oc = d.OnCall })
	g.mu.Lock()
	defer g.mu.Unlock()
	s := g.stat
	s.Enabled = set.Enabled
	s.Configured = set.RoutingKeyRef != "" || len(set.Routes) > 0
	s.Region = set.Region
	s.APIToken = set.APITokenRef != ""
	s.WebhookSecret = set.WebhookSecretRef != ""
	s.Routes = len(set.Routes)
	s.QueueLen = len(g.queue)
	s.BreakerOpen = time.Now().Before(g.breakerTill)
	s.OnCallSyncedAt, s.OnCallError = oc.SyncedAt, oc.Error
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

func (g *Gateway) deliver(ctx context.Context, cmd alert.PDCommand) {
	set := g.Settings()
	if !set.Enabled {
		g.report(cmd, "", ErrDisabled)
		return
	}
	if cmd.Action == alert.PDTrigger && cmd.Alert.Severity.Rank() < set.MinSeverity.Rank() {
		g.report(cmd, "", fmt.Errorf("%w %s", alert.ErrPDSkipped, set.MinSeverity))
		return
	}
	routeName, ref := Route(set, cmd.Alert)
	if ref == "" {
		g.report(cmd, routeName, ErrNoKey)
		return
	}
	key, err := g.sec.Resolve(ref)
	if err != nil {
		g.report(cmd, routeName, fmt.Errorf("ключ интеграции: %w", err))
		return
	}
	backoff := g.cfg.Backoff
	for attempt := 0; attempt < g.cfg.Retries; attempt++ {
		if g.breakerOpen() {
			err = errors.New("circuit breaker открыт: отправка приостановлена после серии ошибок")
			break
		}
		err = g.post(ctx, set.Events(), Build(g.cfg.PublicURL, g.grafanaConfigured(), key, cmd, g.owners(cmd.Alert.CIID)))
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
	now := time.Now()
	if err == nil {
		g.stat.Sent++
		g.stat.ConsecFails = 0
		g.stat.LastSuccessAt = &now
	} else {
		g.stat.Failed++
		g.stat.LastError, g.stat.LastErrorAt = err.Error(), &now
	}
	g.mu.Unlock()
	g.report(cmd, routeName, err)
}

func (g *Gateway) grafanaConfigured() bool {
	ok := false
	g.st.Read(func(d *store.Data) { ok = d.Settings.GrafanaURL != "" })
	return ok
}

func (g *Gateway) owners(ciID string) []string {
	var out []string
	g.st.Read(func(d *store.Data) {
		if ci := d.CIs[ciID]; ci != nil {
			out = OwnerList(ci.Owners)
		}
	})
	return out
}

func OwnerList(list []model.Owner) []string {
	var out []string
	for _, o := range list {
		v := o.Name
		if o.Email != "" {
			v += " <" + o.Email + ">"
		}
		if o.Role != "" {
			v += " (" + o.Role + ")"
		}
		out = append(out, strings.TrimSpace(v))
	}
	return out
}

func Route(set model.PDSettings, a model.Alert) (string, string) {
	for _, r := range set.Routes {
		if r.Team == "" && r.Service == "" {
			continue
		}
		if (r.Team == "" || strings.EqualFold(r.Team, a.Team)) && (r.Service == "" || strings.EqualFold(r.Service, a.Service)) {
			return r.Name, r.RoutingKeyRef
		}
	}
	return defaultRoute, set.RoutingKeyRef
}

func (g *Gateway) report(cmd alert.PDCommand, route string, err error) {
	if g.result != nil {
		g.result(cmd.Alert.ID, cmd.Action, route, err)
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
	now := time.Now()
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

func Build(publicURL string, grafana bool, routingKey string, cmd alert.PDCommand, owners []string) Event {
	a := cmd.Alert
	ev := Event{RoutingKey: routingKey, EventAction: string(cmd.Action), DedupKey: a.PDKey, Client: "Umbrella"}
	base := strings.TrimRight(publicURL, "/")
	if base != "" {
		ev.ClientURL = base + "/incidents?id=" + a.ID
	}
	if cmd.Action != alert.PDTrigger {
		return ev
	}
	sources := make([]string, 0, len(a.Sources))
	for s := range a.Sources {
		sources = append(sources, s)
	}
	source := a.CIName
	if source == "" {
		source = "umbrella"
	}
	ev.Payload = &Payload{
		Summary:   truncate(fmt.Sprintf("[%s] %s", strings.ToUpper(string(a.Severity)), a.Title), 1024),
		Source:    source,
		Severity:  string(a.Severity),
		Timestamp: a.FirstSeen.UTC().Format(time.RFC3339),
		Component: a.CIName,
		Group:     a.Service,
		Class:     a.Signal,
		CustomDetails: map[string]any{
			"umbrella_id": a.ID,
			"method":      a.Method,
			"ci_type":     a.CIType,
			"team":        a.Team,
			"count":       a.Count,
			"sources":     sources,
			"related":     a.RelatedID,
		},
	}
	if len(owners) > 0 {
		ev.Payload.CustomDetails["owners"] = owners
	}
	if base != "" {
		ev.Links = []Link{{Href: base + "/incidents?id=" + a.ID, Text: "Инцидент в Umbrella"}}
		if grafana {
			ev.Links = append(ev.Links, Link{Href: base + "/go/incidents/" + a.ID + "/grafana", Text: "Контекст инцидента в Grafana"})
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
		return fmt.Errorf("Events API ответил %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	default:
		return permanent{fmt.Errorf("Events API отклонил событие (%d): %s", resp.StatusCode, eventsError(msg))}
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

func (g *Gateway) SendTest(ctx context.Context) error {
	set := g.Settings()
	if set.RoutingKeyRef == "" {
		return ErrNoKey
	}
	key, err := g.sec.Resolve(set.RoutingKeyRef)
	if err != nil {
		return err
	}
	now := time.Now()
	a := model.Alert{ID: "TEST", PDKey: fmt.Sprintf("umb-test-%d", now.Unix()), Title: "Проверка связи Umbrella → PagerDuty",
		CIName: "umbrella", Severity: model.SevInfo, Signal: "umbrella.test", FirstSeen: now, Sources: map[string]string{}}
	if err := g.post(ctx, set.Events(), Build("", false, key, alert.PDCommand{Action: alert.PDTrigger, Alert: a}, nil)); err != nil {
		return err
	}
	return g.post(ctx, set.Events(), Build("", false, key, alert.PDCommand{Action: alert.PDResolve, Alert: a}, nil))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

var _ alert.Sender = (*Gateway)(nil)
