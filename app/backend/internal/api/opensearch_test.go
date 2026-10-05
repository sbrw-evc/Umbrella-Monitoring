package api

import (
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

func TestOpenSearchPullIntegration(t *testing.T) {
	env := newEnv(t, false)
	var mu sync.Mutex
	var queries []string
	var auth string
	os := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		queries = append(queries, r.URL.Path+" "+string(b))
		auth = r.Header.Get("Authorization")
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"hits": map[string]any{"hits": []any{map[string]any{
			"_index": "pg-logs-2026.10.02", "_id": "doc-1",
			"_source": map[string]any{"@timestamp": "2026-10-02T10:00:00Z", "level": "error", "host": "zabbix-db",
				"service": "postgresql", "message": "relation does not exist", "error_code": "SQLSTATE_42P01"},
		}}}})
	}))
	t.Cleanup(os.Close)
	var it struct {
		ID          string
		ConnectorID string `json:"connector_id"`
	}
	body := `{"type":"opensearch","name":"PG logs","url":"` + os.URL + `","auth_type":"basic","username":"reader","secret":"os-pass",
	  "params":{"index":"pg-logs-*","interval":"5s","levels":"fatal,error,warning"}}`
	if code := do(t, "POST", env.ts.URL+"/api/integrations", body, &it); code != 201 {
		t.Fatalf("create code = %d", code)
	}
	go env.rt.Run(t.Context())
	waitFor(t, "incident from OpenSearch", func() bool {
		found := false
		env.st.Read(func(d *store.Data) {
			for _, a := range d.Alerts {
				found = found || (a.CIName == "zabbix-db" && a.Signal == "opensearch:postgresql:SQLSTATE_42P01" && a.Severity == model.SevError)
			}
		})
		return found
	})
	mu.Lock()
	q := queries[0]
	a := auth
	mu.Unlock()
	if !strings.HasPrefix(q, "/pg-logs-*/_search") || !strings.Contains(q, `"ERROR"`) || !strings.Contains(q, "now-5m") || a == "" || !strings.HasPrefix(a, "Basic ") {
		t.Fatalf("query %q auth %q", q, a)
	}
	for i := 0; i < 100; i++ {
		mu.Lock()
		n := len(queries)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	env.st.Read(func(d *store.Data) {
		n := 0
		for _, ev := range d.Events {
			if ev.ExternalID == "doc-1" {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("document polled twice became %d events", n)
		}
	})
}
