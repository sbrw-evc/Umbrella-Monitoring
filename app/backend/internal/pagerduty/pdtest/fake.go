// Package pdtest is a fake PagerDuty: the Events API v2 and the part of the REST API Umbrella
// uses.
package pdtest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pagerduty"
)

const (
	Token     = "pd-token"
	ServiceID = "PSVC1"
	// ServiceKey is the Events API key of the integration the fake creates for ServiceID.
	ServiceKey = "svc-key-1"
	Secret     = "whsec-1"
)

type Fake struct {
	Server *httptest.Server

	mu     sync.Mutex
	events []pagerduty.Event
	// EventsStatus answers Events API calls with this status when not zero.
	EventsStatus   int
	Subscriptions  map[string]string
	integration    bool
	IncidentAlerts map[string][]string
}

func New(t *testing.T) *Fake {
	t.Helper()
	f := &Fake{Subscriptions: map[string]string{}, IncidentAlerts: map[string][]string{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Server.Close)
	return f
}

func (f *Fake) EventsURL() string { return f.Server.URL + "/v2/enqueue" }

func (f *Fake) APIURL() string { return f.Server.URL + "/api" }

func (f *Fake) Events() []pagerduty.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]pagerduty.Event(nil), f.events...)
}

func (f *Fake) SetEventsStatus(code int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.EventsStatus = code
}

// Sign signs a webhook body the way PagerDuty does.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path == "/v2/enqueue" {
		if f.EventsStatus != 0 {
			write(w, f.EventsStatus, map[string]any{"status": "error", "message": "unavailable"})
			return
		}
		var ev pagerduty.Event
		if err := json.NewDecoder(r.Body).Decode(&ev); err != nil || ev.RoutingKey == "" || ev.DedupKey == "" {
			write(w, http.StatusBadRequest, map[string]any{"status": "invalid event", "message": "Event object is invalid", "errors": []string{"bad"}})
			return
		}
		f.events = append(f.events, ev)
		write(w, http.StatusAccepted, map[string]any{"status": "success", "dedup_key": ev.DedupKey})
		return
	}
	path, ok := strings.CutPrefix(r.URL.Path, "/api")
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Header.Get("Authorization") != "Token token="+Token {
		write(w, http.StatusUnauthorized, map[string]any{"error": map[string]any{"message": "Unauthorized"}})
		return
	}
	integrations := func() []map[string]string {
		if !f.integration {
			return []map[string]string{}
		}
		return []map[string]string{{"id": "PINT1", "type": "events_api_v2_inbound_integration", "integration_key": ServiceKey}}
	}
	switch {
	case path == "/abilities":
		write(w, http.StatusOK, map[string]any{"abilities": []string{"teams", "urgencies"}})
	case path == "/services":
		write(w, http.StatusOK, map[string]any{"services": []map[string]any{{"id": ServiceID, "name": "Payments", "status": "active",
			"html_url": "https://pd/services/" + ServiceID, "integrations": integrations()}}, "more": false})
	case path == "/services/"+ServiceID && r.Method == http.MethodGet:
		write(w, http.StatusOK, map[string]any{"service": map[string]any{"id": ServiceID, "name": "Payments", "integrations": integrations()}})
	case path == "/services/"+ServiceID+"/integrations" && r.Method == http.MethodPost:
		f.integration = true
		write(w, http.StatusCreated, map[string]any{"integration": integrations()[0]})
	case path == "/webhook_subscriptions" && r.Method == http.MethodPost:
		var body struct {
			Sub struct {
				DeliveryMethod struct {
					URL string `json:"url"`
				} `json:"delivery_method"`
			} `json:"webhook_subscription"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		id := "PSUB1"
		f.Subscriptions[id] = body.Sub.DeliveryMethod.URL
		write(w, http.StatusCreated, map[string]any{"webhook_subscription": map[string]any{"id": id, "delivery_method": map[string]any{"secret": Secret}}})
	case strings.HasPrefix(path, "/webhook_subscriptions/") && r.Method == http.MethodDelete:
		delete(f.Subscriptions, strings.TrimPrefix(path, "/webhook_subscriptions/"))
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(path, "/incidents/") && strings.HasSuffix(path, "/alerts"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/incidents/"), "/alerts")
		var alerts []map[string]string
		for _, k := range f.IncidentAlerts[id] {
			alerts = append(alerts, map[string]string{"alert_key": k})
		}
		write(w, http.StatusOK, map[string]any{"alerts": alerts})
	default:
		write(w, http.StatusNotFound, map[string]any{"error": map[string]any{"message": "Not Found"}})
	}
}
