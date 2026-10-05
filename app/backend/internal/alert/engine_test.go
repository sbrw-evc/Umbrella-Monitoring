package alert

import (
	"sync"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pipeline"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type fakePD struct {
	mu   sync.Mutex
	cmds []PDCommand
}

func (f *fakePD) Send(c PDCommand) { f.mu.Lock(); f.cmds = append(f.cmds, c); f.mu.Unlock() }

func setup() (*Engine, *fakePD, *store.Store, *time.Time) {
	st := store.New()
	st.Write(func(d *store.Data) {
		d.CIs["CI-1"] = &model.CI{ID: "CI-1", Name: "svc", Type: model.CIITService, Team: "t"}
		d.CIs["CI-2"] = &model.CI{ID: "CI-2", Name: "host1", Type: model.CIHost, Team: "t",
			Identities: []model.Identity{{Kind: "cloud_instance", Value: "i-old"}, {Kind: "cloud_instance", Value: "i-new"}}}
		d.Relations = []model.Relation{{From: "CI-1", To: "CI-2", Type: "runs_on"}}
	})
	pd := &fakePD{}
	e := New(st, pd, nil)
	now := time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)
	e.SetClock(func() time.Time { return now })
	return e, pd, st, &now
}

func draft(ci, sig string, sev model.Severity, status model.EventStatus, ext string) pipeline.Draft {
	return pipeline.Draft{CI: ci, Signal: sig, Severity: sev, Status: status, ExternalID: ext, Method: model.MethodUSE, Title: sig}
}

func TestDedupAcrossSourcesAndResolve(t *testing.T) {
	e, pd, st, _ := setup()
	e.Ingest(Source{ID: "A", Name: "A"}, []pipeline.Draft{draft("host1", "cpu", model.SevWarning, model.EventFiring, "1")})
	e.Ingest(Source{ID: "B", Name: "B"}, []pipeline.Draft{draft("i-new", "cpu", model.SevCritical, model.EventFiring, "x")})

	var alerts []*model.Alert
	st.Read(func(d *store.Data) {
		for _, a := range d.Alerts {
			alerts = append(alerts, a)
		}
	})
	if len(alerts) != 1 {
		t.Fatalf("want 1 alert, got %d", len(alerts))
	}
	a := alerts[0]
	if a.Severity != model.SevCritical || a.Count != 2 || a.Service != "svc" || a.CIID != "CI-2" {
		t.Errorf("alert = %+v", a)
	}
	if len(pd.cmds) != 2 || pd.cmds[0].Action != PDTrigger || pd.cmds[1].Alert.Severity != model.SevCritical {
		t.Errorf("pd commands = %+v", pd.cmds)
	}

	e.Ingest(Source{ID: "A", Name: "A"}, []pipeline.Draft{draft("host1", "cpu", model.SevWarning, model.EventResolved, "1")})
	if got, _ := e.Get(a.ID); got.Status != model.AlertOpen {
		t.Fatalf("one source still firing, status = %s", got.Status)
	}
	e.Ingest(Source{ID: "B", Name: "B"}, []pipeline.Draft{draft("i-new", "cpu", model.SevWarning, model.EventResolved, "x")})
	if got, _ := e.Get(a.ID); got.Status != model.AlertResolved {
		t.Fatalf("all sources ok, status = %s", got.Status)
	}
	if last := pd.cmds[len(pd.cmds)-1]; last.Action != PDResolve {
		t.Errorf("last pd action = %s", last.Action)
	}
}

func TestInboxDropsRetries(t *testing.T) {
	e, _, _, _ := setup()
	d := draft("host1", "cpu", model.SevWarning, model.EventFiring, "42")
	e.Ingest(Source{ID: "A", Name: "A"}, []pipeline.Draft{d})
	if got := e.Ingest(Source{ID: "A", Name: "A"}, []pipeline.Draft{d}); len(got) != 0 {
		t.Errorf("retry must be dropped, got %d events", len(got))
	}
}

func TestMaintenanceSuppresses(t *testing.T) {
	e, pd, st, now := setup()
	st.Write(func(d *store.Data) {
		d.Maintenance["MW-1"] = &model.Maintenance{ID: "MW-1", CIID: "CI-1", Start: now.Add(-time.Hour), End: now.Add(time.Hour)}
	})
	evs := e.Ingest(Source{ID: "A", Name: "A"}, []pipeline.Draft{draft("host1", "cpu", model.SevCritical, model.EventFiring, "1")})
	if !evs[0].Suppressed || len(pd.cmds) != 0 {
		t.Errorf("suppressed=%v pd=%d", evs[0].Suppressed, len(pd.cmds))
	}
}

func TestFallbackAfterTimeout(t *testing.T) {
	e, _, _, now := setup()
	evs := e.Ingest(Source{ID: "A", Name: "A"}, []pipeline.Draft{draft("host1", "cpu", model.SevError, model.EventFiring, "1")})
	e.PDResult(evs[0].AlertID, PDTrigger, "", errTest)
	*now = now.Add(3 * time.Minute)
	e.Tick()
	if a, _ := e.Get(evs[0].AlertID); !a.Fallback || a.PDState != model.PDFailed {
		t.Errorf("fallback=%v pd=%s", a.Fallback, a.PDState)
	}
}

func TestRetryAndCatchUpAfterOutage(t *testing.T) {
	e, pd, _, now := setup()
	id := e.Ingest(Source{ID: "A", Name: "A"}, []pipeline.Draft{draft("host1", "cpu", model.SevError, model.EventFiring, "1")})[0].AlertID
	e.PDResult(id, PDTrigger, "", errTest)
	if _, err := e.Act(id, "ack", "duty", ""); err != nil {
		t.Fatal(err)
	}
	e.PDResult(id, PDAcknowledge, "", errTest)
	sent := len(pd.cmds)
	e.Tick()
	if len(pd.cmds) != sent {
		t.Fatalf("retried before RetryEvery: %d", len(pd.cmds)-sent)
	}
	*now = now.Add(61 * time.Second)
	e.Tick()
	if len(pd.cmds) != sent+1 || pd.cmds[len(pd.cmds)-1].Action != PDTrigger {
		t.Fatalf("trigger not retried: %+v", pd.cmds[sent:])
	}
	e.PDResult(id, PDTrigger, "default", nil)
	if last := pd.cmds[len(pd.cmds)-1]; last.Action != PDAcknowledge {
		t.Fatalf("acknowledge not caught up, last = %s", last.Action)
	}
	e.PDResult(id, PDAcknowledge, "default", nil)
	if a, _ := e.Get(id); a.PDState != model.PDAcked || a.PDRetry != "" || a.PDRoute != "default" {
		t.Fatalf("state = %s retry = %q route = %q", a.PDState, a.PDRetry, a.PDRoute)
	}
	*now = now.Add(time.Hour)
	before := len(pd.cmds)
	e.Tick()
	if len(pd.cmds) != before {
		t.Fatal("delivered alert re-sent")
	}
}

func TestBelowThresholdIsSkipped(t *testing.T) {
	e, _, _, now := setup()
	id := e.Ingest(Source{ID: "A", Name: "A"}, []pipeline.Draft{draft("host1", "cpu", model.SevError, model.EventFiring, "1")})[0].AlertID
	e.PDResult(id, PDTrigger, "", ErrPDSkipped)
	*now = now.Add(time.Hour)
	e.Tick()
	if a, _ := e.Get(id); a.PDState != model.PDSkipped || a.Fallback {
		t.Fatalf("state = %s fallback = %v", a.PDState, a.Fallback)
	}
}

func TestRedUseLink(t *testing.T) {
	e, _, _, _ := setup()
	use := e.Ingest(Source{ID: "A", Name: "A"}, []pipeline.Draft{draft("host1", "cpu", model.SevError, model.EventFiring, "1")})
	red := draft("svc", "red.errors", model.SevCritical, model.EventFiring, "2")
	red.Method = model.MethodRED
	r := e.Ingest(Source{ID: "B", Name: "B"}, []pipeline.Draft{red})
	if a, _ := e.Get(r[0].AlertID); a.RelatedID != use[0].AlertID {
		t.Errorf("related = %q, want %q", a.RelatedID, use[0].AlertID)
	}
}

type testErr struct{}

func (testErr) Error() string { return "down" }

var errTest = testErr{}

func TestRedeliveredEventRebindsOrphanedIncident(t *testing.T) {
	e, _, st, _ := setup()
	e.Ingest(Source{ID: "Z", Name: "Zabbix"}, []pipeline.Draft{draft("host1", "zabbix:7", model.SevError, model.EventFiring, "901")})
	st.Write(func(d *store.Data) {
		d.DeleteCI("CI-2")
		d.CIs["CI-9"] = &model.CI{ID: "CI-9", Name: "host1", Type: model.CIHost, Team: "infra"}
	})
	e.Ingest(Source{ID: "Z", Name: "Zabbix"}, []pipeline.Draft{draft("host1", "zabbix:7", model.SevError, model.EventFiring, "901")})

	var alerts []model.Alert
	st.Read(func(d *store.Data) {
		for _, a := range d.Alerts {
			alerts = append(alerts, *a)
		}
	})
	if len(alerts) != 1 {
		t.Fatalf("redelivery must not open a second incident: %+v", alerts)
	}
	if a := alerts[0]; a.CIID != "CI-9" || a.DedupKey != "CI-9|zabbix:7" || a.Count != 1 {
		t.Fatalf("incident not rebound: %+v", a)
	}

	e.Ingest(Source{ID: "Z", Name: "Zabbix"}, []pipeline.Draft{draft("host1", "zabbix:7", model.SevError, model.EventResolved, "901")})
	st.Read(func(d *store.Data) {
		for _, a := range d.Alerts {
			if a.Status.Active() {
				t.Fatalf("recovery did not reach the rebound incident: %+v", a)
			}
		}
	})
}
