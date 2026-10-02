package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/connector"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/demo"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pagerduty"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type nopPD struct{}

func (nopPD) Send(alert.PDCommand) {}

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	st := store.New()
	demo.Seed(st)
	eng := alert.New(st, nopPD{}, nil)
	rt := connector.New(st, eng, connector.EnvSecrets{}, nil)
	srv := New(Config{}, st, eng, rt, pagerduty.New(pagerduty.Config{}), NewHub())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func do(t *testing.T, method, url, body string, out any) int {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
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
