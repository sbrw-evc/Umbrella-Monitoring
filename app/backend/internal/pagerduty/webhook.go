package pagerduty

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

var (
	ErrNoWebhookSecret = errors.New("секрет подписи вебхуков PagerDuty не задан: webhook отклонён")
	ErrBadSignature    = errors.New("подпись X-PagerDuty-Signature не прошла проверку")
)

func VerifySignature(secret string, body []byte, header string) bool {
	if secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	for _, part := range strings.Split(header, ",") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(part), "v1="); ok && hmac.Equal([]byte(v), []byte(want)) {
			return true
		}
	}
	return false
}

type summary struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

type WebhookEvent struct {
	Event struct {
		ID           string   `json:"id"`
		EventType    string   `json:"event_type"`
		ResourceType string   `json:"resource_type"`
		Agent        *summary `json:"agent"`
		Data         struct {
			ID          string    `json:"id"`
			Type        string    `json:"type"`
			HTMLURL     string    `json:"html_url"`
			IncidentKey string    `json:"incident_key"`
			Status      string    `json:"status"`
			Content     string    `json:"content"`
			Assignees   []summary `json:"assignees"`
			Priority    *summary  `json:"priority"`
			Incident    *struct {
				ID      string `json:"id"`
				HTMLURL string `json:"html_url"`
			} `json:"incident"`
		} `json:"data"`
	} `json:"event"`
}

func (w WebhookEvent) incident() (string, string) {
	d := w.Event.Data
	if d.Incident != nil && d.Incident.ID != "" {
		return d.Incident.ID, d.Incident.HTMLURL
	}
	if d.Type == "incident" || strings.HasPrefix(w.Event.EventType, "incident.") {
		return d.ID, d.HTMLURL
	}
	return "", ""
}

func (w WebhookEvent) detail() string {
	d := w.Event.Data
	switch w.Event.EventType {
	case "incident.reassigned", "incident.escalated":
		var names []string
		for _, a := range d.Assignees {
			names = append(names, a.Summary)
		}
		return strings.Join(names, ", ")
	case "incident.priority_updated":
		if d.Priority != nil {
			return d.Priority.Summary
		}
	case "incident.annotated":
		return truncate(d.Content, 300)
	}
	return ""
}

func (g *Gateway) HandleWebhook(ctx context.Context, body []byte, signature string) (int, error) {
	set := g.Settings()
	if set.WebhookSecretRef == "" {
		return 0, ErrNoWebhookSecret
	}
	secret, err := g.sec.Resolve(set.WebhookSecretRef)
	if err != nil {
		return 0, err
	}
	if !VerifySignature(secret, body, signature) {
		return 0, ErrBadSignature
	}
	var ev WebhookEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return 0, err
	}
	now := time.Now()
	g.mu.Lock()
	g.stat.LastWebhookAt = &now
	g.mu.Unlock()
	if ev.Event.EventType == "pagey.ping" {
		return 0, nil
	}
	incID, incURL := ev.incident()
	keys := g.keysFor(ctx, ev.Event.Data.IncidentKey, incID)
	actor := ""
	if ev.Event.Agent != nil {
		actor = ev.Event.Agent.Summary
	}
	applied := 0
	for _, k := range keys {
		if g.inbound == nil {
			break
		}
		err := g.inbound(alert.PDUpdate{DedupKey: k, EventType: ev.Event.EventType, Actor: actor,
			IncidentID: incID, IncidentURL: incURL, Detail: ev.detail()})
		if err == nil {
			applied++
		}
	}
	return applied, nil
}

func (g *Gateway) keysFor(ctx context.Context, incidentKey, incidentID string) []string {
	var keys []string
	g.st.Read(func(d *store.Data) {
		for _, a := range d.Alerts {
			if (incidentID != "" && a.PDIncidentID == incidentID) || (incidentKey != "" && a.PDKey == incidentKey) {
				keys = append(keys, a.PDKey)
			}
		}
	})
	if len(keys) > 0 || incidentID == "" {
		return keys
	}
	if strings.HasPrefix(incidentKey, "umb-") {
		return []string{incidentKey}
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	found, err := g.IncidentAlertKeys(cctx, incidentID)
	if err != nil {
		return nil
	}
	for _, k := range found {
		if strings.HasPrefix(k, "umb-") {
			keys = append(keys, k)
		}
	}
	return keys
}
