package alert_test

import (
	"context"
	"testing"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// templatePolicy turns the impact policy of the test catalog back to the default: the template.
func templatePolicy(st *store.Store) {
	st.Write(func(d *store.Data) { d.Settings.Impact = model.ImpactPolicy{} })
}

func only(t *testing.T, e *alert.Engine) alert.Alert {
	t.Helper()
	list := active(t, e)
	if len(list) != 1 {
		t.Fatalf("active alerts = %d", len(list))
	}
	return list[0]
}

func entriesOf(t *testing.T, e *alert.Engine, id, code string) []alert.Entry {
	t.Helper()
	_, entries, err := e.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	var out []alert.Entry
	for _, en := range entries {
		if en.Code == code {
			out = append(out, en)
		}
	}
	return out
}

func TestPriorityFromImpact(t *testing.T) {
	ctx := context.Background()
	e, st, rec, _ := setup(t)
	templatePolicy(st)

	// A P3 event on an item of the critical Payments service is a P2 incident; PagerDuty gets P2.
	if err := e.Ingest(ctx, []alert.Incoming{ev("CON-1", "z", "app-01", "disk", "warning", "firing")}); err != nil {
		t.Fatal(err)
	}
	a := only(t, e)
	if a.EventSeverity != "warning" || a.Severity != "error" || a.Impact == nil || a.Impact.Level != "critical" ||
		len(a.Impact.Services) != 1 || a.Impact.Services[0].Name != "Payments" || !a.Impact.Services[0].Direct {
		t.Fatalf("alert = %s/%s %+v", a.EventSeverity, a.Severity, a.Impact)
	}
	cmds := rec.take()
	if len(cmds) != 1 || cmds[0].Action != alert.PDTrigger || cmds[0].Alert.Severity != "error" {
		t.Fatalf("PagerDuty gets the priority: %+v", cmds)
	}
	set := entriesOf(t, e, a.ID, "priority_set")
	if len(set) != 1 || set[0].Args["event"] != "warning" || set[0].Args["impact"] != "critical" || set[0].Args["severity"] != "error" {
		t.Errorf("priority_set = %+v", set)
	}

	// A critical source raises the event severity: P1 stays P1.
	e.Ingest(ctx, []alert.Incoming{ev("CON-2", "p", "app-01", "disk", "critical", "firing")})
	if a = only(t, e); a.Severity != "critical" || a.EventSeverity != "critical" {
		t.Fatalf("raised = %s/%s", a.EventSeverity, a.Severity)
	}
	if cmds = rec.take(); len(cmds) != 1 || cmds[0].Alert.Severity != "critical" || len(entriesOf(t, e, a.ID, "severity_raised")) != 1 {
		t.Errorf("raised: %+v", cmds)
	}
	// It stops firing: the event severity is P3 again, the priority P2.
	e.Ingest(ctx, []alert.Incoming{ev("CON-2", "p", "app-01", "disk", "critical", "resolved")})
	if a = only(t, e); a.Severity != "error" || a.EventSeverity != "warning" {
		t.Fatalf("lowered = %s/%s", a.EventSeverity, a.Severity)
	}
	rec.take()

	// The catalog changes: Payments becomes low. The priority follows without a new event and
	// a lower priority is not sent to PagerDuty.
	st.Write(func(d *store.Data) { d.Services["S-1"].Criticality = model.CriticalityLow })
	if err := e.Reroute(ctx); err != nil {
		t.Fatal(err)
	}
	if a = only(t, e); a.Severity != "warning" || a.Impact.Level != "low" {
		t.Fatalf("after the catalog change: %s %+v", a.Severity, a.Impact)
	}
	if cmds = rec.take(); len(cmds) != 0 {
		t.Errorf("a lower priority is not sent: %+v", cmds)
	}

	// Payments now depends on Shop (critical) through a new dependency: the incident affects
	// Shop upstream and rises to P2 again, which PagerDuty gets.
	st.Write(func(d *store.Data) {
		d.Services["S-9"] = &model.Service{ID: "S-9", Name: "Shop", Criticality: model.CriticalityCritical, Status: model.ServiceActive,
			DependsOn: []string{"S-1"}}
	})
	if err := e.Reroute(ctx); err != nil {
		t.Fatal(err)
	}
	a = only(t, e)
	if a.Severity != "error" || a.Impact.Level != "critical" || len(a.Impact.Services) != 2 || a.Impact.Services[0].Name != "Shop" || a.Impact.Services[0].Direct {
		t.Fatalf("upstream: %s %+v", a.Severity, a.Impact)
	}
	if cmds = rec.take(); len(cmds) != 1 || cmds[0].Action != alert.PDTrigger || cmds[0].Alert.Severity != "error" {
		t.Errorf("the raised priority goes to PagerDuty: %+v", cmds)
	}
	// Nothing changed: nothing is written again.
	before := len(entriesOf(t, e, a.ID, "priority_set"))
	e.Reroute(ctx)
	if after := len(entriesOf(t, e, a.ID, "priority_set")); after != before || len(rec.take()) != 0 {
		t.Errorf("a second reroute changes nothing: %d -> %d", before, after)
	}

	// A rule of the policy: everything on app-01 is P1.
	st.Write(func(d *store.Data) {
		p := model.DefaultImpactPolicy()
		p.Rules = []model.ClassificationRule{{ID: "app", Name: "App servers", Enabled: true, When: "ci == 'app-01'", Action: "set", Severity: "critical"}}
		d.Settings.Impact, _ = p.Normalize()
	})
	e.Reroute(ctx)
	if a = only(t, e); a.Severity != "critical" || a.Impact.Rule != "App servers" {
		t.Fatalf("rule: %s %+v", a.Severity, a.Impact)
	}
	set = entriesOf(t, e, a.ID, "priority_set")
	if last := set[len(set)-1]; last.Args["rule"] != "App servers" || last.Args["from"] != "error" {
		t.Errorf("priority_set = %+v", last)
	}
}

func TestPriorityPolicyDisabled(t *testing.T) {
	ctx := context.Background()
	e, _, rec, _ := setup(t)
	// The test catalog has the policy off: the priority is the event severity, the impact is shown.
	e.Ingest(ctx, []alert.Incoming{ev("CON-1", "z", "app-01", "disk", "warning", "firing")})
	a := only(t, e)
	if a.Severity != "warning" || a.EventSeverity != "warning" || a.Impact == nil || a.Impact.Level != "critical" || len(a.Impact.Services) != 1 {
		t.Fatalf("disabled: %s %+v", a.Severity, a.Impact)
	}
	if cmds := rec.take(); len(cmds) != 1 || cmds[0].Alert.Severity != "warning" {
		t.Errorf("cmds = %+v", cmds)
	}
	if n := len(entriesOf(t, e, a.ID, "priority_set")); n != 0 {
		t.Errorf("no priority_set: %d", n)
	}
}

// An alert stored before priorities has no event severity: its severity is taken as one, and
// the policy applies to it at the next catalog change.
func TestPriorityOfOldAlert(t *testing.T) {
	ctx := context.Background()
	e, st, rec, _ := setup(t)
	e.Ingest(ctx, []alert.Incoming{ev("CON-1", "z", "lonely", "ping", "error", "firing")})
	if _, err := e.DB().Exec(ctx, `UPDATE alerts SET doc = doc - 'event_severity' - 'impact'`); err != nil {
		t.Fatal(err)
	}
	rec.take()
	templatePolicy(st)
	if err := e.Reroute(ctx); err != nil {
		t.Fatal(err)
	}
	// lonely is in no service: no impact, the template lowers P2 to P3.
	a := only(t, e)
	if a.EventSeverity != "error" || a.Severity != "warning" || a.Impact == nil || a.Impact.Level != model.ImpactNone {
		t.Fatalf("old alert: %s/%s %+v", a.EventSeverity, a.Severity, a.Impact)
	}
	if set := entriesOf(t, e, a.ID, "priority_set"); len(set) != 1 || set[0].Args["impact"] != "none" {
		t.Errorf("priority_set = %+v", set)
	}
	// A new event of the same severity keeps the priority.
	e.Ingest(ctx, []alert.Incoming{ev("CON-2", "y", "lonely", "ping", "error", "firing")})
	if a = only(t, e); a.Severity != "warning" {
		t.Errorf("after an event: %s", a.Severity)
	}
}

// An alert that waited for its item gets its priority when it is bound.
func TestPriorityOnBind(t *testing.T) {
	ctx := context.Background()
	e, st, rec, _ := setup(t)
	templatePolicy(st)
	e.Ingest(ctx, []alert.Incoming{ev("CON-1", "a", "edge-07", "ping", "warning", "firing")})
	a := only(t, e)
	if a.Severity != "warning" || a.Impact.Level != "" {
		t.Fatalf("unknown item: %s %+v", a.Severity, a.Impact)
	}
	rec.take()
	st.Write(func(d *store.Data) { d.ConfigItems["CI-2"].Aliases = []string{"edge-07"} })
	if _, err := e.BindUnknown(ctx, "admin"); err != nil {
		t.Fatal(err)
	}
	if a = only(t, e); a.Severity != "error" || a.Impact.Level != "critical" {
		t.Fatalf("bound: %s %+v", a.Severity, a.Impact)
	}
	if cmds := rec.take(); len(cmds) != 1 || cmds[0].Alert.Severity != "error" {
		t.Errorf("cmds = %+v", cmds)
	}
}
