// Package notify sends incident notifications to messenger webhooks:
// Microsoft Teams (Adaptive Card through an incoming webhook or a Workflows
// webhook) and Zoom Team Chat (incoming webhook app). PagerDuty stays the
// primary channel; a channel in "fallback" mode speaks only when PagerDuty
// did not take the incident in time.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// Secrets resolves openbao:// references.
type Secrets interface {
	Resolve(ref string) (string, error)
}

// Config of the notifier.
type Config struct {
	PublicURL string // Umbrella address for incident links
	AllowHTTP bool   // allow http:// webhooks (lab and tests only)
	Retries   int    // attempts per message, default 3
	Backoff   time.Duration
}

type job struct {
	ch    model.Channel
	alert model.Alert
	event string
}

// Notifier watches alert changes and delivers notifications.
type Notifier struct {
	cfg     Config
	st      *store.Store
	secrets Secrets
	client  *http.Client
	queue   chan job

	mu   sync.Mutex
	seen map[string]state
}

type state struct {
	status     model.AlertStatus
	sev        model.Severity
	fallback   bool
	suppressed bool
	opened     bool // an "open" notification went out
}

// New creates the notifier. Call Run to start delivery.
func New(cfg Config, st *store.Store, secrets Secrets) *Notifier {
	if cfg.Retries <= 0 {
		cfg.Retries = 3
	}
	if cfg.Backoff <= 0 {
		cfg.Backoff = time.Second
	}
	return &Notifier{cfg: cfg, st: st, secrets: secrets, client: &http.Client{Timeout: 10 * time.Second},
		queue: make(chan job, 2000), seen: map[string]state{}}
}

// Run delivers queued messages until ctx ends.
func (n *Notifier) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-n.queue:
			d := n.deliver(ctx, j.ch, j.alert, j.event, "")
			n.record(d)
		}
	}
}

// Observe takes every live update; alert changes become notification events.
func (n *Notifier) Observe(kind string, v any) {
	a, ok := v.(model.Alert)
	if kind != "alert" || !ok {
		return
	}
	for _, ev := range n.events(a) {
		n.dispatch(a, ev)
	}
}

// events compares the alert with what was seen before.
func (n *Notifier) events(a model.Alert) []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	prev, known := n.seen[a.ID]
	cur := state{status: a.Status, sev: a.Severity, fallback: a.Fallback, suppressed: a.Suppressed, opened: prev.opened}
	var out []string
	active := a.Status.Active()
	switch {
	case a.Suppressed:
		// Maintenance: nobody is told.
	case active && (!known || !prev.opened || !prev.status.Active()):
		out = append(out, model.NotifyOpen)
		cur.opened = true
	case active && a.Severity.Rank() > prev.sev.Rank():
		out = append(out, model.NotifyEscalate)
	}
	if known && prev.opened && !a.Suppressed {
		if prev.status == model.AlertOpen && a.Status == model.AlertAcknowledged {
			out = append(out, model.NotifyAck)
		}
		if prev.status.Active() && !active {
			out = append(out, model.NotifyResolve)
		}
	}
	if a.Fallback && !prev.fallback && active {
		out = append(out, model.NotifyFallback)
	}
	if !active {
		cur.opened = false
	}
	n.seen[a.ID] = cur
	if len(n.seen) > 50000 {
		for id, s := range n.seen {
			if !s.status.Active() {
				delete(n.seen, id)
			}
		}
	}
	return out
}

// dispatch queues the event for every channel that wants it.
func (n *Notifier) dispatch(a model.Alert, event string) {
	var targets []model.Channel
	n.st.Read(func(d *store.Data) {
		for _, ch := range d.Channels {
			if wants(d, ch, a, event) {
				targets = append(targets, *ch)
			}
		}
	})
	for _, ch := range targets {
		select {
		case n.queue <- job{ch: ch, alert: a, event: event}:
		default:
			slog.Warn("notification queue is full", "channel", ch.ID, "alert", a.ID)
		}
	}
}

// wants applies the channel filters. Call inside a store read.
func wants(d *store.Data, ch *model.Channel, a model.Alert, event string) bool {
	if !ch.Enabled || a.Severity.Rank() < ch.MinSeverity.Rank() {
		return false
	}
	listed := false
	for _, e := range ch.Events {
		if e == event {
			listed = true
		}
	}
	if !listed {
		return false
	}
	if ch.Mode == model.ChannelFallback {
		// Only incidents PagerDuty did not take, from the fallback moment on.
		if !a.Fallback || event == model.NotifyOpen || event == model.NotifyEscalate {
			return false
		}
	}
	if len(ch.Services) > 0 {
		if a.CIID == "" || !auth.ServiceScope(d, ch.Services)[a.CIID] {
			return false
		}
	}
	return true
}

// Test sends a test message right away and records the result.
func (n *Notifier) Test(ch model.Channel, actor string) model.Delivery {
	now := time.Now()
	a := model.Alert{ID: "TEST", Title: "Проверка канала уведомлений Umbrella", CIName: "umbrella", Severity: model.SevInfo,
		Status: model.AlertOpen, Signal: "test", Method: model.MethodOther, FirstSeen: now, LastSeen: now, Count: 1, Team: actor}
	d := n.deliver(context.Background(), ch, a, "test", actor)
	n.record(d)
	return d
}

func (n *Notifier) record(d model.Delivery) {
	n.st.Write(func(s *store.Data) {
		d.ID = s.NextID("NTF")
		s.AddDelivery(&d)
		if ch := s.Channels[d.ChannelID]; ch != nil {
			at := d.At
			ch.LastAt = &at
			if d.OK {
				ch.Sent++
				ch.LastStatus, ch.LastError = "ok", ""
			} else {
				ch.Failed++
				ch.LastStatus, ch.LastError = "failed", d.Error
			}
		}
	})
}

// deliver sends one message with retries.
func (n *Notifier) deliver(ctx context.Context, ch model.Channel, a model.Alert, event, actor string) model.Delivery {
	d := model.Delivery{ChannelID: ch.ID, Channel: ch.Name, AlertID: a.ID, Event: event, At: time.Now()}
	if a.ID == "TEST" {
		d.AlertID = ""
	}
	target, token, err := n.resolve(ch)
	if err != nil {
		d.Error = err.Error()
		return d
	}
	req, err := n.build(ch, a, event, target, token)
	if err != nil {
		d.Error = err.Error()
		return d
	}
	wait := n.cfg.Backoff
	for attempt := 1; attempt <= n.cfg.Retries; attempt++ {
		d.Attempts = attempt
		status, err := n.send(ctx, req)
		d.Status = status
		if err == nil {
			d.OK, d.Error = true, ""
			return d
		}
		d.Error = err.Error()
		// 4xx other than 429 will not get better.
		if status >= 400 && status < 500 && status != http.StatusTooManyRequests {
			return d
		}
		if attempt < n.cfg.Retries {
			select {
			case <-ctx.Done():
				return d
			case <-time.After(wait):
			}
			wait *= 3
		}
	}
	return d
}

type request struct {
	url     string
	headers map[string]string
	body    []byte
}

func (n *Notifier) send(ctx context.Context, r request) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(r.body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return 0, redact(err, r.url)
	}
	defer resp.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if resp.StatusCode/100 != 2 {
		return resp.StatusCode, fmt.Errorf("ответ %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return resp.StatusCode, nil
}

// redact keeps webhook secrets (path and query) out of error messages.
func redact(err error, raw string) error {
	u, perr := url.Parse(raw)
	if perr != nil {
		return errors.New("ошибка соединения")
	}
	return errors.New(strings.ReplaceAll(err.Error(), raw, u.Scheme+"://"+u.Host+"/…"))
}

func (n *Notifier) resolve(ch model.Channel) (string, string, error) {
	target, token := ch.URL, ch.Token
	if ch.URLRef != "" {
		v, err := n.secrets.Resolve(ch.URLRef)
		if err != nil {
			return "", "", err
		}
		target = v
	}
	if ch.TokenRef != "" {
		v, err := n.secrets.Resolve(ch.TokenRef)
		if err != nil {
			return "", "", err
		}
		token = v
	}
	if err := CheckURL(target, n.cfg.AllowHTTP); err != nil {
		return "", "", err
	}
	return target, token, nil
}

// CheckURL accepts https URLs (and http when allowed).
func CheckURL(raw string, allowHTTP bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return errors.New("адрес webhook не задан или неверен")
	}
	if u.Scheme != "https" && !(allowHTTP && u.Scheme == "http") {
		return errors.New("адрес webhook должен начинаться с https://")
	}
	return nil
}

// Host returns the host of a webhook URL for display.
func Host(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

// ---- messages ----

var eventTitle = map[string]string{
	model.NotifyOpen:     "Новый инцидент",
	model.NotifyEscalate: "Важность повышена",
	model.NotifyAck:      "Инцидент подтверждён",
	model.NotifyResolve:  "Инцидент решён",
	model.NotifyFallback: "PagerDuty не принял инцидент",
	"test":               "Проверка канала",
}

var sevColor = map[model.Severity]string{
	model.SevCritical: "#d92d20", model.SevError: "#f97316", model.SevWarning: "#eab308", model.SevInfo: "#3b82f6",
}

func (n *Notifier) link(a model.Alert) string {
	base := strings.TrimRight(n.cfg.PublicURL, "/")
	if a.ID == "TEST" {
		return base + "/notifications"
	}
	return base + "/incidents?id=" + url.QueryEscape(a.ID)
}

type fact struct{ k, v string }

func facts(a model.Alert) []fact {
	out := []fact{{"Важность", string(a.Severity)}, {"Статус", string(a.Status)}, {"КЕ", a.CIName}}
	if a.Service != "" {
		out = append(out, fact{"Сервис", a.Service})
	}
	if a.Team != "" {
		out = append(out, fact{"Команда", a.Team})
	}
	out = append(out, fact{"Сигнал", a.Signal}, fact{"Открыт", a.FirstSeen.Format("02.01 15:04:05")})
	if a.AckedBy != "" {
		out = append(out, fact{"Подтвердил", a.AckedBy})
	}
	if a.PDState != "" {
		out = append(out, fact{"PagerDuty", string(a.PDState)})
	}
	return out
}

func (n *Notifier) build(ch model.Channel, a model.Alert, event, target, token string) (request, error) {
	head := eventTitle[event]
	if head == "" {
		head = event
	}
	title := fmt.Sprintf("%s: %s", head, a.Title)
	if a.ID != "TEST" {
		title = fmt.Sprintf("%s · %s: %s", head, a.ID, a.Title)
	}
	switch ch.Type {
	case model.ChannelTeams:
		var fs []map[string]string
		for _, f := range facts(a) {
			fs = append(fs, map[string]string{"title": f.k, "value": f.v})
		}
		color := "Default"
		switch {
		case event == model.NotifyResolve:
			color = "Good"
		case a.Severity == model.SevCritical || a.Severity == model.SevError:
			color = "Attention"
		case a.Severity == model.SevWarning:
			color = "Warning"
		}
		card := map[string]any{
			"$schema": "http://adaptivecards.io/schemas/adaptive-card.json", "type": "AdaptiveCard", "version": "1.4",
			"msteams": map[string]any{"width": "Full"},
			"body": []any{
				map[string]any{"type": "TextBlock", "text": "Umbrella", "size": "Small", "isSubtle": true},
				map[string]any{"type": "TextBlock", "text": title, "size": "Medium", "weight": "Bolder", "wrap": true, "color": color},
				map[string]any{"type": "FactSet", "facts": fs},
			},
			"actions": []any{map[string]any{"type": "Action.OpenUrl", "title": "Открыть в Umbrella", "url": n.link(a)}},
		}
		body, err := json.Marshal(map[string]any{"type": "message", "summary": title,
			"attachments": []any{map[string]any{"contentType": "application/vnd.microsoft.card.adaptive", "contentUrl": nil, "content": card}}})
		return request{url: target, body: body}, err
	case model.ChannelZoom:
		u, err := url.Parse(target)
		if err != nil {
			return request{}, err
		}
		q := u.Query()
		if q.Get("format") == "" {
			q.Set("format", "full")
			u.RawQuery = q.Encode()
		}
		var items []map[string]string
		for _, f := range facts(a) {
			items = append(items, map[string]string{"key": f.k, "value": f.v})
		}
		color := sevColor[a.Severity]
		if event == model.NotifyResolve {
			color = "#16a34a"
		}
		msg := map[string]any{"content": map[string]any{
			"head": map[string]any{"text": title, "style": map[string]any{"bold": true, "color": color},
				"sub_head": map[string]any{"text": "Umbrella"}},
			"body": []any{
				map[string]any{"type": "fields", "items": items},
				map[string]any{"type": "message", "text": "Открыть в Umbrella: " + n.link(a)},
			},
		}}
		body, err := json.Marshal(msg)
		h := map[string]string{}
		if token != "" {
			h["Authorization"] = token
		}
		return request{url: u.String(), headers: h, body: body}, err
	}
	return request{}, errors.New("неизвестный тип канала " + ch.Type)
}
