// Package pagerduty is the PagerDuty Gateway: it sends alerts to the Events
// API v2 with dedup_key umb-<alert id>, retries with backoff, opens a
// circuit breaker after repeated failures and verifies inbound Webhooks v3.
package pagerduty

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
)

// Config of the gateway.
type Config struct {
	// EventsURL defaults to https://events.pagerduty.com/v2/enqueue.
	EventsURL string
	// RoutingKey is the integration key. Empty means dry-run: nothing leaves
	// the process and every send is reported as accepted.
	RoutingKey string
	// WebhookSecret verifies X-PagerDuty-Signature on inbound webhooks.
	WebhookSecret string
	// PublicURL is the base for links in the alert (Grafana context).
	PublicURL string
	Retries   int
}

// Status is shown on the self-check page.
type Status struct {
	Mode          string     `json:"mode"` // live, dry-run
	BreakerOpen   bool       `json:"breaker_open"`
	Outage        bool       `json:"simulated_outage"`
	ConsecFails   int        `json:"consecutive_failures"`
	Sent          int        `json:"sent"`
	Failed        int        `json:"failed"`
	QueueLen      int        `json:"queue"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

// Gateway implements alert.Sender.
type Gateway struct {
	cfg    Config
	client *http.Client
	queue  chan alert.PDCommand
	result func(alertID string, action alert.PDAction, err error)

	mu          sync.Mutex
	st          Status
	breakerTill time.Time
}

// New creates the gateway. Call SetResult before Run.
func New(cfg Config) *Gateway {
	if cfg.EventsURL == "" {
		cfg.EventsURL = "https://events.pagerduty.com/v2/enqueue"
	}
	if cfg.Retries == 0 {
		cfg.Retries = 3
	}
	g := &Gateway{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}, queue: make(chan alert.PDCommand, 10000)}
	g.st.Mode = "live"
	if cfg.RoutingKey == "" {
		g.st.Mode = "dry-run"
	}
	return g
}

// SetResult registers the callback that reports delivery results.
func (g *Gateway) SetResult(f func(alertID string, action alert.PDAction, err error)) { g.result = f }

// Send queues a command (transactional outbox in the target design).
func (g *Gateway) Send(cmd alert.PDCommand) {
	select {
	case g.queue <- cmd:
	default:
		g.report(cmd, errors.New("очередь PagerDuty Gateway переполнена"))
	}
}

// SetOutage simulates PagerDuty being unavailable (self-check page), to
// exercise the circuit breaker and the fallback path.
func (g *Gateway) SetOutage(on bool) {
	g.mu.Lock()
	g.st.Outage = on
	if !on {
		g.breakerTill = time.Time{}
		g.st.BreakerOpen = false
		g.st.ConsecFails = 0
	}
	g.mu.Unlock()
}

// Status returns a snapshot.
func (g *Gateway) Status() Status {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := g.st
	s.QueueLen = len(g.queue)
	s.BreakerOpen = time.Now().Before(g.breakerTill)
	return s
}

// Run delivers queued commands until ctx is done.
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

func (g *Gateway) deliver(ctx context.Context, cmd alert.PDCommand) {
	var err error
	backoff := 500 * time.Millisecond
	for attempt := 0; attempt < g.cfg.Retries; attempt++ {
		if g.breakerOpen() {
			err = errors.New("circuit breaker открыт: отправка приостановлена")
			break
		}
		err = g.post(ctx, cmd)
		if err == nil {
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
	if err == nil {
		g.mu.Lock()
		g.st.Sent++
		g.st.ConsecFails = 0
		t := time.Now()
		g.st.LastSuccessAt = &t
		g.mu.Unlock()
	} else {
		g.mu.Lock()
		g.st.Failed++
		g.mu.Unlock()
	}
	g.report(cmd, err)
}

func (g *Gateway) report(cmd alert.PDCommand, err error) {
	if g.result != nil {
		g.result(cmd.Alert.ID, cmd.Action, err)
	}
}

func (g *Gateway) breakerOpen() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return time.Now().Before(g.breakerTill)
}

// fail counts a failed attempt; 5 in a row open the breaker for 60 s.
func (g *Gateway) fail(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.st.ConsecFails++
	g.st.LastError = err.Error()
	if g.st.ConsecFails >= 5 {
		g.breakerTill = time.Now().Add(60 * time.Second)
		g.st.ConsecFails = 0
		slog.Warn("pagerduty circuit breaker opened", "err", err)
	}
}

// Event is the Events API v2 body.
type Event struct {
	RoutingKey  string    `json:"routing_key"`
	EventAction string    `json:"event_action"`
	DedupKey    string    `json:"dedup_key"`
	Payload     *Payload  `json:"payload,omitempty"`
	Links       []Link    `json:"links,omitempty"`
	Client      string    `json:"client,omitempty"`
	ClientURL   string    `json:"client_url,omitempty"`
}

// Payload of a trigger event.
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

// Link in the incident.
type Link struct {
	Href string `json:"href"`
	Text string `json:"text"`
}

// Build turns a command into an Events API v2 body (the event template).
func Build(cfg Config, cmd alert.PDCommand) Event {
	a := cmd.Alert
	ev := Event{RoutingKey: cfg.RoutingKey, EventAction: string(cmd.Action), DedupKey: a.PDKey, Client: "Umbrella"}
	base := strings.TrimRight(cfg.PublicURL, "/")
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
	ev.Payload = &Payload{
		Summary:   truncate(fmt.Sprintf("[%s] %s", strings.ToUpper(string(a.Severity)), a.Title), 1024),
		Source:    a.CIName,
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
	if base != "" {
		ev.Links = []Link{
			{Href: base + "/go/incidents/" + a.ID + "/grafana", Text: "Контекст инцидента в Grafana"},
			{Href: base + "/incidents?id=" + a.ID, Text: "Инцидент в Umbrella"},
		}
	}
	return ev
}

func (g *Gateway) post(ctx context.Context, cmd alert.PDCommand) error {
	g.mu.Lock()
	outage := g.st.Outage
	g.mu.Unlock()
	if outage {
		return errors.New("PagerDuty недоступен (симуляция)")
	}
	if g.cfg.RoutingKey == "" {
		return nil // dry-run
	}
	body, _ := json.Marshal(Build(g.cfg, cmd))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.cfg.EventsURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode/100 != 2 {
		return fmt.Errorf("Events API ответил %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

// VerifySignature checks X-PagerDuty-Signature ("v1=<hex>[,v1=<hex>]").
// With no secret configured it accepts everything (dev mode).
func VerifySignature(secret string, body []byte, header string) bool {
	if secret == "" {
		return true
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if v, ok := strings.CutPrefix(part, "v1="); ok && hmac.Equal([]byte(v), []byte(want)) {
			return true
		}
	}
	return false
}

// WebhookEvent is the subset of a Webhooks v3 message Umbrella needs.
type WebhookEvent struct {
	Event struct {
		EventType string `json:"event_type"`
		Agent     struct {
			Summary string `json:"summary"`
		} `json:"agent"`
		Data struct {
			ID       string `json:"id"`
			DedupKey string `json:"incident_key"`
			Alerts   []struct {
				DedupKey string `json:"dedup_key"`
			} `json:"alerts"`
		} `json:"data"`
	} `json:"event"`
}

// DedupKeys returns the alert keys the webhook refers to.
func (w WebhookEvent) DedupKeys() []string {
	var keys []string
	if w.Event.Data.DedupKey != "" {
		keys = append(keys, w.Event.Data.DedupKey)
	}
	for _, a := range w.Event.Data.Alerts {
		if a.DedupKey != "" {
			keys = append(keys, a.DedupKey)
		}
	}
	return keys
}

var _ alert.Sender = (*Gateway)(nil)

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
