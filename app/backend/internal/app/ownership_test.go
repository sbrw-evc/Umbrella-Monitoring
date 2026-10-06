package app_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// A wallboard for a supporting team shows the incidents of the services the team supports.
func TestWallboardTeamFilterIncludesSupportingTeams(t *testing.T) {
	f := newConnFixture(t, true)
	seedCatalog(f.h.st)
	f.h.st.Write(func(d *store.Data) {
		d.Teams["T-2"] = &model.Team{ID: "T-2", Name: "Support"}
		d.Teams["T-3"] = &model.Team{ID: "T-3", Name: "Unrelated"}
		d.Services["S-1"].TeamIDs = []string{"T-2"}
	})
	hook := f.webhookConnector()
	f.ingest("hook", `{"id":"a1","title":"Disk full","host":"db-01","signal":"disk","severity":"critical","status":"firing"}`, hook)
	f.expect(f.admin, http.MethodPost, "/api/wallboards", wallboardBody(map[string]any{"slug": "support", "team_ids": []string{"T-2"}}), http.StatusCreated, nil)
	f.expect(f.admin, http.MethodPost, "/api/wallboards", wallboardBody(map[string]any{"slug": "other", "team_ids": []string{"T-3"}}), http.StatusCreated, nil)
	var payload struct {
		Ready     bool `json:"ready"`
		Incidents []map[string]any
	}
	waitFor(t, "the supporting team's board", func() bool {
		r := fetch(t, f.h, "/api/public/tv/support", nil)
		_ = json.Unmarshal([]byte(r.body), &payload)
		return payload.Ready && len(payload.Incidents) == 1
	})
	if payload.Incidents[0]["team"] != "DBA" {
		t.Fatalf("routing stays with the owner team: %v", payload.Incidents[0])
	}
	r := fetch(t, f.h, "/api/public/tv/other", nil)
	_ = json.Unmarshal([]byte(r.body), &payload)
	if !payload.Ready || len(payload.Incidents) != 0 {
		t.Fatalf("an unrelated team sees nothing: %s", r.body)
	}
}

func TestTeamChannelSettings(t *testing.T) {
	f := newConnFixture(t, false)
	seedCatalog(f.h.st)
	var team map[string]any
	f.expect(f.admin, http.MethodPut, "/api/teams/T-1", map[string]any{"email": "not-mail"}, http.StatusBadRequest, nil)
	f.expect(f.admin, http.MethodPut, "/api/teams/T-1", map[string]any{"telegram": "chat?"}, http.StatusBadRequest, nil)
	f.expect(f.admin, http.MethodPut, "/api/teams/T-1", map[string]any{"email": " dba@example.com ", "telegram": "-100123"}, http.StatusOK, &team)
	if team["email"] != "dba@example.com" || team["telegram"] != "-100123" {
		t.Fatalf("team = %v", team)
	}
}
