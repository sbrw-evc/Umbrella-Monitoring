package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/connector"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/demo"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/notify"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pagerduty"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type nopPD struct{}

func (nopPD) Send(alert.PDCommand) {}

const adminPassword = "Admin-pass-2026"

func init() { auth.Iterations = 1000 }

// token is the bearer token of the current test's administrator.
var token string

func newServerStore(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	st := store.New()
	demo.Seed(st)
	auth.Bootstrap(st, auth.BootstrapConfig{AdminPassword: adminPassword})
	n := notify.New(notify.Config{AllowHTTP: true, Backoff: time.Millisecond}, st, connector.EnvSecrets{})
	eng := alert.New(st, nopPD{}, n.Observe)
	rt := connector.New(st, eng, connector.EnvSecrets{}, n.Observe)
	srv := New(Config{AllowHTTPWebhooks: true}, st, eng, rt, pagerduty.New(pagerduty.Config{}), NewHub())
	srv.SetNotifier(n)
	go n.Run(t.Context())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	token = login(t, ts, "admin", adminPassword)
	return ts, st
}

func newServer(t *testing.T) *httptest.Server {
	ts, _ := newServerStore(t)
	return ts
}

func login(t *testing.T, ts *httptest.Server, user, password string) string {
	t.Helper()
	var out struct{ Token string }
	saved := token
	token = ""
	code := do(t, "POST", ts.URL+"/api/auth/token", `{"username":"`+user+`","password":"`+password+`"}`, &out)
	token = saved
	if code != 200 || out.Token == "" {
		t.Fatalf("login %s: code %d", user, code)
	}
	return out.Token
}

func do(t *testing.T, method, url, body string, out any) int {
	t.Helper()
	return doAs(t, token, method, url, body, out)
}

func doAs(t *testing.T, bearer, method, url, body string, out any) int {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func TestIngestToIncidentAndAck(t *testing.T) {
	ts := newServer(t)
	body := `{"id":"x1","service":"Платёжный шлюз","signal":"red.errors","severity":"critical","state":"firing","title":"Ошибки 9%"}`
	if code := do(t, "POST", ts.URL+"/api/ingest/CON-4", body, nil); code != 202 {
		t.Fatalf("ingest code = %d", code)
	}
	var list struct {
		Items []struct {
			ID, Service, Status string
		}
		Counts map[string]int
	}
	do(t, "GET", ts.URL+"/api/incidents?view=open", "", &list)
	if len(list.Items) != 1 || list.Items[0].Service != "Платёжный шлюз" || list.Counts["critical"] != 1 {
		t.Fatalf("list = %+v", list)
	}
	id := list.Items[0].ID
	if code := do(t, "POST", ts.URL+"/api/incidents/"+id+"/ack", "", nil); code != 200 {
		t.Fatalf("ack code = %d", code)
	}
	if code := do(t, "POST", ts.URL+"/api/incidents/"+id+"/ack", "", nil); code != 409 {
		t.Fatalf("second ack code = %d", code)
	}
	do(t, "GET", ts.URL+"/api/incidents?view=open&q=ошибки", "", &list)
	if len(list.Items) != 1 || list.Items[0].Status != "acknowledged" {
		t.Fatalf("after ack = %+v", list)
	}
}

func TestStoppedConnectorRejects(t *testing.T) {
	ts := newServer(t)
	do(t, "POST", ts.URL+"/api/connectors/CON-1/stop", "", nil)
	if code := do(t, "POST", ts.URL+"/api/ingest/CON-1", `{"alerts":[]}`, nil); code != 409 {
		t.Fatalf("code = %d", code)
	}
	if code := do(t, "POST", ts.URL+"/api/ingest/NOPE", `{}`, nil); code != 404 {
		t.Fatalf("code = %d", code)
	}
}

func TestPublishValidates(t *testing.T) {
	ts := newServer(t)
	var c struct{ ID string }
	do(t, "POST", ts.URL+"/api/connectors", `{"name":"t","template":"webhook-json"}`, &c)
	if code := do(t, "PUT", ts.URL+"/api/connectors/"+c.ID, `{"draft":{"nodes":[{"id":"a","kind":"parse.json"}],"edges":[]}}`, nil); code != 200 {
		t.Fatalf("put code = %d", code)
	}
	if code := do(t, "POST", ts.URL+"/api/connectors/"+c.ID+"/publish", "", nil); code != 422 {
		t.Fatalf("publish invalid code = %d", code)
	}
}
