package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pagerduty"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/secrets"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

const pdSecretPath = "pagerduty"

func (s *Server) audit(r *http.Request, action, object string) {
	s.st.Write(func(d *store.Data) {
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: action, Object: object})
	})
}

func (s *Server) pdView() map[string]any {
	set := s.pd.Settings()
	var oc model.OnCall
	s.st.Read(func(d *store.Data) { oc = d.OnCall })
	if oc.Entries == nil {
		oc.Entries = []model.OnCallEntry{}
	}
	if set.Routes == nil {
		set.Routes = []model.PDRoute{}
	}
	if set.EscalationPolicies == nil {
		set.EscalationPolicies = []string{}
	}
	return map[string]any{
		"settings":     set,
		"status":       s.pd.Status(),
		"oncall":       oc,
		"webhook_url":  strings.TrimRight(s.cfg.PublicURL, "/") + "/api/pagerduty/webhook",
		"events_url":   set.Events(),
		"api_url":      set.API(),
		"openbao":      s.vault.Enabled(),
		"webhook_list": pagerduty.WebhookEvents,
	}
}

func (s *Server) getPagerDuty(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, s.pdView())
}

type pdBody struct {
	Enabled            *bool     `json:"enabled"`
	Region             *string   `json:"region"`
	EventsURL          *string   `json:"events_url"`
	APIURL             *string   `json:"api_url"`
	MinSeverity        *string   `json:"min_severity"`
	EscalationPolicies *[]string `json:"escalation_policies"`
	RoutingKey         *string   `json:"routing_key"`
	RoutingKeyRef      *string   `json:"routing_key_ref"`
	APIToken           *string   `json:"api_token"`
	APITokenRef        *string   `json:"api_token_ref"`
	WebhookSecret      *string   `json:"webhook_secret"`
	WebhookSecretRef   *string   `json:"webhook_secret_ref"`
	ClearAPIToken      bool      `json:"clear_api_token"`
}

func checkURL(v string) error {
	if v == "" {
		return nil
	}
	u, err := url.Parse(v)
	if err != nil || u.Scheme != "https" && u.Scheme != "http" || u.Host == "" {
		return errors.New("адрес должен быть полным URL")
	}
	return nil
}

func (s *Server) secretValue(r *http.Request, key string, value, ref *string, cur string) (string, error) {
	if value != nil && *value != "" {
		return s.vault.PutRef(r.Context(), pdSecretPath, key, strings.TrimSpace(*value))
	}
	if ref != nil {
		v := strings.TrimSpace(*ref)
		if v == "" {
			return "", nil
		}
		if _, _, _, err := secrets.ParseRef(v); err != nil {
			return "", err
		}
		if _, err := s.vault.Resolve(v); err != nil {
			return "", err
		}
		return v, nil
	}
	return cur, nil
}

func (s *Server) putPagerDuty(w http.ResponseWriter, r *http.Request) {
	var b pdBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	set := s.pd.Settings()
	if b.Region != nil {
		if *b.Region != model.PDRegionUS && *b.Region != model.PDRegionEU {
			writeErr(w, 400, errors.New("регион: us или eu"))
			return
		}
		set.Region = *b.Region
	}
	for _, p := range []struct {
		in  *string
		out *string
	}{{b.EventsURL, &set.EventsURL}, {b.APIURL, &set.APIURL}} {
		if p.in != nil {
			v := strings.TrimSpace(*p.in)
			if err := checkURL(v); err != nil {
				writeErr(w, 400, err)
				return
			}
			*p.out = v
		}
	}
	if b.MinSeverity != nil {
		sev := model.Severity(*b.MinSeverity)
		if !sev.Valid() {
			writeErr(w, 400, errors.New("порог важности: critical, error, warning или info"))
			return
		}
		set.MinSeverity = sev
	}
	if b.EscalationPolicies != nil {
		set.EscalationPolicies = dedupe(*b.EscalationPolicies)
	}
	var err error
	if set.RoutingKeyRef, err = s.secretValue(r, "routing_key", b.RoutingKey, b.RoutingKeyRef, set.RoutingKeyRef); err != nil {
		writeErr(w, 400, err)
		return
	}
	if b.RoutingKey != nil && *b.RoutingKey != "" {
		set.ServiceID, set.ServiceName = "", ""
	}
	if set.APITokenRef, err = s.secretValue(r, "api_token", b.APIToken, b.APITokenRef, set.APITokenRef); err != nil {
		writeErr(w, 400, err)
		return
	}
	if b.ClearAPIToken {
		set.APITokenRef = ""
	}
	if set.WebhookSecretRef, err = s.secretValue(r, "webhook_secret", b.WebhookSecret, b.WebhookSecretRef, set.WebhookSecretRef); err != nil {
		writeErr(w, 400, err)
		return
	}
	if b.Enabled != nil {
		set.Enabled = *b.Enabled
	}
	if set.Enabled && set.RoutingKeyRef == "" && len(set.Routes) == 0 {
		writeErr(w, 400, errors.New("чтобы включить PagerDuty, задайте ключ интеграции Events API v2 или выберите сервис"))
		return
	}
	s.savePD(r, set, "pagerduty.update")
	s.pd.Resync()
	writeJSON(w, 200, s.pdView())
}

func (s *Server) savePD(r *http.Request, set model.PDSettings, action string) {
	now := time.Now()
	set.UpdatedAt, set.UpdatedBy = &now, actor(r)
	s.st.Write(func(d *store.Data) {
		d.PagerDuty = set
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: actor(r), Action: action, Object: "pagerduty"})
	})
}

func (s *Server) checkPagerDuty(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.pd.CheckAPI(r.Context()))
}

func (s *Server) testPagerDuty(w http.ResponseWriter, r *http.Request) {
	if err := s.pd.SendTest(r.Context()); err != nil {
		writeJSON(w, 200, pagerduty.CheckResult{Message: err.Error()})
		return
	}
	s.audit(r, "pagerduty.test_event", "pagerduty")
	writeJSON(w, 200, pagerduty.CheckResult{OK: true, Message: "тестовое событие принято и закрыто"})
}

func (s *Server) pdServices(w http.ResponseWriter, r *http.Request) {
	list, err := s.pd.Services(r.Context())
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	if list == nil {
		list = []pagerduty.Service{}
	}
	writeJSON(w, 200, map[string]any{"items": list})
}

func (s *Server) pdPolicies(w http.ResponseWriter, r *http.Request) {
	list, err := s.pd.Policies(r.Context())
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	if list == nil {
		list = []pagerduty.Policy{}
	}
	writeJSON(w, 200, map[string]any{"items": list})
}

func (s *Server) pdServiceKey(w http.ResponseWriter, r *http.Request) {
	var b struct {
		ServiceID string `json:"service_id"`
		Create    bool   `json:"create"`
	}
	if err := readJSON(r, &b); err != nil || b.ServiceID == "" {
		writeErr(w, 400, errors.New("нужен service_id"))
		return
	}
	key, name, err := s.pd.ServiceKey(r.Context(), b.ServiceID, b.Create)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	ref, err := s.vault.PutRef(r.Context(), pdSecretPath, "routing_key", key)
	if err != nil {
		writeErr(w, 503, err)
		return
	}
	set := s.pd.Settings()
	set.RoutingKeyRef, set.ServiceID, set.ServiceName = ref, b.ServiceID, name
	s.savePD(r, set, "pagerduty.service "+b.ServiceID)
	writeJSON(w, 200, s.pdView())
}

func (s *Server) pdSaveRoute(w http.ResponseWriter, r *http.Request) {
	var b struct {
		model.PDRoute
		RoutingKey string `json:"routing_key"`
		Create     bool   `json:"create"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	rt := b.PDRoute
	rt.Name = strings.TrimSpace(rt.Name)
	rt.Team, rt.Service = strings.TrimSpace(rt.Team), strings.TrimSpace(rt.Service)
	if rt.Name == "" || rt.Team == "" && rt.Service == "" {
		writeErr(w, 400, errors.New("маршруту нужны название и команда или ИТ-сервис"))
		return
	}
	set := s.pd.Settings()
	var cur *model.PDRoute
	if rt.ID != "" {
		for i := range set.Routes {
			if set.Routes[i].ID == rt.ID {
				cur = &set.Routes[i]
			}
		}
		if cur == nil {
			writeErr(w, 404, errors.New("маршрут не найден"))
			return
		}
	} else {
		s.st.Write(func(d *store.Data) { rt.ID = d.NextID("PDR") })
	}
	path := pdSecretPath + "/routes/" + rt.ID
	key := strings.TrimSpace(b.RoutingKey)
	if key == "" && rt.ServiceID != "" && (cur == nil || cur.ServiceID != rt.ServiceID) {
		k, name, err := s.pd.ServiceKey(r.Context(), rt.ServiceID, b.Create)
		if err != nil {
			writeErr(w, 502, err)
			return
		}
		key, rt.ServiceName = k, name
	}
	switch {
	case key != "":
		ref, err := s.vault.PutRef(r.Context(), path, "routing_key", key)
		if err != nil {
			writeErr(w, 503, err)
			return
		}
		rt.RoutingKeyRef = ref
	case cur != nil && rt.RoutingKeyRef == "":
		rt.RoutingKeyRef, rt.ServiceID, rt.ServiceName = cur.RoutingKeyRef, cur.ServiceID, cur.ServiceName
	case rt.RoutingKeyRef != "":
		if _, err := s.vault.Resolve(rt.RoutingKeyRef); err != nil {
			writeErr(w, 400, err)
			return
		}
	default:
		writeErr(w, 400, errors.New("укажите ключ интеграции или сервис PagerDuty"))
		return
	}
	if cur != nil {
		*cur = rt
	} else {
		set.Routes = append(set.Routes, rt)
	}
	s.savePD(r, set, "pagerduty.route "+rt.ID)
	writeJSON(w, 200, s.pdView())
}

func (s *Server) pdDeleteRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	set := s.pd.Settings()
	kept := set.Routes[:0]
	var gone *model.PDRoute
	for _, rt := range set.Routes {
		if rt.ID == id {
			c := rt
			gone = &c
			continue
		}
		kept = append(kept, rt)
	}
	if gone == nil {
		writeErr(w, 404, errors.New("маршрут не найден"))
		return
	}
	set.Routes = kept
	if s.vault.Owns(gone.RoutingKeyRef, pdSecretPath+"/routes/"+id) {
		_ = s.vault.Delete(r.Context(), pdSecretPath+"/routes/"+id)
	}
	s.savePD(r, set, "pagerduty.route.delete "+id)
	writeJSON(w, 200, s.pdView())
}

func (s *Server) pdCreateSubscription(w http.ResponseWriter, r *http.Request) {
	var b struct {
		URL string `json:"url"`
	}
	if r.ContentLength > 0 {
		_ = readJSON(r, &b)
	}
	target := strings.TrimSpace(b.URL)
	if target == "" {
		target = strings.TrimRight(s.cfg.PublicURL, "/") + "/api/pagerduty/webhook"
	}
	if u, err := url.Parse(target); err != nil || u.Scheme != "https" {
		writeErr(w, 400, errors.New("PagerDuty доставляет вебхуки только на https-адрес; задайте внешний https-адрес Umbrella (UMBRELLA_PUBLIC_URL)"))
		return
	}
	set := s.pd.Settings()
	if set.WebhookSubscriptionID != "" {
		_ = s.pd.DeleteWebhookSubscription(r.Context(), set.WebhookSubscriptionID)
	}
	id, secret, err := s.pd.CreateWebhookSubscription(r.Context(), target)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	ref, err := s.vault.PutRef(r.Context(), pdSecretPath, "webhook_secret", secret)
	if err != nil {
		writeErr(w, 503, err)
		return
	}
	set.WebhookSubscriptionID, set.WebhookSecretRef = id, ref
	s.savePD(r, set, "pagerduty.webhook_subscription "+id)
	writeJSON(w, 200, s.pdView())
}

func (s *Server) pdDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	set := s.pd.Settings()
	if set.WebhookSubscriptionID == "" {
		writeErr(w, 404, errors.New("подписка не создана из Umbrella"))
		return
	}
	if err := s.pd.DeleteWebhookSubscription(r.Context(), set.WebhookSubscriptionID); err != nil {
		writeErr(w, 502, err)
		return
	}
	set.WebhookSubscriptionID = ""
	s.savePD(r, set, "pagerduty.webhook_subscription.delete")
	writeJSON(w, 200, s.pdView())
}

func (s *Server) pdSyncOnCall(w http.ResponseWriter, r *http.Request) {
	if err := s.pd.SyncOnCall(r.Context()); err != nil {
		writeErr(w, 502, err)
		return
	}
	writeJSON(w, 200, s.pdView())
}
