package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type fakeGrafana struct {
	mu       sync.Mutex
	contacts []map[string]any
	policy   map[string]any
	auth     []string
}

func (g *fakeGrafana) server(t *testing.T) *httptest.Server {
	g.policy = map[string]any{"receiver": "default", "routes": []any{map[string]any{"receiver": "team-a", "object_matchers": []any{[]string{"team", "=", "a"}}}}}
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]string{"version": "12.1.1", "database": "ok"})
	})
	mux.HandleFunc("GET /api/v1/provisioning/contact-points", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.auth = append(g.auth, r.Header.Get("Authorization"))
		out := []map[string]any{}
		for _, c := range g.contacts {
			if n := r.URL.Query().Get("name"); n == "" || c["name"] == n {
				out = append(out, c)
			}
		}
		reply(w, out)
	})
	mux.HandleFunc("POST /api/v1/provisioning/contact-points", func(w http.ResponseWriter, r *http.Request) {
		var c map[string]any
		json.NewDecoder(r.Body).Decode(&c)
		g.mu.Lock()
		c["uid"] = "cp-1"
		g.contacts = append(g.contacts, c)
		g.mu.Unlock()
		reply(w, c)
	})
	mux.HandleFunc("DELETE /api/v1/provisioning/contact-points/{uid}", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.contacts = nil
		g.mu.Unlock()
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /api/v1/provisioning/policies", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		reply(w, g.policy)
	})
	mux.HandleFunc("PUT /api/v1/provisioning/policies", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		json.NewDecoder(r.Body).Decode(&g.policy)
		w.WriteHeader(202)
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func TestGrafanaAlertingSetupAndTeardown(t *testing.T) {
	env := newEnv(t, false)
	g := &fakeGrafana{}
	gs := g.server(t)
	var it struct {
		ID        string
		IngestURL string `json:"ingest_url"`
	}
	if code := do(t, "POST", env.ts.URL+"/api/integrations", `{"type":"grafana","name":"Grafana","url":"`+gs.URL+`","auth_type":"token","secret":"glsa_x","params":{"umbrella_url":"http://umbrella:8080"}}`, &it); code != 201 {
		t.Fatalf("create code = %d", code)
	}
	var res struct {
		OK      bool
		Message string
	}
	do(t, "POST", env.ts.URL+"/api/integrations/"+it.ID+"/check", "", &res)
	if !res.OK || !strings.Contains(res.Message, "12.1.1") {
		t.Fatalf("check = %+v", res)
	}
	do(t, "POST", env.ts.URL+"/api/integrations/"+it.ID+"/setup", "", &res)
	if !res.OK {
		t.Fatalf("setup = %+v", res)
	}
	g.mu.Lock()
	cp := g.contacts[0]
	settings := cp["settings"].(map[string]any)
	routes := g.policy["routes"].([]any)
	auth := g.auth[0]
	g.mu.Unlock()
	tok := env.vault.Get("umbrella/integrations/" + it.ID)["webhook_token"]
	if settings["url"] != "http://umbrella:8080/api/ingest/grafana" || settings["authorization_credentials"] != tok || auth != "Bearer glsa_x" {
		t.Fatalf("contact point = %v auth %q", cp, auth)
	}
	if len(routes) != 2 || routes[0].(map[string]any)["receiver"] != "Umbrella" || routes[0].(map[string]any)["continue"] != true {
		t.Fatalf("routes = %v", routes)
	}
	do(t, "POST", env.ts.URL+"/api/integrations/"+it.ID+"/setup", "", &res)
	g.mu.Lock()
	n := len(g.policy["routes"].([]any))
	g.mu.Unlock()
	if n != 2 {
		t.Fatalf("setup is not idempotent: %d routes", n)
	}
	if code := do(t, "DELETE", env.ts.URL+"/api/integrations/"+it.ID+"?teardown=1", "", nil); code != 200 {
		t.Fatalf("delete code = %d", code)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.contacts) != 0 || len(g.policy["routes"].([]any)) != 1 {
		t.Fatalf("teardown left %v / %v", g.contacts, g.policy["routes"])
	}
}
