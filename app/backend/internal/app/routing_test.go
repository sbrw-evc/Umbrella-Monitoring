package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/app"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// routeCatalog: db-01 belongs to Payments (critical, team SRE) and Reports (low, team BI).
func routeCatalog(h *harness) {
	now := time.Now()
	h.st.Write(func(d *store.Data) {
		d.EnsureSystemRoles(now)
		d.Teams["T-1"] = &model.Team{ID: "T-1", Name: "SRE", LeadID: "U-1"}
		d.Teams["T-2"] = &model.Team{ID: "T-2", Name: "BI"}
		d.Users["U-1"] = &model.User{ID: "U-1", Username: "lead", Source: model.SourceLocal, Role: model.RoleUser, Profile: model.Profile{Email: "lead@example.com"}}
		d.ConfigItems["CI-1"] = &model.ConfigItem{ID: "CI-1", Name: "db-01", Kind: model.CIKindVM, Status: model.CIStatusActive}
		d.Services["S-1"] = &model.Service{ID: "S-1", Name: "Payments", OwnerTeamID: "T-1", Criticality: model.CriticalityCritical, Status: model.ServiceActive, CIIDs: []string{"CI-1"}}
		d.Services["S-2"] = &model.Service{ID: "S-2", Name: "Reports", OwnerTeamID: "T-2", Criticality: model.CriticalityLow, Status: model.ServiceActive, CIIDs: []string{"CI-1"}}
	})
}

func TestRoutePreviewAPI(t *testing.T) {
	h := newHarness(t)
	routeCatalog(h)
	h.addLocal("admin", "Admin-pass-2026", model.RoleAdmin, time.Now())
	h.addLocal("nobody", "Nobody-pass-2026", model.RoleUser, time.Now())
	admin := h.client()
	admin.login("admin", "Admin-pass-2026")

	var p app.RoutePreview
	if code := admin.call(http.MethodGet, "/api/cis/CI-1/route", nil, &p); code != 200 {
		t.Fatalf("ci route = %d", code)
	}
	if p.Service == nil || p.Service.ID != "S-1" || p.Team == nil || p.Team.Name != "SRE" || len(p.People) != 1 || p.People[0].UserID != "U-1" ||
		p.Via != "service" || len(p.Services) != 2 || p.PagerDuty != nil {
		t.Fatalf("ci route = %+v", p)
	}

	// With PagerDuty on, the preview names the route a trigger would take.
	h.st.Write(func(d *store.Data) {
		d.Settings.Alerting.PagerDuty = model.PagerDuty{Enabled: true, MinSeverity: "error", RoutingKeyRef: "openbao://x", Routes: []model.PDRoute{
			{ID: "R-1", Name: "BI on-call", TeamID: "T-2", RoutingKeyRef: "openbao://y"},
			{ID: "R-2", Name: "SRE on-call", TeamID: "T-1", RoutingKeyRef: "openbao://z"},
		}}
	})
	admin.call(http.MethodGet, "/api/cis/CI-1/route", nil, &p)
	if p.PagerDuty == nil || p.PagerDuty.ID != "R-2" || p.PagerDuty.Name != "SRE on-call" || p.PagerDuty.MinSeverity != "error" {
		t.Fatalf("pagerduty route = %+v", p.PagerDuty)
	}

	if code := admin.call(http.MethodGet, "/api/services/S-2/route", nil, &p); code != 200 {
		t.Fatalf("service route = %d", code)
	}
	if p.Team == nil || p.Team.ID != "T-2" || p.PagerDuty == nil || p.PagerDuty.ID != "R-1" || len(p.Elsewhere) != 1 || p.Elsewhere[0].Service.ID != "S-1" {
		t.Fatalf("service route = %+v", p)
	}
	if code := admin.call(http.MethodGet, "/api/cis/CI-9/route", nil, nil); code != 404 {
		t.Fatalf("unknown = %d", code)
	}
	nobody := h.client()
	nobody.login("nobody", "Nobody-pass-2026")
	if code := nobody.call(http.MethodGet, "/api/cis/CI-1/route", nil, nil); code != 403 {
		t.Fatalf("without cis:view = %d", code)
	}
}
