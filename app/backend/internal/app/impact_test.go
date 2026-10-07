package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/app"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/flow"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

func impactCatalog(st *store.Store) {
	st.Write(func(d *store.Data) {
		d.Teams["T-1"] = &model.Team{ID: "T-1", Name: "DBA"}
		d.ConfigItems["CI-1"] = &model.ConfigItem{ID: "CI-1", Name: "db-01", Kind: model.CIKindVM, Status: model.CIStatusActive}
		d.Services["S-1"] = &model.Service{ID: "S-1", Name: "Billing", OwnerTeamID: "T-1", Criticality: model.CriticalityMedium,
			Status: model.ServiceActive, CIIDs: []string{"CI-1"}}
		d.Services["S-2"] = &model.Service{ID: "S-2", Name: "Checkout", Criticality: model.CriticalityCritical,
			Status: model.ServiceActive, DependsOn: []string{"S-1"}}
	})
}

func TestImpactSettings(t *testing.T) {
	h := newHarness(t)
	impactCatalog(h.st)
	h.addLocal("admin", "Admin-pass-2026", model.RoleAdmin, time.Now())
	h.addLocal("viewer", "Viewer-pass-2026", model.RoleUser, time.Now())
	user := h.client()
	user.login("viewer", "Viewer-pass-2026")
	if code := user.call(http.MethodGet, "/api/impact", nil, nil); code != http.StatusForbidden {
		t.Fatalf("user reading the policy = %d", code)
	}
	admin := h.client()
	admin.login("admin", "Admin-pass-2026")

	// Never saved: the template applies.
	var view app.ImpactView
	if code := admin.call(http.MethodGet, "/api/impact", nil, &view); code != 200 || view.Policy.Configured || !view.Policy.Enabled ||
		view.Policy.UpstreamDepth != 3 || len(view.Levels) != 5 || len(view.Severities) != 5 || view.Defaults.Matrix["warning"]["critical"] != "error" {
		t.Fatalf("view = %d %+v", code, view)
	}

	// A P3 event on db-01 affects Checkout upstream: P2.
	var pv alert.ImpactPreview
	ev := alert.ImpactEvent{Title: "Disk full", CI: "db-01", Signal: "disk", Severity: "warning"}
	if code := admin.call(http.MethodPost, "/api/impact/preview", app.ImpactPreviewInput{Event: ev}, &pv); code != 200 ||
		pv.Severity != "error" || pv.EventSeverity != "warning" || pv.Impact.Level != "critical" || pv.CI == nil || pv.Team == nil || pv.Team.Name != "DBA" {
		t.Fatalf("preview = %d %+v", code, pv)
	}
	// An unsaved policy can be tried.
	p := view.Policy
	p.Rules = []model.ClassificationRule{{ID: "db", Name: "DB", Enabled: true, When: "ci.startsWith('db-')", Action: "set", Severity: "critical"}}
	if code := admin.call(http.MethodPost, "/api/impact/preview", app.ImpactPreviewInput{Event: ev, Policy: &p}, &pv); code != 200 ||
		pv.Severity != "critical" || pv.Impact.Rule != "DB" {
		t.Fatalf("preview of a draft = %d %+v", code, pv)
	}
	var problem map[string]any
	if code := admin.call(http.MethodPost, "/api/impact/preview", app.ImpactPreviewInput{Event: alert.ImpactEvent{CI: "db-01", Severity: "P9"}}, &problem); code != 400 {
		t.Fatalf("bad severity = %d %v", code, problem)
	}

	// A rule that does not compile is refused, and nothing is saved.
	bad := p
	bad.Rules = []model.ClassificationRule{{ID: "x", Name: "Broken", Enabled: true, When: "ci ==", Action: "raise", N: 1}}
	if code := admin.call(http.MethodPut, "/api/impact", bad, &problem); code != 400 || problem["error"] != "invalid_impact_policy" {
		t.Fatalf("broken rule = %d %v", code, problem)
	}
	bad.Rules[0].When, bad.Rules[0].N = "true", 9
	if code := admin.call(http.MethodPut, "/api/impact", bad, &problem); code != 400 {
		t.Fatalf("bad number of levels = %d %v", code, problem)
	}

	// Saved: configured, audited, the preview uses it.
	p.Enabled, p.UpstreamDepth = true, 0
	if code := admin.call(http.MethodPut, "/api/impact", p, &view); code != 200 || !view.Policy.Configured || len(view.Policy.Rules) != 1 || view.Policy.UpstreamDepth != 0 {
		t.Fatalf("save = %d %+v", code, view)
	}
	h.st.Read(func(d *store.Data) {
		if d.Audit[len(d.Audit)-1].Action != "settings.impact" || !d.Settings.Impact.Configured {
			t.Fatalf("not audited: %+v", d.Audit[len(d.Audit)-1])
		}
	})
	if admin.call(http.MethodPost, "/api/impact/preview", app.ImpactPreviewInput{Event: ev}, &pv); pv.Severity != "critical" || pv.Impact.Level != "medium" {
		t.Fatalf("preview of the saved policy = %+v", pv)
	}
	if code := user.call(http.MethodPut, "/api/impact", p, nil); code != http.StatusForbidden {
		t.Fatalf("user saving = %d", code)
	}

	// Back to the template.
	if code := admin.call(http.MethodPost, "/api/impact/reset-defaults", nil, &view); code != 200 || len(view.Policy.Rules) != 1 ||
		view.Policy.Rules[0].Enabled || view.Policy.UpstreamDepth != 3 || !view.Policy.Configured {
		t.Fatalf("reset = %d %+v", code, view)
	}
}

// Saving the policy finds the priority of the open incidents again at once.
func TestImpactAppliesToIncidents(t *testing.T) {
	f := newConnFixture(t, true)
	impactCatalog(f.h.st)
	cred := f.credential(flow.CredBearer, nil, map[string]string{"token": "tkn-1"})
	var c app.ConnectorView
	f.expect(f.admin, http.MethodPost, "/api/connectors", map[string]any{"name": "Webhook", "slug": "hook", "preset": "webhook",
		"credentials": map[string]string{"webhook.credential": cred.ID}}, http.StatusCreated, &c)
	f.expect(f.admin, http.MethodPost, "/api/connectors/"+c.ID+"/publish", map[string]any{}, http.StatusOK, nil)
	auth := map[string]string{"Authorization": "Bearer tkn-1", "Content-Type": "application/json"}
	f.ingest("hook", `{"id":"a1","title":"Disk full","host":"db-01","signal":"disk","severity":"warning","status":"firing"}`, auth)
	var page alert.Page
	waitFor(t, "an incident", func() bool {
		f.admin.call(http.MethodGet, "/api/incidents?status=active", nil, &page)
		return len(page.Alerts) == 1
	})
	inc := page.Alerts[0]
	if inc.Severity != "error" || inc.EventSeverity != "warning" || inc.Impact == nil || inc.Impact.Level != "critical" {
		t.Fatalf("incident = %s/%s %+v", inc.EventSeverity, inc.Severity, inc.Impact)
	}
	var view app.ImpactView
	f.expect(f.admin, http.MethodGet, "/api/impact", nil, http.StatusOK, &view)
	p := view.Policy
	p.Enabled = false
	f.expect(f.admin, http.MethodPut, "/api/impact", p, http.StatusOK, &view)
	var one struct {
		Alert    alert.Alert   `json:"alert"`
		Timeline []alert.Entry `json:"timeline"`
	}
	f.expect(f.admin, http.MethodGet, "/api/incidents/"+inc.ID, nil, http.StatusOK, &one)
	if one.Alert.Severity != "warning" || one.Alert.Impact.Level != "critical" {
		t.Fatalf("after disabling: %+v", one.Alert)
	}
	n := 0
	for _, e := range one.Timeline {
		if e.Code == "priority_set" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("priority_set entries = %d", n)
	}
}
