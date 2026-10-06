package alert_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
)

// The preview in the CI card must be exactly the route the engine gives an incident.
func TestPreviewEqualsEngineRoute(t *testing.T) {
	ctx := context.Background()
	e, st, _, c := setup(t)
	for _, ci := range []string{"db-01.example.com", "app-01", "lonely"} {
		if err := e.Ingest(ctx, []alert.Incoming{ev("CON-1", ci, ci, "cpu", "critical", "firing")}); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range active(t, e) {
		p, ok := alert.PreviewCI(st, a.CIID, c.now())
		if !ok {
			t.Fatalf("no preview for %s", a.CIID)
		}
		p.At = a.Route.At
		if !reflect.DeepEqual(p, a.Route) {
			t.Errorf("%s: preview %+v != engine %+v", a.CIName, p, a.Route)
		}
	}
	if _, ok := alert.PreviewCI(st, "CI-404", c.now()); ok {
		t.Error("unknown item")
	}

	// db-01 is in Payments (critical) and Reports (low): the primary service is Payments.
	p, _ := alert.PreviewCI(st, "CI-1", c.now())
	if p.Service == nil || p.Service.ID != "S-1" || p.Team == nil || p.Team.ID != "T-2" || len(p.Services) != 2 {
		t.Fatalf("primary = %+v", p)
	}

	// Reports alone goes to its empty team's parent; its item db-01 goes by Payments instead.
	r, elsewhere, ok := alert.PreviewService(st, "S-2", c.now())
	if !ok || r.Team == nil || r.Team.ID != "T-3" || len(r.People) != 2 || r.Via != alert.ViaService {
		t.Fatalf("service preview = %+v", r)
	}
	if len(elsewhere) != 1 || elsewhere[0].CI.ID != "CI-1" || elsewhere[0].Service.ID != "S-1" || elsewhere[0].Team.ID != "T-2" {
		t.Fatalf("elsewhere = %+v", elsewhere)
	}
	if _, elsewhere, _ := alert.PreviewService(st, "S-1", c.now()); len(elsewhere) != 0 {
		t.Fatalf("Payments routes its own items: %+v", elsewhere)
	}
}
