package app_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/app"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

func TestGrafanaLink(t *testing.T) {
	opened := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	al := alert.Alert{ID: "INC-3", CIID: "CI-1", CIName: "db-01", Signal: "disk", OpenedAt: opened,
		Route: alert.Route{Services: []alert.Ref{{ID: "S-1", Name: "Billing"}}, Team: &alert.Ref{ID: "T-1", Name: "DBA"}}}
	if app.GrafanaLink(model.Grafana{}, al, opened) != "" {
		t.Fatal("a link without a dashboard")
	}
	set := model.Grafana{DashboardURL: "https://grafana.example/d/abc/incident?orgId=2", WindowMinute: 15}
	u, _ := url.Parse(app.GrafanaLink(set, al, opened.Add(time.Hour)))
	q := u.Query()
	if u.Path != "/d/abc/incident" || q.Get("orgId") != "2" || q.Get("from") != "1791193500000" || q.Get("to") != "now" ||
		q.Get("var-ci") != "db-01" || q.Get("var-ci_id") != "CI-1" || q.Get("var-service") != "Billing" || q.Get("var-team") != "DBA" ||
		q.Get("var-incident") != "INC-3" || q.Get("var-signal") != "disk" {
		t.Fatalf("link = %s", u)
	}
	resolved := opened.Add(20 * time.Minute)
	al.ResolvedAt = &resolved
	u, _ = url.Parse(app.GrafanaLink(set, al, opened.Add(time.Hour)))
	if u.Query().Get("to") != "1791196500000" {
		t.Fatalf("resolved link = %s", u)
	}
}

func TestGrafanaRedirect(t *testing.T) {
	f := newConnFixture(t, true)
	f.h.st.Write(func(d *store.Data) {
		d.ConfigItems["CI-1"] = &model.ConfigItem{ID: "CI-1", Name: "db-01", Status: model.CIStatusActive}
	})
	auth := f.webhookConnector()
	f.expect(f.admin, http.MethodPut, "/api/grafana", map[string]any{"dashboard_url": "ftp://x"}, http.StatusBadRequest, nil)
	var g model.Grafana
	f.expect(f.admin, http.MethodPut, "/api/grafana", map[string]any{"dashboard_url": "https://grafana.example/d/abc"}, http.StatusOK, &g)
	if g.WindowMinute != 10 {
		t.Fatalf("grafana = %+v", g)
	}
	f.ingest("hook", `{"id":"a1","title":"Disk full","host":"db-01","signal":"disk","severity":"warning","status":"firing"}`, auth)
	var page struct {
		Alerts []alert.Alert `json:"alerts"`
	}
	waitFor(t, "the incident", func() bool {
		f.admin.call(http.MethodGet, "/api/incidents", nil, &page)
		return len(page.Alerts) == 1
	})
	id := page.Alerts[0].ID
	var detail struct {
		Grafana string `json:"grafana_url"`
	}
	f.admin.call(http.MethodGet, "/api/incidents/"+id, nil, &detail)
	if u, _ := url.Parse(detail.Grafana); u == nil || u.Host != "grafana.example" || u.Query().Get("var-ci") != "db-01" {
		t.Fatalf("grafana_url = %q", detail.Grafana)
	}

	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noFollow.Get(f.h.srv.URL + "/go/incidents/" + id + "/grafana")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); resp.StatusCode != http.StatusFound || loc != "/incidents?id="+id {
		t.Fatalf("anonymous: %d %s", resp.StatusCode, loc)
	}
	req, _ := http.NewRequest(http.MethodGet, f.h.srv.URL+"/go/incidents/"+id+"/grafana", nil)
	for _, c := range f.admin.http.Jar.Cookies(req.URL) {
		req.AddCookie(c)
	}
	resp, err = noFollow.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if u, _ := url.Parse(resp.Header.Get("Location")); resp.StatusCode != http.StatusFound || u.Host != "grafana.example" {
		t.Fatalf("signed in: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}
