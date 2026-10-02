package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/connector"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pagerduty"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// newUser creates a user with a password that need not be changed.
func newUser(t *testing.T, ts *httptest.Server, username string, roles, services []string) string {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"username": username, "name": username, "password": "Secret-pass-77",
		"roles": roles, "business_services": services, "must_change_password": false})
	var u struct{ ID string }
	if code := do(t, "POST", ts.URL+"/api/users", string(b), &u); code != 201 {
		t.Fatalf("create user %s: code %d", username, code)
	}
	return u.ID
}

func TestAuthRequired(t *testing.T) {
	ts := newServer(t)
	if code := doAs(t, "", "GET", ts.URL+"/api/incidents", "", nil); code != 401 {
		t.Fatalf("anonymous code = %d", code)
	}
	if code := doAs(t, "bogus", "GET", ts.URL+"/api/incidents", "", nil); code != 401 {
		t.Fatalf("bad token code = %d", code)
	}
	if code := doAs(t, "", "GET", ts.URL+"/healthz", "", nil); code != 200 {
		t.Fatalf("healthz code = %d", code)
	}
	var bad struct{ Error string }
	if code := doAs(t, "", "POST", ts.URL+"/api/auth/token", `{"username":"admin","password":"wrong"}`, &bad); code != 401 || bad.Error == "" {
		t.Fatalf("wrong password code = %d %+v", code, bad)
	}
}

func TestViewerCannotAct(t *testing.T) {
	ts := newServer(t)
	newUser(t, ts, "viewer1", []string{"viewer"}, []string{"CI-1"})
	v := login(t, ts, "viewer1", "Secret-pass-77")
	do(t, "POST", ts.URL+"/api/ingest/CON-4", `{"id":"v1","service":"Платёжный шлюз","signal":"red.errors","severity":"critical","state":"firing","title":"x"}`, nil)
	var list struct{ Items []struct{ ID string } }
	if code := doAs(t, v, "GET", ts.URL+"/api/incidents?view=open", "", &list); code != 200 || len(list.Items) != 1 {
		t.Fatalf("viewer list code = %d items %d", code, len(list.Items))
	}
	if code := doAs(t, v, "POST", ts.URL+"/api/incidents/"+list.Items[0].ID+"/ack", "", nil); code != 403 {
		t.Fatalf("viewer ack code = %d", code)
	}
	for _, u := range []string{"/api/users", "/api/roles", "/api/channels", "/api/audit"} {
		if code := doAs(t, v, "GET", ts.URL+u, "", nil); code != 403 {
			t.Fatalf("viewer %s code = %d", u, code)
		}
	}
}

// An owner bound to "Интернет-банк" sees its dependency tree only.
func TestOwnerScope(t *testing.T) {
	ts := newServer(t)
	newUser(t, ts, "owner1", []string{"owner"}, []string{"CI-2"})
	o := login(t, ts, "owner1", "Secret-pass-77")
	do(t, "POST", ts.URL+"/api/ingest/CON-4", `{"id":"p1","service":"Платёжный шлюз","signal":"red.errors","severity":"critical","state":"firing","title":"pay"}`, nil)
	do(t, "POST", ts.URL+"/api/ingest/CON-4", `{"id":"l1","service":"Личный кабинет","signal":"red.errors","severity":"error","state":"firing","title":"lk"}`, nil)

	var all, mine struct {
		Items []struct{ ID, Title string }
	}
	do(t, "GET", ts.URL+"/api/incidents?view=open", "", &all)
	doAs(t, o, "GET", ts.URL+"/api/incidents?view=open", "", &mine)
	if len(all.Items) != 2 || len(mine.Items) != 1 || mine.Items[0].Title != "lk" {
		t.Fatalf("admin %d, owner %+v", len(all.Items), mine.Items)
	}
	var hidden string
	for _, it := range all.Items {
		if it.Title == "pay" {
			hidden = it.ID
		}
	}
	if code := doAs(t, o, "GET", ts.URL+"/api/incidents/"+hidden, "", nil); code != 404 {
		t.Fatalf("hidden incident code = %d", code)
	}
	if code := doAs(t, o, "POST", ts.URL+"/api/incidents/"+mine.Items[0].ID+"/ack", "", nil); code != 200 {
		t.Fatalf("owner ack code = %d", code)
	}
	var cis struct{ Items []struct{ Name string } }
	doAs(t, o, "GET", ts.URL+"/api/cis", "", &cis)
	for _, c := range cis.Items {
		if c.Name == "pay-db-01" || c.Name == "Онлайн-платежи" {
			t.Fatalf("owner sees %s", c.Name)
		}
	}
	var me struct {
		AllServices      bool                  `json:"all_services"`
		BusinessServices []struct{ ID string } `json:"business_services"`
	}
	doAs(t, o, "GET", ts.URL+"/api/auth/me", "", &me)
	if me.AllServices || len(me.BusinessServices) != 1 || me.BusinessServices[0].ID != "CI-2" {
		t.Fatalf("me = %+v", me)
	}
}

func TestCookieSessionNeedsCSRF(t *testing.T) {
	ts := newServer(t)
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	res, err := c.Post(ts.URL+"/api/auth/login", "application/json", strings.NewReader(`{"username":"admin","password":"`+adminPassword+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	var me struct{ CSRF string }
	_ = json.NewDecoder(res.Body).Decode(&me)
	res.Body.Close()
	if res.StatusCode != 200 || me.CSRF == "" {
		t.Fatalf("login code = %d csrf %q", res.StatusCode, me.CSRF)
	}
	send := func(method, url, csrf string) int {
		req, _ := http.NewRequest(method, ts.URL+url, strings.NewReader(`{}`))
		if csrf != "" {
			req.Header.Set("X-Umbrella-CSRF", csrf)
		}
		r, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		return r.StatusCode
	}
	if code := send("GET", "/api/incidents", ""); code != 200 {
		t.Fatalf("cookie GET code = %d", code)
	}
	if code := send("POST", "/api/connectors/CON-1/stop", ""); code != 403 {
		t.Fatalf("POST without CSRF code = %d", code)
	}
	if code := send("POST", "/api/connectors/CON-1/stop", me.CSRF); code != 200 {
		t.Fatalf("POST with CSRF code = %d", code)
	}
	if code := send("POST", "/api/auth/logout", me.CSRF); code != 204 {
		t.Fatalf("logout code = %d", code)
	}
	if code := send("GET", "/api/incidents", ""); code != 401 {
		t.Fatalf("after logout code = %d", code)
	}
}

func TestLockoutAndPasswordChange(t *testing.T) {
	ts := newServer(t)
	b, _ := json.Marshal(map[string]any{"username": "new1", "password": "Secret-pass-77", "roles": []string{"oncall"}})
	do(t, "POST", ts.URL+"/api/users", string(b), nil)
	n := login(t, ts, "new1", "Secret-pass-77")
	var e struct{ Code string }
	if code := doAs(t, n, "GET", ts.URL+"/api/incidents", "", &e); code != 403 || e.Code != "password_change_required" {
		t.Fatalf("must change: %d %+v", code, e)
	}
	if code := doAs(t, n, "POST", ts.URL+"/api/auth/password", `{"current":"Secret-pass-77","new":"short"}`, nil); code != 400 {
		t.Fatalf("weak password code = %d", code)
	}
	if code := doAs(t, n, "POST", ts.URL+"/api/auth/password", `{"current":"Secret-pass-77","new":"Better-pass-88"}`, nil); code != 204 {
		t.Fatalf("change code = %d", code)
	}
	if code := doAs(t, n, "GET", ts.URL+"/api/incidents", "", nil); code != 200 {
		t.Fatalf("after change code = %d", code)
	}
	for range 5 {
		doAs(t, "", "POST", ts.URL+"/api/auth/token", `{"username":"new1","password":"nope"}`, nil)
	}
	if code := doAs(t, "", "POST", ts.URL+"/api/auth/token", `{"username":"new1","password":"Better-pass-88"}`, nil); code != 423 {
		t.Fatalf("locked code = %d", code)
	}
}

func TestLastAdminProtected(t *testing.T) {
	ts := newServer(t)
	if code := do(t, "DELETE", ts.URL+"/api/users/USR-1", "", nil); code < 400 {
		t.Fatalf("delete last admin code = %d", code)
	}
	if code := do(t, "PUT", ts.URL+"/api/users/USR-1", `{"roles":["viewer"]}`, nil); code < 400 {
		t.Fatalf("demote last admin code = %d", code)
	}
	if code := do(t, "DELETE", ts.URL+"/api/roles/admin", "", nil); code < 400 {
		t.Fatalf("delete built-in role code = %d", code)
	}
}

func TestAPITokenAndCustomRole(t *testing.T) {
	ts := newServer(t)
	if code := do(t, "POST", ts.URL+"/api/roles", `{"id":"cmdb-editor","name":"Редактор КЕ","permissions":["cmdb.view","cmdb.edit"],"all_services":true}`, nil); code != 201 {
		t.Fatalf("create role code = %d", code)
	}
	if code := do(t, "POST", ts.URL+"/api/roles", `{"id":"bad","name":"x","permissions":["nope"]}`, nil); code != 400 {
		t.Fatalf("bad permission code = %d", code)
	}
	id := newUser(t, ts, "bot1", []string{"cmdb-editor"}, nil)
	var tok struct{ Token string }
	if code := do(t, "POST", ts.URL+"/api/users/"+id+"/tokens", `{"name":"ci","days":30}`, &tok); code != 201 || !strings.HasPrefix(tok.Token, "umb_") {
		t.Fatalf("token code = %d", code)
	}
	if code := doAs(t, tok.Token, "GET", ts.URL+"/api/cis", "", nil); code != 200 {
		t.Fatalf("token cis code = %d", code)
	}
	if code := doAs(t, tok.Token, "GET", ts.URL+"/api/incidents", "", nil); code != 403 {
		t.Fatalf("token incidents code = %d", code)
	}
	var list struct{ Items []struct{ ID string } }
	do(t, "GET", ts.URL+"/api/users/"+id+"/tokens", "", &list)
	if len(list.Items) != 1 {
		t.Fatalf("tokens = %+v", list)
	}
	do(t, "DELETE", ts.URL+"/api/users/"+id+"/tokens/"+list.Items[0].ID, "", nil)
	if code := doAs(t, tok.Token, "GET", ts.URL+"/api/cis", "", nil); code != 401 {
		t.Fatalf("revoked token code = %d", code)
	}
}

func TestSlugIngest(t *testing.T) {
	ts := newServer(t)
	if code := do(t, "PUT", ts.URL+"/api/connectors/CON-4", `{"slug":"lab-hook"}`, nil); code != 200 {
		t.Fatalf("set slug code = %d", code)
	}
	body := `{"id":"s1","service":"Платёжный шлюз","signal":"red.errors","severity":"critical","state":"firing","title":"slug"}`
	if code := do(t, "POST", ts.URL+"/api/ingest/lab-hook", body, nil); code != 202 {
		t.Fatalf("slug ingest code = %d", code)
	}
	if code := do(t, "PUT", ts.URL+"/api/connectors/CON-1", `{"slug":"Bad Slug"}`, nil); code != 400 {
		t.Fatalf("bad slug code = %d", code)
	}
}

type hook struct {
	mu     sync.Mutex
	bodies []map[string]any
	auth   []string
	paths  []string
}

func (h *hook) server(t *testing.T) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v map[string]any
		_ = json.NewDecoder(r.Body).Decode(&v)
		h.mu.Lock()
		h.bodies = append(h.bodies, v)
		h.auth = append(h.auth, r.Header.Get("Authorization"))
		h.paths = append(h.paths, r.URL.RequestURI())
		h.mu.Unlock()
		w.WriteHeader(200)
	}))
	t.Cleanup(s.Close)
	return s
}

func (h *hook) wait(t *testing.T, n int) {
	t.Helper()
	for range 200 {
		h.mu.Lock()
		got := len(h.bodies)
		h.mu.Unlock()
		if got >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("webhook got %d messages, want %d", len(h.bodies), n)
}

func TestTeamsAndZoomChannels(t *testing.T) {
	ts := newServer(t)
	var teams, zoom hook
	tsrv, zsrv := teams.server(t), zoom.server(t)
	var ch struct{ ID string }
	if code := do(t, "POST", ts.URL+"/api/channels", `{"name":"Teams","type":"teams","url":"`+tsrv.URL+`/hook","min_severity":"error","events":["open","resolve"]}`, &ch); code != 201 {
		t.Fatalf("teams channel code = %d", code)
	}
	var del struct {
		OK     bool
		Status int
	}
	if code := do(t, "POST", ts.URL+"/api/channels/"+ch.ID+"/test", "", &del); code != 200 || !del.OK {
		t.Fatalf("test code = %d %+v", code, del)
	}
	if code := do(t, "POST", ts.URL+"/api/channels", `{"name":"Zoom","type":"zoom","url":"`+zsrv.URL+`/chat","token":"verif-123","services":["CI-1"],"events":["open"]}`, nil); code != 201 {
		t.Fatalf("zoom channel code = %d", code)
	}
	if code := do(t, "POST", ts.URL+"/api/channels", `{"name":"x","type":"slack","url":"https://x"}`, nil); code != 400 {
		t.Fatalf("bad type code = %d", code)
	}
	var listed struct {
		Items []map[string]any
	}
	do(t, "GET", ts.URL+"/api/channels", "", &listed)
	for _, it := range listed.Items {
		if _, ok := it["url"]; ok {
			t.Fatalf("channel list leaks the url: %v", it)
		}
	}

	// Teams got the test; payments incident: both channels; web incident:
	// only Teams; a warning is below the Teams threshold.
	do(t, "POST", ts.URL+"/api/ingest/CON-4", `{"id":"n1","service":"Платёжный шлюз","signal":"red.errors","severity":"critical","state":"firing","title":"pay down"}`, nil)
	do(t, "POST", ts.URL+"/api/ingest/CON-4", `{"id":"n2","service":"Личный кабинет","signal":"red.errors","severity":"error","state":"firing","title":"lk down"}`, nil)
	do(t, "POST", ts.URL+"/api/ingest/CON-4", `{"id":"n3","service":"Личный кабинет","signal":"use.errors","severity":"warning","state":"firing","title":"too low"}`, nil)
	teams.wait(t, 3)
	zoom.wait(t, 1)
	time.Sleep(50 * time.Millisecond)
	if len(teams.bodies) != 3 || len(zoom.bodies) != 1 {
		t.Fatalf("teams %d zoom %d", len(teams.bodies), len(zoom.bodies))
	}
	tb := teams.bodies[1]
	att := tb["attachments"].([]any)[0].(map[string]any)
	if tb["type"] != "message" || att["contentType"] != "application/vnd.microsoft.card.adaptive" {
		t.Fatalf("teams body = %v", tb)
	}
	zb := zoom.bodies[0]["content"].(map[string]any)
	head := zb["head"].(map[string]any)
	if !strings.Contains(head["text"].(string), "pay down") || zoom.auth[0] != "verif-123" || !strings.Contains(zoom.paths[0], "format=full") {
		t.Fatalf("zoom = %v auth %q path %q", zb, zoom.auth[0], zoom.paths[0])
	}
	var dl struct{ Items []struct{ OK bool } }
	do(t, "GET", ts.URL+"/api/deliveries", "", &dl)
	if len(dl.Items) != 4 {
		t.Fatalf("deliveries = %d", len(dl.Items))
	}
}

func TestMetrics(t *testing.T) {
	ts := newServer(t)
	do(t, "POST", ts.URL+"/api/ingest/CON-4", `{"id":"m1","service":"Платёжный шлюз","signal":"red.errors","severity":"critical","state":"firing","title":"m"}`, nil)
	res, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	text := string(b)
	for _, want := range []string{"umbrella_up 1", `severity="critical",status="open",team="payments"} 1`, "umbrella_incidents_total", "umbrella_users_enabled"} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics miss %q:\n%s", want, text)
		}
	}
}

// A deployment without demo data: empty CMDB filled from events.
func TestCleanStartAutoCMDB(t *testing.T) {
	st := store.New()
	auth.Bootstrap(st, auth.BootstrapConfig{AdminPassword: adminPassword})
	eng := alert.New(st, nopPD{}, nil)
	eng.AutoCMDB = true
	rt := connector.New(st, eng, connector.EnvSecrets{}, nil)
	ts := httptest.NewServer(New(Config{}, st, eng, rt, pagerduty.New(pagerduty.Config{}), NewHub()).Handler())
	t.Cleanup(ts.Close)
	token = login(t, ts, "admin", adminPassword)

	var cis struct {
		Items []struct{ Name, Origin string }
	}
	do(t, "GET", ts.URL+"/api/cis", "", &cis)
	if len(cis.Items) != 0 {
		t.Fatalf("clean CMDB has %d CIs", len(cis.Items))
	}
	var c struct{ ID string }
	do(t, "POST", ts.URL+"/api/connectors", `{"name":"hook","slug":"hook","template":"webhook-json"}`, &c)
	g := starterGraph("webhook")
	g.Nodes[0].Config["auth"] = "none"
	b, _ := json.Marshal(map[string]any{"draft": g})
	do(t, "PUT", ts.URL+"/api/connectors/"+c.ID, string(b), nil)
	if code := do(t, "POST", ts.URL+"/api/connectors/"+c.ID+"/publish", "", nil); code != 200 {
		t.Fatalf("publish code = %d", code)
	}
	do(t, "POST", ts.URL+"/api/connectors/"+c.ID+"/start", "", nil)
	if code := doAs(t, "", "POST", ts.URL+"/api/ingest/hook", `{"id":"1","host":"srv-01","title":"disk","severity":"error","status":"firing"}`, nil); code != 202 {
		t.Fatalf("ingest code = %d", code)
	}
	do(t, "GET", ts.URL+"/api/cis", "", &cis)
	if len(cis.Items) != 1 || cis.Items[0].Name != "srv-01" || cis.Items[0].Origin != "auto" {
		t.Fatalf("cis = %+v", cis.Items)
	}
	var list struct {
		Items []struct {
			CIName string `json:"ci_name"`
		}
	}
	do(t, "GET", ts.URL+"/api/incidents?view=open", "", &list)
	if len(list.Items) != 1 || list.Items[0].CIName != "srv-01" {
		t.Fatalf("incidents = %+v", list.Items)
	}
}
