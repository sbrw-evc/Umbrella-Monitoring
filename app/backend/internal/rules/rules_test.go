package rules

import (
	"context"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/integration"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pipeline"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type fakeQ struct{ samples []integration.MetricSample }

func (f *fakeQ) Query(context.Context, string, string) ([]integration.MetricSample, error) {
	return f.samples, nil
}

type sink struct{ drafts []pipeline.Draft }

func (s *sink) Ingest(_ alert.Source, d []pipeline.Draft) []model.Event {
	s.drafts = append(s.drafts, d...)
	return nil
}

func TestFiringAfterHoldAndResolve(t *testing.T) {
	st := store.New()
	r := model.Rule{ID: "RUL-1", Name: "CPU", Method: model.MethodUSE, Signal: "use.cpu.utilization", SourceID: "INT-1",
		Query: "cpu", Op: ">", Threshold: 50, For: "1m", Interval: "30s", Severity: model.SevWarning, Enabled: true}
	if err := Normalize(&r); err != nil {
		t.Fatal(err)
	}
	st.Write(func(d *store.Data) { d.Rules[r.ID] = &r })
	q := &fakeQ{samples: []integration.MetricSample{
		{Labels: map[string]string{"host": "web-1", "instance": "web-1:9100"}, Value: 80},
		{Labels: map[string]string{"host": "web-2", "instance": "web-2:9100"}, Value: 10},
	}}
	out := &sink{}
	e := New(st, q, out)
	now := time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)
	e.SetClock(func() time.Time { return now })
	ctx := context.Background()

	e.Evaluate(ctx, r.ID)
	if len(out.drafts) != 0 {
		t.Fatalf("fired before hold: %+v", out.drafts)
	}
	now = now.Add(90 * time.Second)
	e.Evaluate(ctx, r.ID)
	if len(out.drafts) != 1 || out.drafts[0].CI != "web-1" || out.drafts[0].Status != model.EventFiring || out.drafts[0].Method != model.MethodUSE {
		t.Fatalf("drafts = %+v", out.drafts)
	}
	e.Evaluate(ctx, r.ID)
	if len(out.drafts) != 1 {
		t.Fatal("firing sent twice")
	}
	st.Read(func(d *store.Data) {
		if d.Rules[r.ID].Firing != 1 || d.Rules[r.ID].SeriesCount != 2 {
			t.Fatalf("counters %+v", d.Rules[r.ID])
		}
	})
	q.samples = q.samples[1:]
	e.Evaluate(ctx, r.ID)
	if len(out.drafts) != 2 || out.drafts[1].Status != model.EventResolved || out.drafts[1].ExternalID != out.drafts[0].ExternalID {
		t.Fatalf("resolve = %+v", out.drafts)
	}
}

func TestNormalizeRejects(t *testing.T) {
	bad := []model.Rule{
		{Name: "x", Method: "rate", SourceID: "s", Query: "q", Op: ">", Severity: model.SevInfo},
		{Name: "x", Method: model.MethodRED, SourceID: "s", Query: "q", Op: "=>", Severity: model.SevInfo},
		{Name: "x", Method: model.MethodRED, SourceID: "s", Query: "q", Op: ">", Severity: model.SevInfo, Interval: "1s"},
		{Name: "x", Method: model.MethodRED, Query: "q", Op: ">", Severity: model.SevInfo},
	}
	for i, r := range bad {
		if err := Normalize(&r); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
	for _, tpl := range Templates() {
		tpl.SourceID = "INT-1"
		if err := Normalize(&tpl); err != nil {
			t.Fatalf("template %s: %v", tpl.Name, err)
		}
	}
}
