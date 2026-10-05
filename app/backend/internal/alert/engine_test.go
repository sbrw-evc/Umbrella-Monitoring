package alert_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store/storetest"
)

type recorder struct {
	mu       sync.Mutex
	cmds     []alert.Command
	fallback []alert.Alert
}

func (r *recorder) Send(c alert.Command) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cmds = append(r.cmds, c)
}

func (r *recorder) Fallback(a alert.Alert) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallback = append(r.fallback, a)
}

func (r *recorder) take() []alert.Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.cmds
	r.cmds = nil
	return out
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func catalog() *store.Store {
	st := store.New()
	st.Write(func(d *store.Data) {
		d.Users["U-1"] = &model.User{ID: "U-1", Username: "lead", Name: "Lead One", TeamID: "T-2", Profile: model.Profile{Email: "lead@example.com"}, Telegram: "1001"}
		d.Users["U-2"] = &model.User{ID: "U-2", Username: "eng", Name: "Engineer Two", TeamID: "T-2", Profile: model.Profile{Email: "eng@example.com"}}
		d.Users["U-3"] = &model.User{ID: "U-3", Username: "owner", Name: "Owner Three", Profile: model.Profile{Email: "owner@example.com"}}
		d.Users["U-4"] = &model.User{ID: "U-4", Username: "gone", Name: "Gone", TeamID: "T-2", Disabled: true}
		d.Teams["T-1"] = &model.Team{ID: "T-1", Name: "Platform"}
		d.Teams["T-2"] = &model.Team{ID: "T-2", Name: "Payments SRE", ParentID: "T-1", LeadID: "U-1"}
		d.Teams["T-3"] = &model.Team{ID: "T-3", Name: "Empty", ParentID: "T-2"}
		d.ConfigItems["CI-1"] = &model.ConfigItem{ID: "CI-1", Name: "db-01.example.com", Kind: model.CIKindVM, Status: model.CIStatusActive,
			IPs: []string{"10.0.0.11"}, Owners: []model.CIOwner{{UserID: "U-3", Role: "technical"}}}
		d.ConfigItems["CI-2"] = &model.ConfigItem{ID: "CI-2", Name: "app-01", Kind: model.CIKindVM, Status: model.CIStatusActive,
			Owners: []model.CIOwner{{UserID: "U-3", Role: "technical"}}}
		d.ConfigItems["CI-3"] = &model.ConfigItem{ID: "CI-3", Name: "lonely", Kind: model.CIKindDevice, Status: model.CIStatusActive,
			Owners: []model.CIOwner{{UserID: "U-3", Role: "admin"}}}
		d.Services["S-1"] = &model.Service{ID: "S-1", Name: "Payments", OwnerTeamID: "T-2", Criticality: model.CriticalityCritical,
			Status: model.ServiceActive, CIIDs: []string{"CI-1", "CI-2"}}
		d.Services["S-2"] = &model.Service{ID: "S-2", Name: "Reports", OwnerTeamID: "T-3", Criticality: model.CriticalityLow,
			Status: model.ServiceActive, CIIDs: []string{"CI-1"}}
	})
	return st
}

func setup(t *testing.T) (*alert.Engine, *store.Store, *recorder, *clock) {
	t.Helper()
	cfg := storetest.TempDatabase(t)
	db, err := pgxpool.New(context.Background(), cfg.DSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := alert.EnsureSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := alert.EnsureSchema(context.Background(), db); err != nil {
		t.Fatalf("the schema is created idempotently: %v", err)
	}
	st := catalog()
	e := alert.New(db, st)
	rec := &recorder{}
	c := &clock{t: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)}
	e.SetSender(rec)
	e.SetNotifier(rec)
	e.SetClock(c.now)
	return e, st, rec, c
}

func ev(conn, key, ci, signal, sev, status string) alert.Incoming {
	return alert.Incoming{ConnectorID: conn, Key: key, Title: signal + " on " + ci, CI: ci, Signal: signal, Method: "other", Severity: sev, Status: status}
}

func active(t *testing.T, e *alert.Engine) []alert.Alert {
	t.Helper()
	p, err := e.List(context.Background(), alert.Filter{Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	return p.Alerts
}

func TestFoldAcrossSourcesAndResolve(t *testing.T) {
	ctx := context.Background()
	e, _, rec, c := setup(t)

	// Zabbix and Prometheus see the same problem on the same item under different names.
	if err := e.Ingest(ctx, []alert.Incoming{ev("CON-1", "z1", "db-01.example.com", "cpu", "warning", "firing")}); err != nil {
		t.Fatal(err)
	}
	c.advance(time.Minute)
	if err := e.Ingest(ctx, []alert.Incoming{ev("CON-2", "p1", "10.0.0.11:9100", "CPU", "critical", "firing")}); err != nil {
		t.Fatal(err)
	}
	list := active(t, e)
	if len(list) != 1 {
		t.Fatalf("one alert per item and signal, got %d", len(list))
	}
	a := list[0]
	if a.CIID != "CI-1" || a.Severity != "critical" || len(a.Sources) != 2 || a.Count != 2 || a.PD.Key != "umb-"+a.ID {
		t.Errorf("alert = %+v", a)
	}
	cmds := rec.take()
	if len(cmds) != 2 || cmds[0].Action != alert.PDTrigger || cmds[1].Action != alert.PDTrigger || cmds[1].Alert.Severity != "critical" {
		t.Errorf("trigger on open and on severity raise: %+v", cmds)
	}

	// Repeated delivery touches the alert without a new trigger.
	if err := e.Ingest(ctx, []alert.Incoming{ev("CON-2", "p1", "10.0.0.11:9100", "CPU", "critical", "firing")}); err != nil {
		t.Fatal(err)
	}
	if cmds := rec.take(); len(cmds) != 0 {
		t.Errorf("a duplicate sends nothing: %+v", cmds)
	}

	// One source back to normal: still open, severity follows the one that fires.
	if err := e.Ingest(ctx, []alert.Incoming{ev("CON-2", "p1", "10.0.0.11:9100", "CPU", "critical", "resolved")}); err != nil {
		t.Fatal(err)
	}
	if list := active(t, e); len(list) != 1 || list[0].Severity != "warning" {
		t.Fatalf("open while a source fires: %+v", list)
	}
	if err := e.Ingest(ctx, []alert.Incoming{ev("CON-1", "z1", "db-01.example.com", "cpu", "warning", "resolved")}); err != nil {
		t.Fatal(err)
	}
	if list := active(t, e); len(list) != 0 {
		t.Fatalf("resolved when all sources are normal: %+v", list)
	}
	if cmds := rec.take(); len(cmds) != 1 || cmds[0].Action != alert.PDResolve {
		t.Errorf("resolve goes to PagerDuty: %+v", cmds)
	}

	// Firing again within 10 minutes reopens the same alert.
	c.advance(5 * time.Minute)
	if err := e.Ingest(ctx, []alert.Incoming{ev("CON-1", "z1", "db-01.example.com", "cpu", "warning", "firing")}); err != nil {
		t.Fatal(err)
	}
	if list := active(t, e); len(list) != 1 || list[0].ID != a.ID {
		t.Fatalf("reopened in the window: %+v", list)
	}
	e.Ingest(ctx, []alert.Incoming{ev("CON-1", "z1", "db-01.example.com", "cpu", "warning", "resolved")})
	c.advance(11 * time.Minute)
	e.Ingest(ctx, []alert.Incoming{ev("CON-1", "z1", "db-01.example.com", "cpu", "warning", "firing")})
	if list := active(t, e); len(list) != 1 || list[0].ID == a.ID {
		t.Fatalf("a new alert after the window: %+v", list)
	}

	_, entries, err := e.Get(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]int{}
	for _, en := range entries {
		codes[en.Code]++
	}
	if codes["opened"] != 1 || codes["reopened"] != 1 || codes["resolved"] != 2 || codes["severity_raised"] != 1 || codes["routed"] != 1 {
		t.Errorf("timeline = %v", codes)
	}
}

func TestRouting(t *testing.T) {
	ctx := context.Background()
	e, st, _, _ := setup(t)
	e.Ingest(ctx, []alert.Incoming{ev("CON-1", "a", "db-01", "disk", "error", "firing")})
	a := active(t, e)[0]
	r := a.Route
	if r.Via != alert.ViaService || r.Team == nil || r.Team.ID != "T-2" || len(r.Services) != 2 || r.Services[0].ID != "S-1" {
		t.Fatalf("the most critical service decides the team: %+v", r)
	}
	if len(r.People) != 2 || r.People[0].UserID != "U-1" || r.People[0].Role != "lead" || r.People[0].Telegram != "1001" || r.People[1].UserID != "U-2" {
		t.Errorf("lead first, then members, disabled users left out: %+v", r.People)
	}
	if len(r.Owners) != 1 || r.Owners[0].UserID != "U-3" {
		t.Errorf("CI owners are kept: %+v", r.Owners)
	}

	// An item without a service goes to its NetBox owners.
	e.Ingest(ctx, []alert.Incoming{ev("CON-1", "b", "lonely", "power", "critical", "firing")})
	p, _ := e.List(ctx, alert.Filter{Status: "active", CIID: "CI-3"})
	if len(p.Alerts) != 1 || p.Alerts[0].Route.Via != alert.ViaCIOwners || p.Alerts[0].Route.Recipients()[0].UserID != "U-3" {
		t.Errorf("owners route: %+v", p.Alerts)
	}

	// A team with nobody hands the alert to its parent.
	st.Write(func(d *store.Data) {
		d.Services["S-1"].OwnerTeamID = "T-3"
		d.Services["S-2"].Status = model.ServiceRetired
	})
	if err := e.Reroute(ctx); err != nil {
		t.Fatal(err)
	}
	p, _ = e.List(ctx, alert.Filter{Status: "active", TeamID: "T-3"})
	if len(p.Alerts) != 1 || len(p.Alerts[0].Route.People) != 2 || len(p.Alerts[0].Route.Services) != 1 {
		t.Errorf("reroute to the parent's people: %+v", p.Alerts)
	}
	p, _ = e.List(ctx, alert.Filter{Status: "active", ServiceID: "S-1", Query: "payments"})
	if len(p.Alerts) != 1 {
		t.Errorf("filter by service and search by service name: %+v", p.Alerts)
	}
}

func TestUnknownItemIsBoundLater(t *testing.T) {
	ctx := context.Background()
	e, st, _, _ := setup(t)
	e.Ingest(ctx, []alert.Incoming{ev("CON-1", "a", "new-host", "ping", "critical", "firing")})
	a := active(t, e)[0]
	if a.CIID != "" || a.Route.Via != alert.ViaNone {
		t.Fatalf("not in the catalog yet: %+v", a)
	}
	st.Write(func(d *store.Data) {
		d.ConfigItems["CI-9"] = &model.ConfigItem{ID: "CI-9", Name: "new-host", Status: model.CIStatusActive}
		d.Services["S-1"].CIIDs = append(d.Services["S-1"].CIIDs, "CI-9")
	})
	e.Ingest(ctx, []alert.Incoming{ev("CON-1", "a", "new-host", "ping", "critical", "firing")})
	list := active(t, e)
	if len(list) != 1 || list[0].ID != a.ID || list[0].CIID != "CI-9" || list[0].Route.Team == nil {
		t.Fatalf("the same alert is bound to the item: %+v", list)
	}
}

func TestActionsAndPagerDuty(t *testing.T) {
	ctx := context.Background()
	e, _, rec, c := setup(t)
	e.Ingest(ctx, []alert.Incoming{ev("CON-1", "a", "app-01", "errors", "error", "firing")})
	a := active(t, e)[0]
	rec.take()

	if _, err := e.Act(ctx, a.ID, "resolve", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Act(ctx, a.ID, "ack", "eng", ""); !errors.Is(err, alert.ErrNotOpen) {
		t.Errorf("a resolved alert is not acknowledged: %v", err)
	}
	rec.take()

	e.Ingest(ctx, []alert.Incoming{ev("CON-1", "b", "app-01", "latency", "critical", "firing")})
	b := active(t, e)[0]
	rec.take()
	// PagerDuty is down: the trigger fails and is retried; after 2 minutes backup notification.
	e.PDResult(ctx, b.ID, alert.PDTrigger, "default", errors.New("Events API answered 503"))
	got, _, _ := e.Get(ctx, b.ID)
	if got.PD.State != alert.PDFailed || got.PD.Retry != "trigger" {
		t.Fatalf("pd = %+v", got.PD)
	}
	c.advance(30 * time.Second)
	e.Tick(ctx)
	if cmds := rec.take(); len(cmds) != 0 || len(rec.fallback) != 0 {
		t.Errorf("nothing before the retry interval: %+v", cmds)
	}
	c.advance(90 * time.Second)
	e.Tick(ctx)
	if cmds := rec.take(); len(cmds) != 1 || cmds[0].Action != alert.PDTrigger {
		t.Errorf("retry: %+v", cmds)
	}
	if len(rec.fallback) != 1 || rec.fallback[0].ID != b.ID || len(rec.fallback[0].Route.Recipients()) != 2 {
		t.Fatalf("backup notification after 2 minutes: %+v", rec.fallback)
	}
	e.Tick(ctx)
	if len(rec.fallback) != 1 {
		t.Error("backup notification is sent once")
	}

	// Acknowledged in Umbrella while PagerDuty was down: after the trigger is taken, the
	// acknowledgement follows.
	if _, err := e.Act(ctx, b.ID, "ack", "eng", ""); err != nil {
		t.Fatal(err)
	}
	rec.take()
	e.PDResult(ctx, b.ID, alert.PDTrigger, "Payments", nil)
	if cmds := rec.take(); len(cmds) != 1 || cmds[0].Action != alert.PDAcknowledge {
		t.Errorf("the acknowledgement follows: %+v", cmds)
	}
	e.PDResult(ctx, b.ID, alert.PDAcknowledge, "Payments", nil)

	// Resolved in PagerDuty: resolved here.
	keys, err := e.PDKeys(ctx, "umb-"+b.ID, "")
	if err != nil || len(keys) != 1 {
		t.Fatal(keys, err)
	}
	if err := e.PDInbound(ctx, alert.PDUpdate{DedupKey: keys[0], EventType: "incident.resolved", Actor: "Jane", IncidentID: "Q1", IncidentURL: "https://pd/incidents/Q1"}); err != nil {
		t.Fatal(err)
	}
	got, entries, _ := e.Get(ctx, b.ID)
	if got.Status != alert.StatusResolved || got.ResolvedBy != "Jane" || got.PD.IncidentID != "Q1" || got.PD.State != alert.PDAcked {
		t.Errorf("after webhook: %+v", got)
	}
	if keys, _ := e.PDKeys(ctx, "", "Q1"); len(keys) != 1 {
		t.Error("the alert is found by the PagerDuty incident")
	}
	if entries[len(entries)-1].Code != "resolved" || entries[len(entries)-1].Args["why"] != "pagerduty" {
		t.Errorf("timeline end = %+v", entries[len(entries)-1])
	}
	if _, err := e.Act(ctx, b.ID, "comment", "eng", "  "); !errors.Is(err, alert.ErrEmptyComment) {
		t.Error("empty comments are refused")
	}
}

func TestBelowThresholdIsNotRetried(t *testing.T) {
	ctx := context.Background()
	e, _, rec, c := setup(t)
	e.Ingest(ctx, []alert.Incoming{ev("CON-1", "a", "app-01", "errors", "warning", "firing")})
	a := active(t, e)[0]
	e.PDResult(ctx, a.ID, alert.PDTrigger, "", alert.ErrPDSkipped)
	rec.take()
	c.advance(5 * time.Minute)
	e.Tick(ctx)
	if cmds := rec.take(); len(cmds) != 0 || len(rec.fallback) != 0 {
		t.Errorf("skipped alerts are left alone: %+v %+v", cmds, rec.fallback)
	}
}

func TestMaintenance(t *testing.T) {
	ctx := context.Background()
	e, st, rec, c := setup(t)
	st.Write(func(d *store.Data) {
		d.Maintenance["MW-1"] = &model.Maintenance{ID: "MW-1", Title: "Patching", ServiceIDs: []string{"S-1"},
			Start: c.t.Add(-time.Minute), End: c.t.Add(30 * time.Minute)}
	})
	e.Ingest(ctx, []alert.Incoming{ev("CON-1", "a", "app-01", "errors", "critical", "firing")})
	a := active(t, e)[0]
	if !a.Suppressed || a.MaintenanceID != "MW-1" || a.PD.State != alert.PDSkipped {
		t.Fatalf("suppressed: %+v", a)
	}
	c.advance(5 * time.Minute)
	e.Tick(ctx)
	if cmds := rec.take(); len(cmds) != 0 || len(rec.fallback) != 0 {
		t.Errorf("nothing is sent in a window: %+v", cmds)
	}
	c.advance(30 * time.Minute)
	e.Tick(ctx)
	if cmds := rec.take(); len(cmds) != 1 || cmds[0].Action != alert.PDTrigger {
		t.Errorf("sent when the window is over: %+v", cmds)
	}
	got, _, _ := e.Get(ctx, a.ID)
	if got.Suppressed {
		t.Error("no longer suppressed")
	}
}

func TestRedUseLink(t *testing.T) {
	ctx := context.Background()
	e, _, _, c := setup(t)
	use := ev("CON-1", "u", "db-01", "use.cpu", "warning", "firing")
	use.Method = "use"
	red := ev("CON-2", "r", "app-01", "red.errors", "error", "firing")
	red.Method = "red"
	e.Ingest(ctx, []alert.Incoming{use})
	c.advance(time.Minute)
	e.Ingest(ctx, []alert.Incoming{red})
	p, _ := e.List(ctx, alert.Filter{Status: "active", Method: "red"})
	if len(p.Alerts) != 1 || p.Alerts[0].RelatedID == "" {
		t.Fatalf("red linked to use: %+v", p.Alerts)
	}
	if p.Counts.Active != 2 || p.Counts.BySeverity["error"] != 1 || p.Counts.PDNotTaken != 2 {
		t.Errorf("counts = %+v", p.Counts)
	}
}
