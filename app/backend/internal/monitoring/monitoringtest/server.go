// Package monitoringtest runs fake Zabbix and Prometheus servers for tests.
package monitoringtest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// Zabbix answers the JSON-RPC methods Umbrella uses. Version decides where the token is
// expected: the Authorization header from 6.4 on, the auth field before.
type Zabbix struct {
	URL      string
	Version  string
	Token    string
	User     string
	Password string

	mu        sync.Mutex
	hosts     []map[string]any
	sessions  map[string]bool
	Calls     []string
	LoggedOut int
}

func StartZabbix(t *testing.T, version string) *Zabbix {
	z := &Zabbix{Version: version, Token: "zbx-token", User: "Admin", Password: "zabbix", sessions: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(z.serve))
	t.Cleanup(srv.Close)
	z.URL = srv.URL
	return z
}

// AddHost adds a host; interfaces are {ip, dns, available}.
func (z *Zabbix) AddHost(id int, host, name string, disabled bool, groups []string, ifaces ...map[string]any) {
	z.mu.Lock()
	defer z.mu.Unlock()
	status := "0"
	if disabled {
		status = "1"
	}
	var gs []map[string]any
	for _, g := range groups {
		gs = append(gs, map[string]any{"name": g})
	}
	if ifaces == nil {
		ifaces = []map[string]any{}
	}
	z.hosts = append(z.hosts, map[string]any{"hostid": strconv.Itoa(id), "host": host, "name": name, "status": status, "interfaces": ifaces, "groups": gs})
}

func Iface(ip, dns string, available int) map[string]any {
	return map[string]any{"ip": ip, "dns": dns, "useip": "1", "available": strconv.Itoa(available)}
}

func (z *Zabbix) headerAuth() bool {
	var major, minor int
	fmt.Sscanf(z.Version, "%d.%d", &major, &minor)
	return major > 6 || (major == 6 && minor >= 4)
}

func (z *Zabbix) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api_jsonrpc.php" {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		Auth   string          `json:"auth"`
		ID     int             `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	z.mu.Lock()
	defer z.mu.Unlock()
	z.Calls = append(z.Calls, req.Method)
	reply := func(result any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "result": result, "id": req.ID})
	}
	fail := func(code int, msg, data string) {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "error": map[string]any{"code": code, "message": msg, "data": data}, "id": req.ID})
	}
	token := req.Auth
	if z.headerAuth() {
		if req.Auth != "" {
			fail(-32602, "Invalid params.", `Invalid parameter "/": unexpected parameter "auth".`)
			return
		}
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	switch req.Method {
	case "apiinfo.version":
		reply(z.Version)
		return
	case "user.login":
		var p map[string]string
		_ = json.Unmarshal(req.Params, &p)
		if p["username"] != z.User || p["password"] != z.Password {
			fail(-32500, "Application error.", "Incorrect user name or password or account is temporarily blocked.")
			return
		}
		sid := fmt.Sprintf("session-%d", len(z.sessions)+1)
		z.sessions[sid] = true
		reply(sid)
		return
	}
	if token == "" || (token != z.Token && !z.sessions[token]) {
		fail(-32602, "Invalid params.", "Not authorized.")
		return
	}
	switch req.Method {
	case "user.logout":
		delete(z.sessions, token)
		z.LoggedOut++
		reply(true)
	case "host.get":
		var p map[string]any
		_ = json.Unmarshal(req.Params, &p)
		groupKey := "groups"
		if _, ok := p["selectHostGroups"]; ok {
			groupKey = "hostgroups"
		}
		out := []map[string]any{}
		for _, h := range z.hosts {
			c := map[string]any{}
			for k, v := range h {
				if k == "groups" {
					k = groupKey
				}
				c[k] = v
			}
			out = append(out, c)
		}
		reply(out)
	default:
		fail(-32601, "Method not found.", req.Method)
	}
}

// Prometheus answers instant queries with the series it was given, whatever the query.
type Prometheus struct {
	URL     string
	Token   string
	mu      sync.Mutex
	series  []map[string]any
	Queries []string
}

func StartPrometheus(t *testing.T) *Prometheus {
	p := &Prometheus{}
	srv := httptest.NewServer(http.HandlerFunc(p.serve))
	t.Cleanup(srv.Close)
	p.URL = srv.URL
	return p
}

// Target adds an up series.
func (p *Prometheus) Target(job, instance string, up bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v := "0"
	if up {
		v = "1"
	}
	p.series = append(p.series, map[string]any{"metric": map[string]string{"__name__": "up", "job": job, "instance": instance}, "value": []any{1.7e9, v}})
}

func (p *Prometheus) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/query" {
		http.NotFound(w, r)
		return
	}
	if p.Token != "" && r.Header.Get("Authorization") != "Bearer "+p.Token {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "errorType": "unauthorized", "error": "unauthorized"})
		return
	}
	_ = r.ParseForm()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Queries = append(p.Queries, r.Form.Get("query"))
	series := p.series
	if series == nil {
		series = []map[string]any{}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": series}})
}
