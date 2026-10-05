// Package netboxtest is an in-memory NetBox REST API for tests.
package netboxtest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const Token = "0123456789abcdef0123456789abcdef01234567"

type Server struct {
	URL string

	mu      sync.Mutex
	objects map[string]map[int]map[string]any
	next    int
	// Requests lists "METHOD path" of every API call.
	Requests []string
}

// Start serves an empty NetBox. Collections are named by their API path, such as dcim/devices.
func Start(t *testing.T) *Server {
	t.Helper()
	s := &Server{objects: map[string]map[int]map[string]any{}, next: 100}
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	s.URL = srv.URL
	return s
}

// Put stores an object as NetBox would return it; it must have an integer "id".
func (s *Server) Put(collection string, obj map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.objects[collection] == nil {
		s.objects[collection] = map[int]map[string]any{}
	}
	s.objects[collection][toInt(obj["id"])] = obj
}

func (s *Server) Get(collection string, id int) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.objects[collection][id]
}

func (s *Server) Remove(collection string, id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects[collection], id)
}

func (s *Server) Count(collection string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.objects[collection])
}

func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	}
	return 0
}

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
	s.Requests = append(s.Requests, r.Method+" "+path)
	if r.Header.Get("Authorization") != "Token "+Token {
		write(w, http.StatusForbidden, map[string]string{"detail": "Invalid token"})
		return
	}
	if path == "status" {
		write(w, http.StatusOK, map[string]any{"netbox-version": "4.1.3"})
		return
	}
	collection, id := path, 0
	if i := strings.LastIndex(path, "/"); i > 0 {
		if n, err := strconv.Atoi(path[i+1:]); err == nil {
			collection, id = path[:i], n
		}
	}
	objs := s.objects[collection]
	switch {
	case r.Method == http.MethodGet && id == 0:
		s.list(w, r, objs)
	case r.Method == http.MethodPost && id == 0:
		s.create(w, r, collection)
	case objs == nil || objs[id] == nil:
		write(w, http.StatusNotFound, map[string]string{"detail": "No " + collection + " matches the given query."})
	case r.Method == http.MethodGet:
		write(w, http.StatusOK, objs[id])
	case r.Method == http.MethodPatch:
		var patch map[string]any
		if json.NewDecoder(r.Body).Decode(&patch) != nil {
			write(w, http.StatusBadRequest, map[string]string{"detail": "bad JSON"})
			return
		}
		for k, v := range patch {
			if k == "status" {
				v = map[string]any{"value": v, "label": v}
			}
			objs[id][k] = v
		}
		write(w, http.StatusOK, objs[id])
	case r.Method == http.MethodDelete:
		delete(objs, id)
		w.WriteHeader(http.StatusNoContent)
	default:
		write(w, http.StatusMethodNotAllowed, map[string]string{"detail": "method not allowed"})
	}
}

func (s *Server) list(w http.ResponseWriter, r *http.Request, objs map[int]map[string]any) {
	ids := make([]int, 0, len(objs))
	for id := range objs {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 50
	}
	// Pages are capped below the requested size to exercise paging.
	limit = min(limit, 2)
	results := []map[string]any{}
	for i := offset; i < len(ids) && i < offset+limit; i++ {
		results = append(results, objs[ids[i]])
	}
	write(w, http.StatusOK, map[string]any{"count": len(ids), "next": nil, "results": results})
}

var required = map[string][]string{
	"dcim/devices":                    {"name", "site", "device_type", "role"},
	"virtualization/virtual-machines": {"name"},
}

func (s *Server) create(w http.ResponseWriter, r *http.Request, collection string) {
	var in map[string]any
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		write(w, http.StatusBadRequest, map[string]string{"detail": "bad JSON"})
		return
	}
	for _, f := range required[collection] {
		if in[f] == nil || in[f] == "" {
			write(w, http.StatusBadRequest, map[string][]string{f: {"This field is required."}})
			return
		}
	}
	s.next++
	obj := map[string]any{"id": s.next, "display": in["name"]}
	for k, v := range in {
		switch k {
		case "status":
			obj[k] = map[string]any{"value": v, "label": v}
		case "site", "device_type", "role", "cluster":
			obj[k] = map[string]any{"id": v, "name": collection + " ref"}
		default:
			obj[k] = v
		}
	}
	if s.objects[collection] == nil {
		s.objects[collection] = map[int]map[string]any{}
	}
	s.objects[collection][s.next] = obj
	write(w, http.StatusCreated, obj)
}
