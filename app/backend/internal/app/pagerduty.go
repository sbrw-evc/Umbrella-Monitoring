package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/credentials"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pagerduty"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

const (
	pdSecretPath      = "pagerduty"
	pdWebhookPath     = "/api/pagerduty/webhook"
	maxPDRoutes       = 200
	maxWebhookBody    = 1 << 20
	pdRoutesSecretDir = "pagerduty/routes"
)

// PagerDutyService keeps the PagerDuty settings and their secrets in OpenBao.
type PagerDutyService struct {
	st      *store.Store
	secrets Secrets
	gw      *pagerduty.Gateway
}

func NewPagerDutyService(st *store.Store, secrets Secrets, gw *pagerduty.Gateway) *PagerDutyService {
	return &PagerDutyService{st: st, secrets: secrets, gw: gw}
}

type PDRouteView struct {
	model.PDRoute
	HasKey bool `json:"has_key"`
}

type PagerDutyView struct {
	model.PagerDuty
	Routes           []PDRouteView    `json:"routes"`
	HasRoutingKey    bool             `json:"has_routing_key"`
	HasAPIToken      bool             `json:"has_api_token"`
	HasWebhookSecret bool             `json:"has_webhook_secret"`
	PublicURL        string           `json:"public_url"`
	WebhookURL       string           `json:"webhook_url"`
	Status           pagerduty.Status `json:"status"`
}

func (s *PagerDutyService) View() PagerDutyView {
	var al model.Alerting
	s.st.Read(func(d *store.Data) {
		al = d.Settings.Alerting
		al.PagerDuty.Routes = slices.Clone(d.Settings.Alerting.PagerDuty.Routes)
	})
	pd := al.PagerDuty
	v := PagerDutyView{PagerDuty: pd, Routes: []PDRouteView{}, HasRoutingKey: pd.RoutingKeyRef != "", HasAPIToken: pd.APITokenRef != "",
		HasWebhookSecret: pd.WebhookSecretRef != "", PublicURL: al.PublicURL, Status: s.gw.Status()}
	if v.Region == "" {
		v.Region = model.PDRegionUS
	}
	if al.PublicURL != "" {
		v.WebhookURL = strings.TrimRight(al.PublicURL, "/") + pdWebhookPath
	}
	for _, r := range pd.Routes {
		v.Routes = append(v.Routes, PDRouteView{PDRoute: r, HasKey: r.RoutingKeyRef != ""})
	}
	v.PagerDuty.Routes = nil
	return v
}

type PDRouteInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	TeamID      string `json:"team_id"`
	ServiceID   string `json:"service_id"`
	RoutingKey  string `json:"routing_key"`
	PDServiceID string `json:"pd_service_id"`
}

type PagerDutyInput struct {
	Enabled     bool   `json:"enabled"`
	Region      string `json:"region"`
	MinSeverity string `json:"min_severity"`
	// PublicURL is kept for older clients; nil keeps the address set in the «Umbrella address» card.
	PublicURL *string `json:"public_url,omitempty"`
	EventsURL string  `json:"events_url"`
	APIURL    string  `json:"api_url"`
	// Write-only secrets: empty keeps the stored one.
	RoutingKey    string         `json:"routing_key"`
	PDServiceID   string         `json:"pd_service_id"`
	APIToken      string         `json:"api_token"`
	ClearAPIToken bool           `json:"clear_api_token"`
	WebhookSecret string         `json:"webhook_secret"`
	Routes        []PDRouteInput `json:"routes"`
}

// NormalizePublicURL checks the address Umbrella is reached at.
func NormalizePublicURL(v string) (string, error) { return model.NormalizePublicURL(v) }

func optionalURL(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%q is not an http or https URL", v)
	}
	return v, nil
}

func (s *PagerDutyService) put(ctx context.Context, path, key, value string) (string, error) {
	if s.secrets == nil {
		return "", credentials.ErrUnavailable
	}
	ref, err := s.secrets.PutRef(ctx, path, key, value)
	if err != nil {
		return "", fmt.Errorf("%w: %v", credentials.ErrUnavailable, err)
	}
	return ref, nil
}

// Save validates and stores the settings. Keys picked from a PagerDuty service are fetched
// (or the integration created) with the REST API token.
func (s *PagerDutyService) Save(ctx context.Context, actor string, in PagerDutyInput) (PagerDutyView, error) {
	var cur model.Alerting
	var teams, services map[string]bool
	s.st.Read(func(d *store.Data) {
		cur = d.Settings.Alerting
		cur.PagerDuty.Routes = slices.Clone(d.Settings.Alerting.PagerDuty.Routes)
		teams, services = map[string]bool{}, map[string]bool{}
		for id := range d.Teams {
			teams[id] = true
		}
		for id := range d.Services {
			services[id] = true
		}
	})
	pd := cur.PagerDuty
	var err error
	pub := cur.PublicURL
	if in.PublicURL != nil {
		if pub, err = NormalizePublicURL(*in.PublicURL); err != nil {
			return PagerDutyView{}, invalid("public_url_invalid", err)
		}
	}
	switch in.Region {
	case "", model.PDRegionUS, model.PDRegionEU:
	default:
		return PagerDutyView{}, invalid("region_invalid", nil)
	}
	if in.MinSeverity != "" && alert.SeverityRank(in.MinSeverity) == 0 {
		return PagerDutyView{}, invalid("severity_invalid", nil)
	}
	if pd.EventsURL, err = optionalURL(in.EventsURL); err != nil {
		return PagerDutyView{}, invalid("url_invalid", err)
	}
	if pd.APIURL, err = optionalURL(in.APIURL); err != nil {
		return PagerDutyView{}, invalid("url_invalid", err)
	}
	if len(in.Routes) > maxPDRoutes {
		return PagerDutyView{}, invalid("too_many_routes", nil)
	}
	pd.Enabled, pd.Region, pd.MinSeverity = in.Enabled, in.Region, in.MinSeverity
	if pd.Region == "" {
		pd.Region = model.PDRegionUS
	}
	type pending struct{ path, key, value string }
	// Secrets are written only after everything is checked.
	var secrets []pending
	var setRefs []func(ref string)
	stage := func(path, key, value string, set func(ref string)) {
		secrets = append(secrets, pending{path, key, value})
		setRefs = append(setRefs, set)
	}
	token := strings.TrimSpace(in.APIToken)
	if token != "" {
		stage(pdSecretPath, "api_token", token, func(ref string) { pd.APITokenRef = ref })
	} else if in.ClearAPIToken {
		pd.APITokenRef = ""
	}
	// serviceKey fetches the key of a PagerDuty service with the token being saved.
	serviceKey := func(serviceID string) (string, string, error) {
		key, name, err := s.gw.ServiceKey(ctx, pd, token, serviceID, true)
		if err != nil {
			return "", "", invalid("pagerduty_failed", err)
		}
		return key, name, nil
	}
	if key := strings.TrimSpace(in.RoutingKey); key != "" {
		stage(pdSecretPath, "routing_key", key, func(ref string) { pd.RoutingKeyRef = ref })
		pd.ServiceID, pd.ServiceName = "", ""
	} else if in.PDServiceID != "" && in.PDServiceID != pd.ServiceID {
		key, name, err := serviceKey(in.PDServiceID)
		if err != nil {
			return PagerDutyView{}, err
		}
		stage(pdSecretPath, "routing_key", key, func(ref string) { pd.RoutingKeyRef = ref })
		pd.ServiceID, pd.ServiceName = in.PDServiceID, name
	}
	if sec := strings.TrimSpace(in.WebhookSecret); sec != "" {
		stage(pdSecretPath, "webhook_secret", sec, func(ref string) { pd.WebhookSecretRef = ref })
	}
	old := map[string]model.PDRoute{}
	for _, r := range pd.Routes {
		old[r.ID] = r
	}
	routes := make([]model.PDRoute, len(in.Routes))
	for i, ri := range in.Routes {
		r := model.PDRoute{ID: strings.TrimSpace(ri.ID), Name: strings.TrimSpace(ri.Name), TeamID: ri.TeamID, ServiceID: ri.ServiceID}
		if r.ID == "" || old[r.ID].ID == "" {
			r.ID = "PDR-" + strings.ToLower(auth.RandomToken("", 4))
		} else {
			prev := old[r.ID]
			r.RoutingKeyRef, r.PDServiceID, r.PDServiceName = prev.RoutingKeyRef, prev.PDServiceID, prev.PDServiceName
		}
		if r.Name == "" || len(r.Name) > 200 {
			return PagerDutyView{}, invalid("route_name", nil)
		}
		if r.TeamID == "" && r.ServiceID == "" {
			return PagerDutyView{}, invalid("route_match", fmt.Errorf("route %q matches nothing: choose a team or a service", r.Name))
		}
		if (r.TeamID != "" && !teams[r.TeamID]) || (r.ServiceID != "" && !services[r.ServiceID]) {
			return PagerDutyView{}, invalid("route_target", fmt.Errorf("route %q refers to a team or service that does not exist", r.Name))
		}
		hasKey := r.RoutingKeyRef != ""
		idx := i
		if key := strings.TrimSpace(ri.RoutingKey); key != "" {
			stage(pdRoutesSecretDir, r.ID, key, func(ref string) { routes[idx].RoutingKeyRef = ref })
			r.PDServiceID, r.PDServiceName, hasKey = "", "", true
		} else if ri.PDServiceID != "" && ri.PDServiceID != r.PDServiceID {
			key, name, err := serviceKey(ri.PDServiceID)
			if err != nil {
				return PagerDutyView{}, err
			}
			stage(pdRoutesSecretDir, r.ID, key, func(ref string) { routes[idx].RoutingKeyRef = ref })
			r.PDServiceID, r.PDServiceName, hasKey = ri.PDServiceID, name, true
		}
		if !hasKey {
			return PagerDutyView{}, invalid("route_key", fmt.Errorf("route %q has no integration key", r.Name))
		}
		routes[i] = r
	}
	hasDefault := pd.RoutingKeyRef != "" || slices.ContainsFunc(secrets, func(p pending) bool { return p.path == pdSecretPath && p.key == "routing_key" })
	if pd.Enabled && !hasDefault && len(routes) == 0 {
		return PagerDutyView{}, invalid("routing_key_required", nil)
	}
	for i, p := range secrets {
		ref, err := s.put(ctx, p.path, p.key, p.value)
		if err != nil {
			return PagerDutyView{}, err
		}
		setRefs[i](ref)
	}
	pd.Routes = routes
	now := time.Now().UTC()
	pd.UpdatedAt, pd.UpdatedBy = &now, actor
	s.st.Write(func(d *store.Data) {
		d.Settings.Alerting.PagerDuty = pd
		d.Settings.Alerting.PublicURL = pub
		d.AddAudit(store.AuditEntry{Actor: actor, Action: "settings.pagerduty", Detail: fmt.Sprintf("enabled=%v routes=%d", pd.Enabled, len(routes))})
	})
	return s.View(), nil
}

// Subscribe creates the Webhooks v3 subscription that brings incident changes back and keeps
// its signing secret in OpenBao.
func (s *PagerDutyService) Subscribe(ctx context.Context, actor string) (PagerDutyView, error) {
	v := s.View()
	if v.WebhookURL == "" {
		return v, invalid("public_url_required", nil)
	}
	if v.WebhookSubscriptionID != "" {
		if err := s.gw.DeleteWebhookSubscription(ctx, v.WebhookSubscriptionID); err != nil {
			return v, invalid("pagerduty_failed", err)
		}
	}
	id, secret, err := s.gw.CreateWebhookSubscription(ctx, v.WebhookURL)
	if err != nil {
		return v, invalid("pagerduty_failed", err)
	}
	ref, err := s.put(ctx, pdSecretPath, "webhook_secret", secret)
	if err != nil {
		return v, err
	}
	s.st.Write(func(d *store.Data) {
		d.Settings.Alerting.PagerDuty.WebhookSubscriptionID = id
		d.Settings.Alerting.PagerDuty.WebhookSecretRef = ref
		d.AddAudit(store.AuditEntry{Actor: actor, Action: "settings.pagerduty", Detail: "webhook subscription " + id})
	})
	return s.View(), nil
}

func (s *PagerDutyService) Unsubscribe(ctx context.Context, actor string) (PagerDutyView, error) {
	v := s.View()
	if v.WebhookSubscriptionID != "" {
		if err := s.gw.DeleteWebhookSubscription(ctx, v.WebhookSubscriptionID); err != nil {
			return v, invalid("pagerduty_failed", err)
		}
	}
	s.st.Write(func(d *store.Data) {
		d.Settings.Alerting.PagerDuty.WebhookSubscriptionID = ""
		d.Settings.Alerting.PagerDuty.WebhookSecretRef = ""
		d.AddAudit(store.AuditEntry{Actor: actor, Action: "settings.pagerduty", Detail: "webhook subscription removed"})
	})
	return s.View(), nil
}

func (a *App) registerPagerDuty(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/pagerduty", a.authed(a.can("settings.alerting:view", a.pdView)))
	mux.HandleFunc("PUT /api/pagerduty", a.authed(a.can("settings.alerting:edit", a.pdSave)))
	mux.HandleFunc("POST /api/pagerduty/test", a.authed(a.can("settings.alerting:test", a.pdTest)))
	mux.HandleFunc("POST /api/pagerduty/check", a.authed(a.can("settings.alerting:test", a.pdCheck)))
	mux.HandleFunc("GET /api/pagerduty/services", a.authed(a.can("settings.alerting:edit", a.pdServices)))
	mux.HandleFunc("POST /api/pagerduty/subscription", a.authed(a.can("settings.alerting:edit", a.pdSubscribe)))
	mux.HandleFunc("DELETE /api/pagerduty/subscription", a.authed(a.can("settings.alerting:edit", a.pdUnsubscribe)))
	mux.HandleFunc("POST "+pdWebhookPath, a.pdWebhook)
}

func (a *App) pdView(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, a.pagerduty.View())
}

func (a *App) pdSave(w http.ResponseWriter, r *http.Request) {
	var in PagerDutyInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	out, err := a.pagerduty.Save(r.Context(), current(r).user.Username, in)
	settingsRespond(w, out, err)
}

type pdTestInput struct {
	RoutingKey string `json:"routing_key"`
	APIToken   string `json:"api_token"`
}

func (a *App) pdTest(w http.ResponseWriter, r *http.Request) {
	var in pdTestInput
	if r.ContentLength != 0 && !httpx.Decode(w, r, &in) {
		return
	}
	if err := a.pdGateway.SendTest(r.Context(), strings.TrimSpace(in.RoutingKey)); err != nil {
		httpx.Error(w, http.StatusBadGateway, "pagerduty_failed", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) pdCheck(w http.ResponseWriter, r *http.Request) {
	var in pdTestInput
	if r.ContentLength != 0 && !httpx.Decode(w, r, &in) {
		return
	}
	out, err := a.pdGateway.CheckAPI(r.Context(), strings.TrimSpace(in.APIToken))
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "pagerduty_failed", err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *App) pdServices(w http.ResponseWriter, r *http.Request) {
	out, err := a.pdGateway.Services(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "pagerduty_failed", err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (a *App) pdSubscribe(w http.ResponseWriter, r *http.Request) {
	out, err := a.pagerduty.Subscribe(r.Context(), current(r).user.Username)
	settingsRespond(w, out, err)
}

func (a *App) pdUnsubscribe(w http.ResponseWriter, r *http.Request) {
	out, err := a.pagerduty.Unsubscribe(r.Context(), current(r).user.Username)
	settingsRespond(w, out, err)
}

// pdWebhook takes Webhooks v3 deliveries. They are checked by signature, not by session.
func (a *App) pdWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
	if err != nil {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "body_too_large", nil)
		return
	}
	if a.alerts == nil || !a.ingestReady() {
		w.Header().Set("Retry-After", "10")
		httpx.Error(w, http.StatusServiceUnavailable, "alerts_unavailable", nil)
		return
	}
	n, err := a.pdGateway.HandleWebhook(r.Context(), body, r.Header.Get("X-PagerDuty-Signature"))
	switch {
	case errors.Is(err, pagerduty.ErrNoWebhookSecret), errors.Is(err, pagerduty.ErrBadSignature):
		httpx.Error(w, http.StatusUnauthorized, "bad_signature", nil)
	case err != nil:
		slog.Warn("pagerduty webhook not applied", "err", err)
		httpx.Error(w, http.StatusBadRequest, "bad_request", nil)
	default:
		httpx.JSON(w, http.StatusOK, map[string]int{"applied": n})
	}
}

func settingsRespond(w http.ResponseWriter, out any, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
