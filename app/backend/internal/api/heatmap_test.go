package api

import (
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/demo"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

func TestHeatmapBuckets(t *testing.T) {
	st := store.New()
	demo.Seed(st)
	now := time.Date(2026, 10, 2, 12, 0, 30, 0, time.UTC)
	resolved := now.Add(-4 * time.Hour)
	var hm Heatmap
	st.Write(func(d *store.Data) {
		d.Alerts = map[string]*model.Alert{
			// active for the last 90 minutes: the last two hourly buckets
			"A1": {ID: "A1", CIID: "CI-6", CIName: "pay-app-01", Severity: model.SevCritical, Status: model.AlertOpen, FirstSeen: now.Add(-90 * time.Minute)},
			// 5 h ago for one hour, resolved
			"A2": {ID: "A2", CIID: "CI-6", CIName: "pay-app-01", Severity: model.SevWarning, Status: model.AlertResolved, FirstSeen: now.Add(-5 * time.Hour), ResolvedAt: &resolved},
			// outside the window
			"A3": {ID: "A3", CIID: "CI-6", CIName: "pay-app-01", Severity: model.SevError, Status: model.AlertResolved, FirstSeen: now.Add(-50 * time.Hour), ResolvedAt: ptr(now.Add(-49 * time.Hour))},
			// no CMDB record
			"A4": {ID: "A4", CIName: "unknown-host", Severity: model.SevError, Status: model.AlertOpen, FirstSeen: now.Add(-10 * time.Minute)},
			// suppressed by maintenance: not shown
			"A5": {ID: "A5", CIID: "CI-7", CIName: "pay-app-02", Severity: model.SevCritical, Status: model.AlertOpen, FirstSeen: now.Add(-10 * time.Minute), Suppressed: true},
		}
		hm = buildHeatmap(d, now, 24, "service", "", nil, true)
	})
	if hm.BucketMinutes != 60 || len(hm.Columns) != 24 {
		t.Fatalf("bucket = %d, columns = %d", hm.BucketMinutes, len(hm.Columns))
	}
	var row, unknown *HeatRow
	var rowGroup string
	for _, g := range hm.Groups {
		for _, r := range g.Rows {
			switch r.CIName {
			case "pay-app-01":
				row, rowGroup = r, g.Name
			case "unknown-host":
				unknown = r
			case "pay-app-02":
				t.Fatalf("suppressed alert or quiet CI shown with problemsOnly: %+v", r)
			}
		}
	}
	if row == nil || unknown == nil {
		t.Fatalf("rows missing: %+v", hm.Groups)
	}
	if rowGroup != "Платёжный шлюз" {
		t.Fatalf("group = %q", rowGroup)
	}
	if row.Total != 2 || row.Status != model.SevCritical {
		t.Fatalf("row = %+v", row)
	}
	if c := row.Cells[23]; c.N != 1 || c.Sev != model.SevCritical {
		t.Fatalf("last cell = %+v", c)
	}
	if c := row.Cells[22]; c.N != 1 {
		t.Fatalf("cell 22 = %+v", c)
	}
	if c := row.Cells[18]; c.N != 1 || c.Sev != model.SevWarning {
		t.Fatalf("cell 18 = %+v", c)
	}
	if c := row.Cells[10]; c.N != 0 {
		t.Fatalf("cell 10 = %+v", c)
	}
	if unknown.Open != 1 || unknown.Status != model.SevError {
		t.Fatalf("unknown = %+v", unknown)
	}
	if last := hm.Groups[len(hm.Groups)-1]; last.Key != "" {
		t.Fatalf("no-service group must be last, got %+v", last.Key)
	}
}

func ptr[T any](v T) *T { return &v }
