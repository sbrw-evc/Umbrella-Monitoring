package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type fakeWorld struct {
	mu        sync.Mutex
	srv       *httptest.Server
	events    []map[string]any
	pdDown    bool
	zabbix    []string
	mediaType map[string]any
	cpu       float64
}

func newWorld(t *testing.T) *fakeWorld {
	w := &fakeWorld{cpu: 95}
	mux := http.NewServeMux()
	reply := func(rw http.ResponseWriter, v any) {
		rw.Header().Set("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(v)
	}
	mux.HandleFunc("POST /v2/enqueue", func(rw http.ResponseWriter, r *http.Request) {
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.pdDown {
			rw.WriteHeader(503)
			return
		}
		var ev map[string]any
		json.NewDecoder(r.Body).Decode(&ev)
		w.events = append(w.events, ev)
		rw.WriteHeader(202)
		reply(rw, map[string]string{"status": "success", "dedup_key": ev["dedup_key"].(string)})
	})
	mux.HandleFunc("GET /abilities", func(rw http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token token=pd-api-token" {
			rw.WriteHeader(401)
			return
		}
		reply(rw, map[string]any{"abilities": []string{"teams", "urgencies"}})
	})
	mux.HandleFunc("GET /oncalls", func(rw http.ResponseWriter, r *http.Request) {
		reply(rw, map[string]any{"more": false, "oncalls": []any{map[string]any{"escalation_level": 1,
			"user":              map[string]any{"id": "PU1", "summary": "Анна Дежурная", "name": "Анна Дежурная", "email": "anna@example.com"},
			"escalation_policy": map[string]any{"id": "PE1", "summary": "Ops"}}}})
	})
	mux.HandleFunc("POST /api_jsonrpc.php", func(rw http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &req)
		w.mu.Lock()
		w.zabbix = append(w.zabbix, req.Method)
		w.mu.Unlock()
		if req.Method != "apiinfo.version" && r.Header.Get("Authorization") != "Bearer zbx-token" {
			reply(rw, map[string]any{"error": map[string]string{"message": "Not authorized"}})
			return
		}
		var res any
		switch req.Method {
		case "apiinfo.version":
			res = "7.0.5"
		case "host.get":
			if req.Params["countOutput"] == true {
				res = "1"
			} else {
				res = []any{map[string]any{"hostid": "10084", "host": "db-01", "name": "db-01", "status": "0",
					"interfaces": []any{map[string]string{"ip": "10.0.0.5", "dns": ""}}, "hostgroups": []any{map[string]string{"name": "Linux servers"}}}}
			}
		case "mediatype.get", "action.get":
			res = []any{}
		case "mediatype.create":
			w.mu.Lock()
			w.mediaType = req.Params
			w.mu.Unlock()
			res = map[string]any{"mediatypeids": []string{"21"}}
		case "user.get":
			res = []any{map[string]any{"userid": "1", "medias": []any{}}}
		case "user.update":
			res = map[string]any{"userids": []string{"1"}}
		case "action.create":
			res = map[string]any{"actionids": []string{"7"}}
		case "problem.get":
			res = []any{map[string]any{"eventid": "901", "objectid": "13500", "name": "High CPU utilization (over 90% for 5m)",
				"severity": "4", "opdata": "Current utilization: 97 %", "tags": []any{map[string]string{"tag": "scope", "value": "performance"}}}}
		case "trigger.get":
			res = []any{map[string]any{"triggerid": "13500", "hosts": []any{map[string]string{"host": "db-01", "name": "db-01"}}}}
		}
		reply(rw, map[string]any{"jsonrpc": "2.0", "result": res, "id": 1})
	})
	mux.HandleFunc("GET /api/v1/targets", func(rw http.ResponseWriter, r *http.Request) {
		reply(rw, map[string]any{"status": "success", "data": map[string]any{"activeTargets": []any{
			map[string]any{"labels": map[string]string{"instance": "10.0.0.5:9100", "job": "node", "host": "db-01"}, "health": "up"},
		}}})
	})
	mux.HandleFunc("GET /api/v1/query", func(rw http.ResponseWriter, r *http.Request) {
		w.mu.Lock()
		v := w.cpu
		w.mu.Unlock()
		reply(rw, map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": []any{
			map[string]any{"metric": map[string]string{"host": "db-01", "instance": "10.0.0.5:9100"}, "value": []any{1.0, strings.TrimRight(strings.TrimRight(jsonNum(v), "0"), ".")}},
		}}})
	})
	mux.HandleFunc("GET /api/status/", func(rw http.ResponseWriter, r *http.Request) {
		reply(rw, map[string]any{"netbox-version": "4.3.2"})
	})
	mux.HandleFunc("GET /api/dcim/devices/", func(rw http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token nb-token" {
			rw.WriteHeader(403)
			return
		}
		reply(rw, map[string]any{"count": 1, "results": []any{map[string]any{"id": 12, "name": "DB-01",
			"role": map[string]any{"id": 1, "name": "Database server"}, "site": map[string]any{"id": 3, "name": "DC-1", "slug": "dc-1"},
			"tenant": map[string]any{"id": 5, "name": "Payments", "slug": "payments"}, "primary_ip": map[string]string{"address": "10.0.0.5/24"},
			"status": map[string]string{"value": "active"}}}})
	})
	mux.HandleFunc("GET /api/virtualization/virtual-machines/", func(rw http.ResponseWriter, r *http.Request) {
		reply(rw, map[string]any{"count": 0, "results": []any{}})
	})
	mux.HandleFunc("GET /api/tenancy/contacts/", func(rw http.ResponseWriter, r *http.Request) {
		reply(rw, map[string]any{"count": 1, "results": []any{map[string]any{"id": 9, "name": "Иван Петров", "email": "ivan@example.com"}}})
	})
	mux.HandleFunc("GET /api/tenancy/contact-assignments/", func(rw http.ResponseWriter, r *http.Request) {
		reply(rw, map[string]any{"count": 1, "results": []any{map[string]any{"object_type": "dcim.device", "object_id": 12,
			"contact": map[string]any{"id": 9, "name": "Иван Петров"}, "role": map[string]any{"id": 1, "name": "Owner"}}}})
	})
	w.srv = httptest.NewServer(mux)
	t.Cleanup(w.srv.Close)
	return w
}

func jsonNum(v float64) string {
	b, _ := json.Marshal(v)
	s := string(b)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

func (w *fakeWorld) sent() []map[string]any {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]map[string]any(nil), w.events...)
}

func waitFor(t *testing.T, what string, f func() bool) {
	t.Helper()
	for range 300 {
		if f() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "v1=" + hex.EncodeToString(m.Sum(nil))
}

func TestPagerDutyFlow(t *testing.T) {
	env := newEnv(t, true)
	ts, world := env.ts, newWorld(t)

	body := `{"enabled":true,"events_url":"` + world.srv.URL + `/v2/enqueue","api_url":"` + world.srv.URL + `","routing_key":"R0UTING-KEY","api_token":"pd-api-token","webhook_secret":"whsec"}`
	var view struct {
		Settings model.PDSettings
	}
	if code := do(t, "PUT", ts.URL+"/api/pagerduty", body, &view); code != 200 {
		t.Fatalf("settings code = %d", code)
	}
	if !strings.HasPrefix(view.Settings.RoutingKeyRef, "openbao://umbrella/pagerduty#") {
		t.Fatalf("routing key not a reference: %+v", view.Settings)
	}
	if got := env.vault.Get("umbrella/pagerduty"); got["routing_key"] != "R0UTING-KEY" || got["webhook_secret"] != "whsec" {
		t.Fatalf("vault = %v", got)
	}
	var raw struct{ Body string }
	_ = raw
	var check struct{ OK bool }
	do(t, "POST", ts.URL+"/api/pagerduty/check", "", &check)
	if !check.OK {
		t.Fatal("REST check failed")
	}
	do(t, "POST", ts.URL+"/api/pagerduty/oncall/sync", "", nil)
	env.st.Read(func(d *store.Data) {
		if len(d.OnCall.Entries) != 1 || d.OnCall.Entries[0].Email != "anna@example.com" {
			t.Fatalf("oncall = %+v", d.OnCall)
		}
	})

	do(t, "POST", ts.URL+"/api/ingest/CON-4", `{"id":"p1","service":"Платёжный шлюз","signal":"red.errors","severity":"critical","state":"firing","title":"5xx"}`, nil)
	waitFor(t, "trigger", func() bool { return len(world.sent()) == 1 })
	ev := world.sent()[0]
	if ev["routing_key"] != "R0UTING-KEY" || ev["event_action"] != "trigger" || !strings.HasPrefix(ev["dedup_key"].(string), "umb-INC-") {
		t.Fatalf("event = %v", ev)
	}
	key := ev["dedup_key"].(string)
	id := strings.TrimPrefix(key, "umb-")
	waitFor(t, "accepted", func() bool {
		var a model.Alert
		env.st.Read(func(d *store.Data) { a = *d.Alerts[id] })
		return a.PDState == model.PDAccepted
	})

	hook := []byte(`{"event":{"event_type":"incident.acknowledged","agent":{"summary":"Анна"},"data":{"id":"Q1","type":"incident","html_url":"https://pd/incidents/Q1","incident_key":"` + key + `"}}}`)
	req, _ := http.NewRequest("POST", ts.URL+"/api/pagerduty/webhook", strings.NewReader(string(hook)))
	req.Header.Set("X-PagerDuty-Signature", sign("wrong", hook))
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != 401 {
		t.Fatalf("bad signature code = %d", res.StatusCode)
	}
	req, _ = http.NewRequest("POST", ts.URL+"/api/pagerduty/webhook", strings.NewReader(string(hook)))
	req.Header.Set("X-PagerDuty-Signature", sign("whsec", hook))
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != 200 {
		t.Fatalf("webhook code = %d", res.StatusCode)
	}
	env.st.Read(func(d *store.Data) {
		a := d.Alerts[id]
		if a.Status != model.AlertAcknowledged || a.PDIncidentURL != "https://pd/incidents/Q1" || a.AckedBy != "Анна" {
			t.Fatalf("alert after webhook = %s %s %s", a.Status, a.PDIncidentURL, a.AckedBy)
		}
	})

	world.mu.Lock()
	world.pdDown = true
	world.mu.Unlock()
	do(t, "POST", ts.URL+"/api/ingest/CON-4", `{"id":"p2","service":"Личный кабинет","signal":"red.errors","severity":"error","state":"firing","title":"lk"}`, nil)
	var failedID string
	waitFor(t, "failed delivery", func() bool {
		env.st.Read(func(d *store.Data) {
			for _, a := range d.Alerts {
				if a.Title == "lk" && a.PDState == model.PDFailed && a.PDRetry == "trigger" {
					failedID = a.ID
				}
			}
		})
		return failedID != ""
	})
}

func TestPagerDutyDisabledFallsBack(t *testing.T) {
	env := newEnv(t, true)
	do(t, "POST", env.ts.URL+"/api/ingest/CON-4", `{"id":"d1","service":"Платёжный шлюз","signal":"red.errors","severity":"critical","state":"firing","title":"x"}`, nil)
	waitFor(t, "failed", func() bool {
		ok := false
		env.st.Read(func(d *store.Data) {
			for _, a := range d.Alerts {
				ok = a.PDState == model.PDFailed && strings.Contains(a.PDError, "не подключён")
			}
		})
		return ok
	})
	hook := []byte(`{"event":{"event_type":"incident.resolved","data":{"id":"Q"}}}`)
	req, _ := http.NewRequest("POST", env.ts.URL+"/api/pagerduty/webhook", strings.NewReader(string(hook)))
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != 401 {
		t.Fatalf("unsigned webhook accepted: %d", res.StatusCode)
	}
}

type view struct {
	ID          string
	ConnectorID string `json:"connector_id"`
	Slug        string
	IngestURL   string `json:"ingest_url"`
	SecretRef   string `json:"secret_ref"`
}

func TestIntegrationsMergeHostsAndRules(t *testing.T) {
	env := newEnv(t, false)
	ts, world := env.ts, newWorld(t)

	var zbx view
	if code := do(t, "POST", ts.URL+"/api/integrations", `{"type":"zabbix","name":"Zabbix","team":"ops","url":"`+world.srv.URL+`","auth_type":"token","secret":"zbx-token","params":{"umbrella_url":"http://umbrella:8080"}}`, &zbx); code != 201 {
		t.Fatalf("zabbix create code = %d", code)
	}
	if zbx.Slug != "zabbix" || zbx.IngestURL != "http://umbrella:8080/api/ingest/zabbix" || !strings.HasPrefix(zbx.SecretRef, "openbao://umbrella/integrations/") {
		t.Fatalf("zabbix view = %+v", zbx)
	}
	if got := env.vault.Get("umbrella/integrations/" + zbx.ID); got["secret"] != "zbx-token" || got["webhook_token"] == "" {
		t.Fatalf("vault = %v", got)
	}
	var res struct {
		OK      bool
		Message string
	}
	do(t, "POST", ts.URL+"/api/integrations/"+zbx.ID+"/check", "", &res)
	if !res.OK || !strings.Contains(res.Message, "7.0.5") {
		t.Fatalf("check = %+v", res)
	}
	do(t, "POST", ts.URL+"/api/integrations/"+zbx.ID+"/setup", "", &res)
	if !res.OK || !strings.Contains(res.Message, "открытых проблем передано 1") {
		t.Fatalf("setup = %+v", res)
	}
	var open struct {
		Items []struct {
			Signal, Severity, Status, CI, Title string
		}
	}
	do(t, "GET", ts.URL+"/api/incidents?view=all", "", &open)
	found := false
	for _, a := range open.Items {
		if a.Signal == "zabbix:13500" && a.Severity == "error" && a.Status == "open" {
			found = true
		}
	}
	if !found {
		t.Fatalf("open Zabbix problem was not imported on setup: %+v", open.Items)
	}
	world.mu.Lock()
	mt := world.mediaType
	world.mu.Unlock()
	params, _ := json.Marshal(mt["parameters"])
	tok := env.vault.Get("umbrella/integrations/" + zbx.ID)["webhook_token"].(string)
	if !strings.Contains(string(params), "http://umbrella:8080/api/ingest/zabbix") || !strings.Contains(string(params), tok) {
		t.Fatalf("media type params = %s", params)
	}

	var prom, nb view
	if code := do(t, "POST", ts.URL+"/api/integrations", `{"type":"prometheus","name":"Prometheus","url":"`+world.srv.URL+`","auth_type":"none"}`, &prom); code != 201 {
		t.Fatalf("prometheus create code = %d", code)
	}
	if prom.ConnectorID != "" {
		t.Fatal("metrics source got a connector")
	}
	if code := do(t, "POST", ts.URL+"/api/integrations", `{"type":"netbox","name":"NetBox","url":"`+world.srv.URL+`","auth_type":"token","secret":"nb-token"}`, &nb); code != 201 {
		t.Fatalf("netbox create code = %d", code)
	}
	for _, id := range []string{zbx.ID, prom.ID, nb.ID} {
		do(t, "POST", ts.URL+"/api/integrations/"+id+"/sync", "", &res)
		if !res.OK {
			t.Fatalf("sync %s = %+v", id, res)
		}
	}
	var cis struct {
		Items []struct {
			ID, Name, Type, Team, Origin string
			Owners                       []model.Owner
			Identities                   []model.Identity
		}
	}
	do(t, "GET", ts.URL+"/api/cis", "", &cis)
	if len(cis.Items) != 1 {
		t.Fatalf("hosts not merged: %+v", cis.Items)
	}
	ci := cis.Items[0]
	if ci.Name != "DB-01" || ci.Type != model.CIDatabase || ci.Team != "payments" || len(ci.Owners) != 1 || ci.Owners[0].Email != "ivan@example.com" {
		t.Fatalf("ci = %+v", ci)
	}
	kinds := map[string]bool{}
	for _, i := range ci.Identities {
		kinds[i.Kind] = true
	}
	for _, k := range []string{"zabbix", "prometheus", "netbox", "ip", "instance"} {
		if !kinds[k] {
			t.Fatalf("identity %s missing: %+v", k, ci.Identities)
		}
	}

	body := `{"event_id":"1","status":"firing","host":"db-01","trigger_id":"5","trigger_name":"Disk","severity":"High"}`
	req, _ := http.NewRequest("POST", ts.URL+"/api/ingest/zabbix", strings.NewReader(body))
	req.Header.Set("X-Umbrella-Token", tok)
	if r, _ := http.DefaultClient.Do(req); r.StatusCode != 202 {
		t.Fatalf("zabbix ingest code = %d", r.StatusCode)
	}

	var rule model.Rule
	rb := `{"name":"CPU","method":"use","signal":"use.cpu.utilization","source_id":"` + prom.ID + `","query":"cpu","ci_label":"host","op":">","threshold":50,"for":"0s","interval":"30s","severity":"warning","enabled":true}`
	var preview struct{ Matched int }
	do(t, "POST", ts.URL+"/api/rules/preview", rb, &preview)
	if preview.Matched != 1 {
		t.Fatalf("preview matched = %d", preview.Matched)
	}
	if code := do(t, "POST", ts.URL+"/api/rules", rb, &rule); code != 201 {
		t.Fatalf("rule create code = %d", code)
	}
	do(t, "POST", ts.URL+"/api/rules/"+rule.ID+"/evaluate", "", &rule)
	if rule.Firing != 1 {
		t.Fatalf("rule = %+v", rule)
	}
	var list struct {
		Items []model.Alert
	}
	do(t, "GET", ts.URL+"/api/incidents?view=open", "", &list)
	if len(list.Items) != 3 {
		t.Fatalf("incidents = %+v", list.Items)
	}
	for _, a := range list.Items {
		if a.CIID != ci.ID {
			t.Fatalf("incident %s bound to %q, want %s", a.Title, a.CIID, ci.ID)
		}
	}
	world.mu.Lock()
	world.cpu = 10
	world.mu.Unlock()
	do(t, "POST", ts.URL+"/api/rules/"+rule.ID+"/evaluate", "", &rule)
	do(t, "GET", ts.URL+"/api/incidents?view=open", "", &list)
	for _, a := range list.Items {
		if a.Signal == "use.cpu.utilization" {
			t.Fatalf("rule incident not resolved: %+v", list.Items)
		}
	}
	if len(list.Items) != 2 {
		t.Fatalf("open incidents after rule recovery: %+v", list.Items)
	}
	if code := do(t, "DELETE", ts.URL+"/api/rules/"+rule.ID, "", nil); code != 204 {
		t.Fatalf("rule delete code = %d", code)
	}

	var note struct{ Note string }
	if code := do(t, "DELETE", ts.URL+"/api/integrations/"+zbx.ID, "", &note); code != 200 {
		t.Fatalf("delete code = %d", code)
	}
	if env.vault.Get("umbrella/integrations/"+zbx.ID) != nil {
		t.Fatal("integration secrets left in OpenBao")
	}
}

func TestCIEditAndRelations(t *testing.T) {
	ts := newServer(t)
	var a, b model.CI
	do(t, "POST", ts.URL+"/api/cis", `{"name":"svc-x","type":"it_service","team":"t"}`, &a)
	do(t, "POST", ts.URL+"/api/cis", `{"name":"host-x","type":"host","team":"t"}`, &b)
	if code := do(t, "PUT", ts.URL+"/api/cis/"+b.ID, `{"description":"edited","owners":[{"name":"Ольга","email":"olga@example.com"}],"identities":[{"kind":"ip","value":"10.9.9.9"}]}`, &b); code != 200 {
		t.Fatalf("update code = %d", code)
	}
	if b.Description != "edited" || len(b.Owners) != 1 || len(b.Identities) != 1 {
		t.Fatalf("ci = %+v", b)
	}
	if code := do(t, "PUT", ts.URL+"/api/cis/"+b.ID, `{"name":"svc-x"}`, nil); code != 400 {
		t.Fatalf("duplicate name code = %d", code)
	}
	if code := do(t, "POST", ts.URL+"/api/relations", `{"from":"`+a.ID+`","to":"`+b.ID+`","type":"runs_on"}`, nil); code != 201 {
		t.Fatalf("relation code = %d", code)
	}
	if code := do(t, "POST", ts.URL+"/api/relations", `{"from":"`+a.ID+`","to":"`+b.ID+`","type":"runs_on"}`, nil); code != 400 {
		t.Fatalf("duplicate relation code = %d", code)
	}
	if code := do(t, "DELETE", ts.URL+"/api/relations?from="+a.ID+"&to="+b.ID, "", nil); code != 204 {
		t.Fatalf("relation delete code = %d", code)
	}
	if code := do(t, "DELETE", ts.URL+"/api/cis/"+b.ID, "", nil); code != 204 {
		t.Fatalf("delete code = %d", code)
	}
	if code := do(t, "GET", ts.URL+"/api/cis/"+b.ID, "", nil); code != 404 {
		t.Fatalf("deleted ci code = %d", code)
	}
}
