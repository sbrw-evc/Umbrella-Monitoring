package logs_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/logs"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

func TestLoki(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/query_range" || r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		got = r.URL.Query().Get("query")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "streams", "result": []any{
			map[string]any{"stream": map[string]string{"host": "web-01", "level": "ERROR"}, "values": [][2]string{{"1700000060000000000", "disk full\n"}, {"1700000000000000000", "retrying"}}},
			map[string]any{"stream": map[string]string{"host": "web-01"}, "values": [][2]string{{"1700000030000000000", "hello"}}},
		}}})
	}))
	defer srv.Close()
	src := model.LogSource{Kind: model.LogLoki, URL: srv.URL}
	from := time.Unix(1_699_999_000, 0)
	res, err := logs.Fetch(context.Background(), src, &logs.Auth{Type: "bearer", Secrets: map[string]string{"token": "tok"}},
		logs.Query{Hosts: []string{"web-01", "10.0.0.1", "WEB-01"}, From: from, To: from.Add(time.Hour), Limit: 2, Text: "disk."})
	if err != nil {
		t.Fatal(err)
	}
	if got != `{host=~"(?i)(10\\.0\\.0\\.1|web-01)"} |~ "(?i)disk\\."` {
		t.Fatalf("query %s", got)
	}
	if len(res.Lines) != 2 || !res.Truncated || res.Lines[0].Text != "disk full" || res.Lines[0].Level != "error" || res.Lines[1].Text != "hello" {
		t.Fatalf("lines %+v", res)
	}
}

func TestOpenSearch(t *testing.T) {
	var body map[string]any
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]any{"hits": map[string]any{"hits": []any{
			map[string]any{"_index": "logs-1", "_source": map[string]any{"@timestamp": "2023-11-14T22:14:20Z", "message": "oom killer", "host": map[string]any{"name": "db-01"}, "log.level": "WARN"}},
			map[string]any{"_index": "logs-1", "_source": map[string]any{"@timestamp": "2023-11-14T22:15:20.5Z", "message": "started", "host": map[string]any{"name": "db-01"}}},
		}}})
	}))
	defer srv.Close()
	src := model.LogSource{Kind: model.LogOpenSearch, URL: srv.URL, Index: "logs-*"}
	from := time.Unix(1_699_999_000, 0)
	res, err := logs.Fetch(context.Background(), src, nil, logs.Query{Hosts: []string{"db-01"}, From: from, To: from.Add(time.Hour), Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/logs-*/_search" {
		t.Fatalf("path %s", path)
	}
	b, _ := json.Marshal(body)
	if !strings.Contains(string(b), `"host.name":"db-01"`) || !strings.Contains(string(b), `"@timestamp"`) {
		t.Fatalf("body %s", b)
	}
	if len(res.Lines) != 2 || res.Lines[0].Text != "started" || res.Lines[1].Level != "warn" || res.Lines[1].Labels["host"] != "db-01" {
		t.Fatalf("lines %+v", res.Lines)
	}
}

func TestFetchNeedsAName(t *testing.T) {
	from := time.Unix(1_699_999_000, 0)
	if _, err := logs.Fetch(context.Background(), model.LogSource{Kind: model.LogLoki, URL: "http://x"}, nil, logs.Query{From: from, To: from.Add(time.Hour)}); err == nil {
		t.Fatal("no names: want an error")
	}
}
