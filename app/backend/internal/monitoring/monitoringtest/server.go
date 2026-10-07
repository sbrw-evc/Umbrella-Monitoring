// Package monitoringtest runs fake Zabbix, Prometheus and Grafana servers for tests.
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
	items     []item
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

type item struct {
	hostID, itemID, name, key, units string
	valueType                        int
	values                           [][2]float64
}

// AddItem adds a numeric item of a host with its values, [unix seconds, value]. Value type 0
// is float, 3 unsigned.
func (z *Zabbix) AddItem(hostID, itemID int, name, key, units string, valueType int, values ...[2]float64) {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.items = append(z.items, item{hostID: strconv.Itoa(hostID), itemID: strconv.Itoa(itemID), name: name, key: key, units: units, valueType: valueType, values: values})
}

// keyMatches: the * of a search pattern is any text and the pattern may be anywhere in the key.
func keyMatches(pattern, key string) bool {
	parts := strings.Split(pattern, "*")
	i := strings.Index(key, parts[0])
	if i < 0 {
		return false
	}
	rest := key[i+len(parts[0]):]
	for _, p := range parts[1:] {
		j := strings.Index(rest, p)
		if j < 0 {
			return false
		}
		rest = rest[j+len(p):]
	}
	return true
}

func (z *Zabbix) itemGet(p map[string]any) []map[string]any {
	hosts, _ := p["hostids"].([]any)
	filter, _ := p["filter"].(map[string]any)
	search, _ := p["search"].(map[string]any)
	out := []map[string]any{}
	for _, it := range z.items {
		if len(hosts) > 0 && hosts[0] != it.hostID {
			continue
		}
		if k, ok := filter["key_"].(string); ok && k != it.key {
			continue
		}
		if k, ok := search["key_"].(string); ok && !keyMatches(k, it.key) {
			continue
		}
		out = append(out, map[string]any{"itemid": it.itemID, "name": it.name, "key_": it.key, "value_type": strconv.Itoa(it.valueType), "units": it.units})
	}
	return out
}

func (z *Zabbix) values(p map[string]any, trend bool) []map[string]any {
	ids, _ := p["itemids"].([]any)
	from, _ := p["time_from"].(float64)
	till, _ := p["time_till"].(float64)
	out := []map[string]any{}
	for _, it := range z.items {
		want := false
		for _, id := range ids {
			want = want || id == it.itemID
		}
		if !want {
			continue
		}
		if vt, ok := p["history"].(float64); ok && int(vt) != it.valueType {
			continue
		}
		for _, v := range it.values {
			if v[0] < from || v[0] > till {
				continue
			}
			row := map[string]any{"itemid": it.itemID, "clock": strconv.FormatInt(int64(v[0]), 10)}
			val := strconv.FormatFloat(v[1], 'f', -1, 64)
			if trend {
				row["value_avg"] = val
			} else {
				row["value"] = val
			}
			out = append(out, row)
		}
	}
	return out
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
	case "item.get", "history.get", "trend.get":
		var p map[string]any
		_ = json.Unmarshal(req.Params, &p)
		switch req.Method {
		case "item.get":
			reply(z.itemGet(p))
		case "history.get":
			reply(z.values(p, false))
		default:
			reply(z.values(p, true))
		}
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
	ranges  []map[string]any
	Queries []string
}

// Range adds a series every range query answers with: labels and [unix seconds, value] points.
func (p *Prometheus) Range(labels map[string]string, points ...[2]float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	vals := make([][2]any, 0, len(points))
	for _, pt := range points {
		vals = append(vals, [2]any{pt[0], strconv.FormatFloat(pt[1], 'f', -1, 64)})
	}
	p.ranges = append(p.ranges, map[string]any{"metric": labels, "values": vals})
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
	if r.URL.Path != "/api/v1/query" && r.URL.Path != "/api/v1/query_range" {
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
	if r.URL.Path == "/api/v1/query_range" {
		ranges := p.ranges
		if ranges == nil {
			ranges = []map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "matrix", "result": ranges}})
		return
	}
	series := p.series
	if series == nil {
		series = []map[string]any{}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": series}})
}
