package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/app"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/response"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

func TestResponseSettingsAndIncidentActions(t *testing.T) {
	f := newConnFixture(t, true)
	f.h.st.Write(func(d *store.Data) {
		d.Teams["T-1"] = &model.Team{ID: "T-1", Name: "Payments SRE"}
		d.ConfigItems["CI-1"] = &model.ConfigItem{ID: "CI-1", Name: "pay-01", Status: model.CIStatusActive}
		d.Services["S-1"] = &model.Service{ID: "S-1", Name: "Payments", OwnerTeamID: "T-1", Criticality: model.CriticalityCritical, Status: model.ServiceActive, CIIDs: []string{"CI-1"}}
		for _, u := range d.Users {
			if u.Username == "admin" {
				u.Email, u.TeamIDs = "admin@example.com", []string{"T-1"}
				d.Teams["T-1"].LeadID = u.ID
			}
		}
	})
	var v app.ResponseView
	f.expect(f.admin, http.MethodGet, "/api/response", nil, http.StatusOK, &v)
	if v.Mode != model.ModeOff || len(v.Policies) != 5 || v.Impact.Matrix[model.ImpactExtensive][model.SeverityError] != model.SeverityCritical || v.Jira.Mode != model.ModeOff {
		t.Fatalf("defaults = %+v", v)
	}

	// The mode, the impact policy and the policies are checked.
	f.expect(f.admin, http.MethodPut, "/api/response", map[string]any{"mode": "loud"}, http.StatusBadRequest, nil)
	bad := []model.ResponsePolicy{{Priority: model.SeverityCritical, Steps: []model.EscalationStep{{Methods: []string{"pigeon"}}}}}
	f.expect(f.admin, http.MethodPut, "/api/response", map[string]any{"mode": "live", "policies": bad}, http.StatusBadRequest, nil)
	pols := v.Policies
	pols[0].Steps[0].UserIDs = []string{"USR-NOPE"}
	f.expect(f.admin, http.MethodPut, "/api/response", app.ResponsePolicyInput{Mode: model.ModeDryRun, Impact: v.Impact, Policies: pols}, http.StatusOK, &v)
	if v.Mode != model.ModeDryRun || v.ActiveSince == nil || len(v.Policies[0].Steps[0].UserIDs) != 0 {
		t.Fatalf("saved = %+v", v)
	}

	// Jira: live needs the site, the account, the project and the token; the token goes to OpenBao.
	jira := map[string]any{"mode": "live", "base_url": "https://corp.atlassian.net", "email": "bot@example.com", "project": "ops"}
	f.expect(f.admin, http.MethodPut, "/api/response/jira", jira, http.StatusBadRequest, nil)
	jira["base_url"] = "http://corp.atlassian.net"
	jira["token"] = "jira-token"
	f.expect(f.admin, http.MethodPut, "/api/response/jira", jira, http.StatusBadRequest, nil)
	jira["base_url"] = "https://corp.atlassian.net/"
	f.expect(f.admin, http.MethodPut, "/api/response/jira", jira, http.StatusOK, &v)
	if !v.Jira.HasToken || v.Jira.Project != "OPS" || v.Jira.BaseURL != "https://corp.atlassian.net" || v.Jira.TaskType != "Task" {
		t.Fatalf("jira = %+v", v.Jira)
	}
	if f.h.bao.Get("umbrella/response") == nil {
		t.Errorf("the token is in OpenBao: %v", f.h.bao.Paths())
	}
	delete(jira, "token")
	jira["mode"] = "dry_run"
	f.expect(f.admin, http.MethodPut, "/api/response/jira", jira, http.StatusOK, &v)
	if !v.Jira.HasToken || v.Jira.Mode != model.ModeDryRun {
		t.Fatalf("the token is kept: %+v", v.Jira)
	}
	f.expect(f.admin, http.MethodPut, "/api/response/graph", map[string]any{"mode": "live", "tenant_id": "corp"}, http.StatusBadRequest, nil)
	f.expect(f.admin, http.MethodPut, "/api/response/graph", map[string]any{"mode": "dry_run", "tenant_id": "corp.onmicrosoft.com", "client_id": "00000000-0000-0000-0000-000000000001",
		"client_secret": "s", "refresh_token": "r"}, http.StatusOK, &v)
	if !v.Graph.HasSecret || !v.Graph.HasRefresh || v.Graph.Mode != model.ModeDryRun {
		t.Fatalf("graph = %+v", v.Graph)
	}
	f.expect(f.admin, http.MethodPut, "/api/response/zoom", map[string]any{"mode": "dry_run", "account_id": "acc", "client_id": "cid", "client_secret": "z"}, http.StatusOK, &v)
	if !v.Zoom.HasSecret || v.Zoom.User != "me" {
		t.Fatalf("zoom = %+v", v.Zoom)
	}

	// The simulation of an incident of Payments: critical service, a RED high incident is P1.
	var sim app.SimulateView
	f.expect(f.admin, http.MethodPost, "/api/response/simulate", map[string]any{"severity": "error", "service_id": "S-1", "method": "red"}, http.StatusOK, &sim)
	if sim.Assessment.Priority != model.SeverityCritical || sim.Policy == nil || len(sim.Steps) != 3 || len(sim.Steps[0].People) != 1 || len(sim.Room) != 1 {
		t.Fatalf("simulation = %+v", sim)
	}
	f.expect(f.admin, http.MethodPost, "/api/response/simulate", map[string]any{"severity": "loud"}, http.StatusBadRequest, nil)

	// An incident: response assesses it in dry run and a person asks for the postmortem by hand.
	auth := f.webhookConnector()
	f.ingest("hook", `{"id":"a1","title":"5xx","host":"pay-01","signal":"http","severity":"error","status":"firing"}`, auth)
	var page struct {
		Alerts []alert.Alert `json:"alerts"`
	}
	waitFor(t, "the incident", func() bool {
		f.admin.call(http.MethodGet, "/api/incidents", nil, &page)
		return len(page.Alerts) == 1
	})
	id := page.Alerts[0].ID
	var ir struct {
		Mode  string          `json:"mode"`
		State *response.State `json:"state"`
	}
	f.expect(f.admin, http.MethodPost, "/api/incidents/"+id+"/response/assess", nil, http.StatusOK, &ir)
	if ir.State == nil || ir.State.Priority != model.SeverityCritical || ir.State.Task == nil || !ir.State.Task.DryRun || ir.State.Room == nil {
		t.Fatalf("state = %+v", ir.State)
	}
	f.expect(f.admin, http.MethodPost, "/api/incidents/"+id+"/response/fly", nil, http.StatusBadRequest, nil)
	f.expect(f.admin, http.MethodPost, "/api/incidents/"+id+"/response/postmortem", nil, http.StatusOK, &ir)
	f.expect(f.admin, http.MethodPost, "/api/incidents/"+id+"/resolve", map[string]any{}, http.StatusOK, nil)
	f.expect(f.admin, http.MethodPost, "/api/incidents/"+id+"/response/postmortem", nil, http.StatusOK, &ir)
	if ir.State.Postmortem == nil || ir.State.Postmortem.Key != "DRY-PM-"+id {
		t.Fatalf("postmortem = %+v", ir.State)
	}
	f.expect(f.admin, http.MethodGet, "/api/incidents/"+id+"/response", nil, http.StatusOK, &ir)
	if ir.Mode != model.ModeDryRun || ir.State == nil {
		t.Fatalf("get = %+v", ir)
	}
	f.expect(f.admin, http.MethodGet, "/api/incidents/INC-404/response", nil, http.StatusNotFound, nil)

	// A viewer sees neither the settings nor the actions.
	f.h.addLocal("viewer", "Viewer-pass-2026", model.RoleViewer, time.Now())
	viewer := f.h.client()
	viewer.login("viewer", "Viewer-pass-2026")
	if got := viewer.call(http.MethodPut, "/api/response", map[string]any{"mode": "off"}, nil); got != http.StatusForbidden {
		t.Fatalf("viewer PUT = %d", got)
	}
}
