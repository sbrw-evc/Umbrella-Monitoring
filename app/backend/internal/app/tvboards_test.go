package app_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/app"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/flow"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type tvScreen struct {
	Name      string `json:"name"`
	Locale    string `json:"locale"`
	Timezone  string `json:"timezone"`
	Theme     string `json:"theme"`
	Refresh   int    `json:"refresh"`
	Error     string `json:"error"`
	IP        string `json:"ip"`
	Incidents []struct {
		ID       string   `json:"id"`
		CI       string   `json:"ci"`
		Severity string   `json:"severity"`
		Services []string `json:"services"`
		Team     string   `json:"team"`
		Route    any      `json:"route"`
	} `json:"incidents"`
}

// screen opens a board the way a TV does: no cookies, optionally behind a proxy.
func screen(t *testing.T, h *harness, slug, xff string) (int, tvScreen) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, h.srv.URL+"/api/tv/"+slug, nil)
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out tvScreen
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestTVBoardsManage(t *testing.T) {
	f := newConnFixture(t, false)
	f.h.st.Write(func(d *store.Data) {
		d.ConfigItems["CI-1"] = &model.ConfigItem{ID: "CI-1", Name: "db-01", Status: model.CIStatusActive}
		d.Teams["T-1"] = &model.Team{ID: "T-1", Name: "DBA"}
	})
	bad := []map[string]any{
		{"name": "", "allowed_sources": []string{"10.0.0.0/8"}},
		{"name": "Hall", "allowed_sources": []string{}},
		{"name": "Hall", "allowed_sources": []string{"10.0.0.0/33"}},
		{"name": "Hall", "allowed_sources": []string{"10.0.0.1"}, "refresh": 2},
		{"name": "Hall", "allowed_sources": []string{"10.0.0.1"}, "locale": "de"},
		{"name": "Hall", "allowed_sources": []string{"10.0.0.1"}, "timezone": "Mars/Base"},
		{"name": "Hall", "allowed_sources": []string{"10.0.0.1"}, "severities": []string{"fatal"}},
		{"name": "Hall", "allowed_sources": []string{"10.0.0.1"}, "ci_ids": []string{"CI-404"}},
	}
	for _, body := range bad {
		f.expect(f.admin, http.MethodPost, "/api/tv-boards", body, http.StatusBadRequest, nil)
	}
	var b app.TVBoardView
	f.expect(f.admin, http.MethodPost, "/api/tv-boards", map[string]any{"name": "Hall", "ci_ids": []string{"CI-1"}, "team_ids": []string{"T-1"},
		"allowed_sources": []string{"10.0.0.15/8", " 10.0.0.15/8", "192.168.1.7"}, "refresh": 30, "locale": "ru", "timezone": "Europe/Moscow"}, http.StatusCreated, &b)
	if len(b.Slug) < 30 || b.Theme != "dark" || b.Refresh != 30 || b.Locale != "ru" || len(b.AllowedSources) != 2 || b.AllowedSources[0] != "10.0.0.0/8" || b.CIs[0].Name != "db-01" || b.Teams[0].Name != "DBA" {
		t.Fatalf("board = %+v", b)
	}
	var page struct {
		Boards []app.TVBoardView `json:"boards"`
		YourIP string            `json:"your_ip"`
	}
	f.expect(f.admin, http.MethodGet, "/api/tv-boards", nil, http.StatusOK, &page)
	if len(page.Boards) != 1 || page.YourIP != "127.0.0.1" {
		t.Errorf("page = %+v", page)
	}

	// The screen is refused outside the allowed networks and tells its own address.
	code, out := screen(t, f.h, b.Slug, "")
	if code != http.StatusForbidden || out.Error != "address_not_allowed" || out.IP != "127.0.0.1" {
		t.Errorf("outside = %d %+v", code, out)
	}
	// A forged X-Forwarded-For does not help while no proxy is trusted.
	if code, _ := screen(t, f.h, b.Slug, "10.1.2.3"); code != http.StatusForbidden {
		t.Errorf("forged header = %d", code)
	}
	// Behind a trusted proxy the forwarded address is checked.
	f.expect(f.admin, http.MethodPut, "/api/tv-boards/settings", map[string]any{"trusted_proxies": []string{"127.0.0.1"}}, http.StatusOK, nil)
	// Allowed; no PostgreSQL here, so the screen hears that data is not ready yet.
	if code, _ := screen(t, f.h, b.Slug, "10.1.2.3"); code != http.StatusServiceUnavailable {
		t.Errorf("via proxy = %d", code)
	}
	if code, _ := screen(t, f.h, b.Slug, "172.16.0.1"); code != http.StatusForbidden {
		t.Errorf("via proxy, outside = %d", code)
	}
	if code, _ := screen(t, f.h, "nope", ""); code != http.StatusNotFound {
		t.Errorf("unknown board = %d", code)
	}

	// A new address cuts off the old one.
	old := b.Slug
	f.expect(f.admin, http.MethodPost, "/api/tv-boards/"+b.ID+"/rotate", nil, http.StatusOK, &b)
	if code, _ := screen(t, f.h, old, "10.1.2.3"); code != http.StatusNotFound || b.Slug == old {
		t.Errorf("old address = %d", code)
	}
	// A disabled board is not found.
	f.expect(f.admin, http.MethodPut, "/api/tv-boards/"+b.ID, map[string]any{"name": "Hall", "allowed_sources": []string{"10.0.0.0/8"}, "disabled": true}, http.StatusOK, nil)
	if code, _ := screen(t, f.h, b.Slug, "10.1.2.3"); code != http.StatusNotFound {
		t.Errorf("disabled = %d", code)
	}

	// Managing boards needs the permission.
	f.h.addLocal("viewer", "Viewer-pass-2026-x", model.RoleUser, time.Now())
	viewer := f.h.client()
	viewer.login("viewer", "Viewer-pass-2026-x")
	if code := viewer.call(http.MethodGet, "/api/tv-boards", nil, nil); code != http.StatusForbidden {
		t.Errorf("without permission = %d", code)
	}
	f.expect(f.admin, http.MethodDelete, "/api/tv-boards/"+b.ID, nil, http.StatusNoContent, nil)
	f.expect(f.admin, http.MethodDelete, "/api/tv-boards/"+b.ID, nil, http.StatusNotFound, nil)
}

func TestTVBoardScreen(t *testing.T) {
	f := newConnFixture(t, true)
	f.h.st.Write(func(d *store.Data) {
		d.Teams["T-1"] = &model.Team{ID: "T-1", Name: "DBA"}
		d.ConfigItems["CI-1"] = &model.ConfigItem{ID: "CI-1", Name: "db-01", Status: model.CIStatusActive}
		d.ConfigItems["CI-2"] = &model.ConfigItem{ID: "CI-2", Name: "web-01", Status: model.CIStatusActive}
		d.ConfigItems["CI-3"] = &model.ConfigItem{ID: "CI-3", Name: "mq-01", Status: model.CIStatusActive}
		d.Services["S-1"] = &model.Service{ID: "S-1", Name: "Billing", OwnerTeamID: "T-1", Status: model.ServiceActive, CIIDs: []string{"CI-1"}}
	})
	cred := f.credential(flow.CredBearer, nil, map[string]string{"token": "tkn-1"})
	var c app.ConnectorView
	f.expect(f.admin, http.MethodPost, "/api/connectors", map[string]any{"name": "Webhook", "slug": "hook", "preset": "webhook",
		"credentials": map[string]string{"webhook.credential": cred.ID}}, http.StatusCreated, &c)
	f.expect(f.admin, http.MethodPost, "/api/connectors/"+c.ID+"/publish", map[string]any{}, http.StatusOK, nil)
	auth := map[string]string{"Authorization": "Bearer tkn-1", "Content-Type": "application/json"}
	f.ingest("hook", `{"id":"a1","title":"Slow queries","host":"db-01","signal":"latency","severity":"warning","status":"firing"}`, auth)
	f.ingest("hook", `{"id":"a2","title":"Disk full","host":"db-01","signal":"disk","severity":"critical","status":"firing"}`, auth)
	f.ingest("hook", `{"id":"a3","title":"5xx","host":"web-01","signal":"errors","severity":"error","status":"firing"}`, auth)
	f.ingest("hook", `{"id":"a4","title":"Queue","host":"mq-01","signal":"queue","severity":"critical","status":"firing"}`, auth)

	// Scoped to the Billing service and to web-01: mq-01 stays off the screen.
	var b app.TVBoardView
	f.expect(f.admin, http.MethodPost, "/api/tv-boards", map[string]any{"name": "Billing wall", "service_ids": []string{"S-1"}, "ci_ids": []string{"CI-2"},
		"allowed_sources": []string{"127.0.0.0/8"}, "show_acknowledged": true}, http.StatusCreated, &b)
	var out tvScreen
	waitFor(t, "three incidents on the screen", func() bool {
		_, out = screen(t, f.h, b.Slug, "")
		return len(out.Incidents) == 3
	})
	got := []string{}
	for _, i := range out.Incidents {
		got = append(got, i.CI+"/"+i.Severity)
		if i.Route != nil {
			t.Errorf("routing details leak to the screen: %+v", i)
		}
	}
	if want := "db-01/critical web-01/error db-01/warning"; strings.Join(got, " ") != want {
		t.Errorf("order = %s, want %s", strings.Join(got, " "), want)
	}
	if out.Incidents[0].Team != "DBA" || len(out.Incidents[0].Services) != 1 || out.Theme != "dark" || out.Refresh != 15 || out.Timezone != "UTC" {
		t.Errorf("screen = %+v", out)
	}

	// Severity filter, and acknowledged incidents hidden.
	var page struct {
		Alerts []struct {
			ID     string `json:"id"`
			CIName string `json:"ci_name"`
			Signal string `json:"signal"`
		} `json:"alerts"`
	}
	f.expect(f.admin, http.MethodGet, "/api/incidents?status=active&ci=CI-1", nil, http.StatusOK, &page)
	for _, a := range page.Alerts {
		if a.Signal == "disk" {
			f.expect(f.admin, http.MethodPost, "/api/incidents/"+a.ID+"/ack", nil, http.StatusOK, nil)
		}
	}
	f.expect(f.admin, http.MethodPut, "/api/tv-boards/"+b.ID, map[string]any{"name": "Billing wall", "service_ids": []string{"S-1"}, "ci_ids": []string{"CI-2"},
		"allowed_sources": []string{"127.0.0.0/8"}, "severities": []string{"critical", "error"}, "locale": "ru", "refresh": 60}, http.StatusOK, nil)
	_, out = screen(t, f.h, b.Slug, "")
	if len(out.Incidents) != 1 || out.Incidents[0].CI != "web-01" || out.Locale != "ru" || out.Refresh != 60 {
		t.Errorf("filtered = %+v", out)
	}
}
