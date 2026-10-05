package app

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/ingest"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

func TestEventKeys(t *testing.T) {
	for in, want := range map[string][]string{
		"srv-db-01.corp.local:9100": {"srv-db-01.corp.local:9100", "srv-db-01.corp.local", "srv-db-01"},
		"10.0.0.1:9100":             {"10.0.0.1:9100", "10.0.0.1"},
		"SRV-APP-01":                {"srv-app-01"},
	} {
		if got := eventKeys(in); !slices.Equal(got, want) {
			t.Errorf("eventKeys(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCMDBEvents(t *testing.T) {
	st := store.New()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	st.Write(func(d *store.Data) {
		d.ConfigItems["CI-1"] = &model.ConfigItem{ID: "CI-1", Name: "srv-db-01", Status: model.CIStatusActive, IPs: []string{"10.0.0.1"}}
		d.ConfigItems["CI-2"] = &model.ConfigItem{ID: "CI-2", Name: "srv-app-01.corp.local", Status: model.CIStatusActive,
			Directory: &model.CIDirectory{Status: model.DirectoryMissing}}
		d.ConfigItems["CI-3"] = &model.ConfigItem{ID: "CI-3", Name: "vm-new", Status: model.CIStatusPlanned}
		d.ConfigItems["CI-4"] = &model.ConfigItem{ID: "CI-4", Name: "unbound", Status: model.CIStatusFailed}
		d.Services["SVC-1"] = &model.Service{ID: "SVC-1", Name: "Billing", Status: model.ServiceActive, CIIDs: []string{"CI-1", "CI-3"}}
		d.Services["SVC-2"] = &model.Service{ID: "SVC-2", Name: "Shop", Status: model.ServiceActive, CIIDs: []string{"CI-2"}, DependsOn: []string{"SVC-1"}}
		d.Services["SVC-3"] = &model.Service{ID: "SVC-3", Name: "Old", Status: model.ServiceRetired, CIIDs: []string{"CI-1"}}
		d.Services["SVC-4"] = &model.Service{ID: "SVC-4", Name: "Lab", Status: model.ServiceActive, CIIDs: []string{"CI-3"}}
	})
	var since time.Time
	s := NewCMDBService(st, func(_ context.Context, from time.Time) ([]ingest.FiringEvent, error) {
		since = from
		return []ingest.FiringEvent{
			{CI: "10.0.0.1:9100", Severity: "critical", Title: "Disk full", LastSeen: now},
			{CI: "SRV-DB-01", Severity: "warning", Title: "Slow queries", LastSeen: now},
			{CI: "srv-app-01:443", Severity: "info", Title: "Deploy", LastSeen: now},
			{CI: "nobody", Severity: "critical", Title: "Lost", LastSeen: now},
		}, nil
	})
	s.now = func() time.Time { return now }
	m := s.Map(context.Background())
	if !m.Events.Available || !since.Equal(now.Add(-24*time.Hour)) || len(m.CIs) != 3 {
		t.Fatalf("map = %+v since %v", m, since)
	}
	ci := map[string]MapCI{}
	for _, c := range m.CIs {
		ci[c.ID] = c
	}
	if e := ci["CI-1"].Events; e.Critical != 1 || e.Warning != 1 || len(e.Recent) != 2 || ci["CI-1"].Health.Level != HealthCritical {
		t.Fatalf("CI-1 = %+v", ci["CI-1"])
	}
	if c := ci["CI-2"]; c.Events.Info != 1 || c.Health.Level != HealthWarning || c.Health.Reasons[0].Code != "directory_missing" {
		t.Fatalf("CI-2 = %+v", c)
	}
	if ci["CI-3"].Health.Level != HealthUnknown {
		t.Fatalf("CI-3 = %+v", ci["CI-3"])
	}
	svc := map[string]Health{}
	for _, x := range m.Services {
		svc[x.ID] = x.Health
	}
	if h := svc["SVC-2"]; h.Level != HealthWarning || len(h.Reasons) != 2 || h.Reasons[1].Code != "dependency_critical" || h.Reasons[1].Ref != "SVC-1" {
		t.Fatalf("SVC-2 = %+v", h)
	}
	if svc["SVC-1"].Level != HealthCritical || svc["SVC-3"].Level != HealthUnknown || svc["SVC-3"].Reasons[0].Code != "service_retired" {
		t.Fatalf("services = %+v", svc)
	}
	if h := svc["SVC-4"]; h.Level != HealthUnknown || h.Reasons[0].Code != "cis_unknown" {
		t.Fatalf("SVC-4 = %+v", h)
	}

	// Without the event tables the map still comes, and says why events are missing.
	s.firing = func(context.Context, time.Time) ([]ingest.FiringEvent, error) { return nil, errors.New("down") }
	if m := s.Map(context.Background()); m.Events.Available || m.Events.Error != "down" || len(m.CIs) != 3 {
		t.Fatalf("no events = %+v", m.Events)
	}
}
