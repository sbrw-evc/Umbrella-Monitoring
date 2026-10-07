package monitoring_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/monitoring"
)

// TestZabbixLive reads hosts and a graph from a real Zabbix: UMBRELLA_ZABBIX_URL (web interface),
// UMBRELLA_ZABBIX_USER and UMBRELLA_ZABBIX_PASSWORD, or UMBRELLA_ZABBIX_TOKEN for an API token.
func TestZabbixLive(t *testing.T) {
	url := os.Getenv("UMBRELLA_ZABBIX_URL")
	if url == "" {
		t.Skip("UMBRELLA_ZABBIX_URL is not set")
	}
	auth := &monitoring.Auth{Type: "basic", Fields: map[string]string{"username": os.Getenv("UMBRELLA_ZABBIX_USER")}, Secrets: map[string]string{"password": os.Getenv("UMBRELLA_ZABBIX_PASSWORD")}}
	if tok := os.Getenv("UMBRELLA_ZABBIX_TOKEN"); tok != "" {
		auth = &monitoring.Auth{Type: "bearer", Secrets: map[string]string{"token": tok}}
	}
	src := model.MonitoringSource{Kind: model.MonitoringZabbix, URL: url}
	res, err := monitoring.Fetch(context.Background(), src, auth)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hosts) == 0 {
		t.Fatalf("Zabbix %s returned no hosts", res.Version)
	}
	t.Logf("Zabbix %s: %d hosts, first %+v", res.Version, len(res.Hosts), res.Hosts[0])
	for _, span := range []time.Duration{time.Hour, 7 * 24 * time.Hour} {
		now := time.Now()
		series, err := monitoring.Metrics(context.Background(), src, auth, res.Hosts[0], monitoring.MetricQuery{
			Panel: model.HostPanel{ZabbixKey: "*"}, From: now.Add(-span), To: now})
		if err != nil {
			t.Fatalf("graph over %s: %v", span, err)
		}
		t.Logf("graph over %s: %d series", span, len(series))
	}
}
