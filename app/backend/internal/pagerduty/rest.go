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
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

const (
	eventsIntegrationType = "events_api_v2_inbound_integration"
	oncallEvery           = 5 * time.Minute
	pageLimit             = 100
	maxPages              = 20
)

var WebhookEvents = []string{
	"incident.triggered", "incident.acknowledged", "incident.unacknowledged", "incident.resolved",
	"incident.reopened", "incident.reassigned", "incident.escalated", "incident.annotated", "incident.priority_updated",
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("REST API PagerDuty ответил %d: %s", e.Status, e.Message)
}

func (g *Gateway) rest(ctx context.Context, method, path string, q url.Values, body, out any) error {
	set := g.Settings()
	if set.APITokenRef == "" {
		return ErrNoAPIToken
	}
	tok, err := g.sec.Resolve(set.APITokenRef)
	if err != nil {
		return fmt.Errorf("API-токен PagerDuty: %w", err)
	}
	u := strings.TrimRight(set.API(), "/") + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token token="+tok)
	req.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("REST API PagerDuty недоступен: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error struct {
				Message string   `json:"message"`
				Errors  []string `json:"errors"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		msg := strings.TrimSpace(e.Error.Message + " " + strings.Join(e.Error.Errors, "; "))
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return &APIError{Status: resp.StatusCode, Message: msg}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

type CheckResult struct {
	OK        bool     `json:"ok"`
	Message   string   `json:"message"`
	Abilities []string `json:"abilities,omitempty"`
}

func (g *Gateway) CheckAPI(ctx context.Context) CheckResult {
	var out struct {
		Abilities []string `json:"abilities"`
	}
	if err := g.rest(ctx, http.MethodGet, "/abilities", nil, nil, &out); err != nil {
		return CheckResult{Message: err.Error()}
	}
	return CheckResult{OK: true, Message: fmt.Sprintf("токен принят, возможностей аккаунта: %d", len(out.Abilities)), Abilities: out.Abilities}
}

type Service struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	HTMLURL   string `json:"html_url"`
	Status    string `json:"status"`
	EventsKey bool   `json:"events_key"`
}

type integrationRef struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	Name           string `json:"name"`
	IntegrationKey string `json:"integration_key"`
}

type serviceObj struct {
	ID           string           `json:"id"`
	Name         string           `json:"summary"`
	FullName     string           `json:"name"`
	HTMLURL      string           `json:"html_url"`
	Status       string           `json:"status"`
	Integrations []integrationRef `json:"integrations"`
}

func (s serviceObj) name() string {
	if s.FullName != "" {
		return s.FullName
	}
	return s.Name
}

func (g *Gateway) Services(ctx context.Context) ([]Service, error) {
	var out []Service
	for page := 0; page < maxPages; page++ {
		q := url.Values{"limit": {strconv.Itoa(pageLimit)}, "offset": {strconv.Itoa(page * pageLimit)}, "include[]": {"integrations"}, "sort_by": {"name"}}
		var resp struct {
			Services []serviceObj `json:"services"`
			More     bool         `json:"more"`
		}
		if err := g.rest(ctx, http.MethodGet, "/services", q, nil, &resp); err != nil {
			return nil, err
		}
		for _, s := range resp.Services {
			has := false
			for _, in := range s.Integrations {
				if strings.HasPrefix(in.Type, eventsIntegrationType) {
					has = true
				}
			}
			out = append(out, Service{ID: s.ID, Name: s.name(), HTMLURL: s.HTMLURL, Status: s.Status, EventsKey: has})
		}
		if !resp.More {
			break
		}
	}
	return out, nil
}

func (g *Gateway) ServiceKey(ctx context.Context, serviceID string, create bool) (string, string, error) {
	var resp struct {
		Service serviceObj `json:"service"`
	}
	if err := g.rest(ctx, http.MethodGet, "/services/"+url.PathEscape(serviceID), url.Values{"include[]": {"integrations"}}, nil, &resp); err != nil {
		return "", "", err
	}
	name := resp.Service.name()
	for _, in := range resp.Service.Integrations {
		if !strings.HasPrefix(in.Type, eventsIntegrationType) {
			continue
		}
		if in.IntegrationKey != "" {
			return in.IntegrationKey, name, nil
		}
		var full struct {
			Integration integrationRef `json:"integration"`
		}
		if err := g.rest(ctx, http.MethodGet, "/services/"+url.PathEscape(serviceID)+"/integrations/"+url.PathEscape(in.ID), nil, nil, &full); err == nil && full.Integration.IntegrationKey != "" {
			return full.Integration.IntegrationKey, name, nil
		}
	}
	if !create {
		return "", name, fmt.Errorf("у сервиса %s нет интеграции Events API v2", name)
	}
	var created struct {
		Integration integrationRef `json:"integration"`
	}
	body := map[string]any{"integration": map[string]string{"type": eventsIntegrationType, "name": "Umbrella"}}
	if err := g.rest(ctx, http.MethodPost, "/services/"+url.PathEscape(serviceID)+"/integrations", nil, body, &created); err != nil {
		return "", name, fmt.Errorf("создание интеграции Events API v2 в сервисе %s: %w", name, err)
	}
	if created.Integration.IntegrationKey == "" {
		return "", name, errors.New("PagerDuty не вернул ключ новой интеграции")
	}
	return created.Integration.IntegrationKey, name, nil
}

type Policy struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url"`
}

func (g *Gateway) Policies(ctx context.Context) ([]Policy, error) {
	var out []Policy
	for page := 0; page < maxPages; page++ {
		q := url.Values{"limit": {strconv.Itoa(pageLimit)}, "offset": {strconv.Itoa(page * pageLimit)}, "sort_by": {"name"}}
		var resp struct {
			Policies []struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Summary string `json:"summary"`
				HTMLURL string `json:"html_url"`
			} `json:"escalation_policies"`
			More bool `json:"more"`
		}
		if err := g.rest(ctx, http.MethodGet, "/escalation_policies", q, nil, &resp); err != nil {
			return nil, err
		}
		for _, p := range resp.Policies {
			n := p.Name
			if n == "" {
				n = p.Summary
			}
			out = append(out, Policy{ID: p.ID, Name: n, HTMLURL: p.HTMLURL})
		}
		if !resp.More {
			break
		}
	}
	return out, nil
}

type ref struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Name    string `json:"name"`
	Email   string `json:"email"`
}

func (g *Gateway) SyncOnCall(ctx context.Context) error {
	set := g.Settings()
	q := url.Values{"limit": {strconv.Itoa(pageLimit)}, "include[]": {"users"}, "earliest": {"true"}}
	for _, p := range set.EscalationPolicies {
		q.Add("escalation_policy_ids[]", p)
	}
	var entries []model.OnCallEntry
	var err error
	for page := 0; page < maxPages; page++ {
		q.Set("offset", strconv.Itoa(page*pageLimit))
		var resp struct {
			OnCalls []struct {
				User             ref        `json:"user"`
				Schedule         *ref       `json:"schedule"`
				EscalationPolicy ref        `json:"escalation_policy"`
				EscalationLevel  int        `json:"escalation_level"`
				Start            *time.Time `json:"start"`
				End              *time.Time `json:"end"`
			} `json:"oncalls"`
			More bool `json:"more"`
		}
		if err = g.rest(ctx, http.MethodGet, "/oncalls", q, nil, &resp); err != nil {
			break
		}
		for _, o := range resp.OnCalls {
			e := model.OnCallEntry{PolicyID: o.EscalationPolicy.ID, PolicyName: o.EscalationPolicy.Summary, Level: o.EscalationLevel,
				UserID: o.User.ID, UserName: firstNonEmpty(o.User.Name, o.User.Summary), Email: o.User.Email, Start: o.Start, End: o.End}
			if o.Schedule != nil {
				e.Schedule = o.Schedule.Summary
			}
			entries = append(entries, e)
		}
		if !resp.More {
			break
		}
	}
	now := time.Now()
	g.st.Write(func(d *store.Data) {
		if err != nil {
			d.OnCall.Error = err.Error()
			return
		}
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].PolicyName != entries[j].PolicyName {
				return entries[i].PolicyName < entries[j].PolicyName
			}
			return entries[i].Level < entries[j].Level
		})
		d.OnCall = model.OnCall{Entries: entries, SyncedAt: &now}
	})
	return err
}

func (g *Gateway) Resync() {
	select {
	case g.resync <- struct{}{}:
	default:
	}
}

func (g *Gateway) RunSync(ctx context.Context) {
	tk := time.NewTicker(oncallEvery)
	defer tk.Stop()
	sync := func() {
		set := g.Settings()
		if !set.Enabled || set.APITokenRef == "" {
			return
		}
		sctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		if err := g.SyncOnCall(sctx); err != nil {
			slog.Warn("pagerduty on-call sync failed", "err", err)
		}
	}
	sync()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			sync()
		case <-g.resync:
			sync()
		}
	}
}

func (g *Gateway) IncidentAlertKeys(ctx context.Context, incidentID string) ([]string, error) {
	var resp struct {
		Alerts []struct {
			AlertKey string `json:"alert_key"`
		} `json:"alerts"`
	}
	if err := g.rest(ctx, http.MethodGet, "/incidents/"+url.PathEscape(incidentID)+"/alerts", url.Values{"limit": {"100"}}, nil, &resp); err != nil {
		return nil, err
	}
	var keys []string
	for _, a := range resp.Alerts {
		if a.AlertKey != "" {
			keys = append(keys, a.AlertKey)
		}
	}
	return keys, nil
}

func (g *Gateway) CreateWebhookSubscription(ctx context.Context, target string) (string, string, error) {
	body := map[string]any{"webhook_subscription": map[string]any{
		"type":            "webhook_subscription",
		"description":     "Umbrella: статусы инцидентов",
		"events":          WebhookEvents,
		"filter":          map[string]string{"type": "account_reference"},
		"delivery_method": map[string]any{"type": "http_delivery_method", "url": target},
	}}
	var resp struct {
		Sub struct {
			ID             string `json:"id"`
			DeliveryMethod struct {
				Secret string `json:"secret"`
			} `json:"delivery_method"`
		} `json:"webhook_subscription"`
	}
	if err := g.rest(ctx, http.MethodPost, "/webhook_subscriptions", nil, body, &resp); err != nil {
		return "", "", err
	}
	if resp.Sub.DeliveryMethod.Secret == "" {
		return resp.Sub.ID, "", errors.New("PagerDuty не вернул секрет подписи подписки")
	}
	return resp.Sub.ID, resp.Sub.DeliveryMethod.Secret, nil
}

func (g *Gateway) DeleteWebhookSubscription(ctx context.Context, id string) error {
	err := g.rest(ctx, http.MethodDelete, "/webhook_subscriptions/"+url.PathEscape(id), nil, nil, nil)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return nil
	}
	return err
}

func (g *Gateway) PingWebhookSubscription(ctx context.Context, id string) error {
	return g.rest(ctx, http.MethodPost, "/webhook_subscriptions/"+url.PathEscape(id)+"/ping", nil, map[string]any{}, nil)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
