package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/connector"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/integration"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pagerduty"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/rules"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/secrets/secretstest"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

func freshServer(t *testing.T, dir string) (*httptest.Server, *store.Store) {
	t.Helper()
	st := store.New()
	if _, err := st.Open(dir); err != nil {
		t.Fatal(err)
	}
	_, vault := secretstest.New(t)
	pd := pagerduty.New(pagerduty.Config{}, st, vault)
	eng := alert.New(st, pd, nil)
	im := integration.NewManager(st, vault, "")
	srv := New(Config{DataDir: dir, SetupToken: "code-123", Version: "test"}, Deps{Store: st, Engine: eng,
		Runtime: connector.New(st, eng, vault, nil), PagerDuty: pd, Hub: NewHub(), Vault: vault, Integrations: im, Rules: rules.New(st, im, eng)})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, st
}

func setupCall(t *testing.T, url, code, body string, out any) int {
	t.Helper()
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	if code != "" {
		req.Header.Set("X-Umbrella-Setup-Token", code)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = jsonDecode(resp, out)
	}
	return resp.StatusCode
}

func TestSetupWizardFirstRun(t *testing.T) {
	ts, st := freshServer(t, t.TempDir())
	var status struct {
		Required      bool `json:"required"`
		TokenRequired bool `json:"token_required"`
		AdminExists   bool `json:"admin_exists"`
	}
	doAs(t, "", "GET", ts.URL+"/api/setup/status", "", &status)
	if !status.Required || !status.TokenRequired || status.AdminExists {
		t.Fatalf("status = %+v", status)
	}
	body := `{"theme":"dark","locale":"en","storage":{"kind":"file"},"admin":{"username":"root","name":"Root","password":"Strong-pass-2026"}}`
	if code := setupCall(t, ts.URL+"/api/setup/complete", "", body, nil); code != 401 {
		t.Fatalf("without code = %d", code)
	}
	if code := setupCall(t, ts.URL+"/api/setup/complete", "wrong", body, nil); code != 401 {
		t.Fatalf("wrong code = %d", code)
	}
	if code := setupCall(t, ts.URL+"/api/setup/complete", "code-123", `{"admin":{"username":"root","password":"short"}}`, nil); code != 400 {
		t.Fatalf("weak password = %d", code)
	}
	var res struct {
		OK           bool   `json:"ok"`
		AdminCreated string `json:"admin_created"`
	}
	if code := setupCall(t, ts.URL+"/api/setup/complete", "code-123", body, &res); code != 200 || res.AdminCreated != "root" {
		t.Fatalf("complete = %d %+v", code, res)
	}
	if code := setupCall(t, ts.URL+"/api/setup/complete", "code-123", body, nil); code != 409 {
		t.Fatalf("second run = %d", code)
	}
	var def struct{ Theme, Locale string }
	doAs(t, "", "GET", ts.URL+"/api/ui-defaults", "", &def)
	if def.Theme != "dark" || def.Locale != "en" {
		t.Fatalf("defaults = %+v", def)
	}
	login(t, ts, "root", "Strong-pass-2026")
	st.Read(func(d *store.Data) {
		if !d.Settings.SetupCompleted {
			t.Fatal("setup not marked done")
		}
	})
}

func TestSetupAfterBootstrapNeedsAdmin(t *testing.T) {
	env := newEnv(t, false)
	if code := setupCall(t, env.ts.URL+"/api/setup/complete", "", `{"theme":"light","locale":"ru","storage":{"kind":"keep"}}`, nil); code != 401 {
		t.Fatalf("anonymous = %d", code)
	}
	if code := do(t, "POST", env.ts.URL+"/api/setup/complete", `{"theme":"light","locale":"ru","storage":{"kind":"keep"}}`, nil); code != 200 {
		t.Fatalf("admin = %d", code)
	}
	if code := do(t, "PUT", env.ts.URL+"/api/settings", `{"default_theme":"neon"}`, nil); code != 400 {
		t.Fatalf("bad theme = %d", code)
	}
	if code := do(t, "PUT", env.ts.URL+"/api/settings", `{"default_theme":"dark","default_locale":"en"}`, nil); code != 200 {
		t.Fatalf("defaults = %d", code)
	}
}

func TestTeams(t *testing.T) {
	ts := newServer(t)
	if code := do(t, "POST", ts.URL+"/api/teams", `{"id":"Bad Id","name":"x"}`, nil); code != 400 {
		t.Fatalf("bad id = %d", code)
	}
	if code := do(t, "POST", ts.URL+"/api/teams", `{"id":"payments","name":"Платежи","email":"pay@corp","members":["USR-1"],"leads":["USR-1"]}`, nil); code != 201 {
		t.Fatalf("create = %d", code)
	}
	if code := do(t, "POST", ts.URL+"/api/teams", `{"id":"payments","name":"dup"}`, nil); code != 400 {
		t.Fatalf("duplicate = %d", code)
	}
	if code := do(t, "PUT", ts.URL+"/api/teams/payments", `{"members":["USR-404"]}`, nil); code != 400 {
		t.Fatalf("unknown member = %d", code)
	}
	if code := do(t, "PUT", ts.URL+"/api/teams/payments", `{"description":"Платёжные сервисы"}`, nil); code != 200 {
		t.Fatalf("update = %d", code)
	}
	var list struct {
		Items []struct {
			ID      string
			Managed bool
			CIs     int `json:"cis"`
			Members []string
		}
	}
	do(t, "GET", ts.URL+"/api/teams", "", &list)
	found := false
	for _, it := range list.Items {
		if it.ID == "payments" {
			found = it.Managed && it.CIs > 0 && len(it.Members) == 1
		}
	}
	if !found {
		t.Fatalf("teams = %+v", list.Items)
	}
	if code := do(t, "DELETE", ts.URL+"/api/teams/payments", "", nil); code != 409 {
		t.Fatalf("delete used = %d", code)
	}
	if code := do(t, "DELETE", ts.URL+"/api/teams/payments?detach=1", "", nil); code != 204 {
		t.Fatalf("delete detach = %d", code)
	}
}

func TestDeleteIncidentAndParseErrors(t *testing.T) {
	env := newEnv(t, true)
	do(t, "POST", env.ts.URL+"/api/ingest/CON-4", `{"id":"del1","service":"Платёжный шлюз","signal":"red.errors","severity":"critical","state":"firing","title":"x"}`, nil)
	do(t, "POST", env.ts.URL+"/api/ingest/CON-4", `{broken`, nil)
	var id string
	env.st.Read(func(d *store.Data) {
		for _, a := range d.Alerts {
			id = a.ID
		}
	})
	if code := do(t, "DELETE", env.ts.URL+"/api/incidents/"+id, "", nil); code != 200 {
		t.Fatalf("delete incident = %d", code)
	}
	if code := do(t, "DELETE", env.ts.URL+"/api/parse-errors?connector=CON-4", "", nil); code != 200 {
		t.Fatalf("delete parse errors = %d", code)
	}
	env.st.Read(func(d *store.Data) {
		if d.Alerts[id] != nil || len(d.ParseErrors) != 0 {
			t.Fatal("not removed")
		}
		for _, ev := range d.Events {
			if ev.AlertID == id {
				t.Fatal("events of the deleted incident remain")
			}
		}
	})
}
