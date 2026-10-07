package alert

import (
	"strings"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// impactCatalog: db-01 runs Payments (high); Checkout (critical) depends on Payments, Portal
// (low) on Checkout, Payments on Portal again (a cycle); Archive is retired.
func impactCatalog(p model.ImpactPolicy) *store.Store {
	st := store.New()
	st.Write(func(d *store.Data) {
		d.ConfigItems["CI-1"] = &model.ConfigItem{ID: "CI-1", Name: "db-01", Kind: model.CIKindVM}
		d.ConfigItems["CI-2"] = &model.ConfigItem{ID: "CI-2", Name: "spare", Kind: model.CIKindVM}
		d.Teams["T-1"] = &model.Team{ID: "T-1", Name: "DBA"}
		d.Services["S-1"] = &model.Service{ID: "S-1", Name: "Payments", Criticality: model.CriticalityHigh, Status: model.ServiceActive,
			OwnerTeamID: "T-1", CIIDs: []string{"CI-1"}, DependsOn: []string{"S-3"}}
		d.Services["S-2"] = &model.Service{ID: "S-2", Name: "Checkout", Criticality: model.CriticalityCritical, Status: model.ServiceActive,
			DependsOn: []string{"S-1"}}
		d.Services["S-3"] = &model.Service{ID: "S-3", Name: "Portal", Criticality: model.CriticalityLow, Status: model.ServiceActive,
			DependsOn: []string{"S-2", "S-3"}}
		d.Services["S-4"] = &model.Service{ID: "S-4", Name: "Archive", Criticality: model.CriticalityCritical, Status: model.ServiceRetired,
			DependsOn: []string{"S-1"}}
		d.Settings.Impact = p
	})
	return st
}

func policyWith(f func(p *model.ImpactPolicy)) model.ImpactPolicy {
	p := model.DefaultImpactPolicy()
	p.Configured = true
	f(&p)
	return p
}

func eval(t *testing.T, st *store.Store, ci, sev string, labels map[string]string) (string, *Impact) {
	t.Helper()
	p := PreviewImpact(st, ImpactEvent{Title: "disk full on " + ci, CI: ci, Signal: "disk", Severity: sev, Labels: labels}, nil, time.Now())
	return p.Severity, p.Impact
}

func stepCodes(im *Impact) string {
	var out []string
	for _, s := range im.Steps {
		out = append(out, s.Code)
	}
	return strings.Join(out, ",")
}

func TestImpactTemplate(t *testing.T) {
	m := model.DefaultImpactPolicy().Matrix
	want := map[string][5]string{ // critical, high, medium, low, none
		"critical": {"critical", "critical", "critical", "critical", "critical"},
		"error":    {"error", "error", "error", "error", "warning"},
		"warning":  {"error", "warning", "warning", "warning", "low"},
		"low":      {"warning", "warning", "low", "low", "info"},
		"info":     {"info", "info", "info", "info", "info"},
	}
	for ev, row := range want {
		for i, lvl := range model.ImpactLevels {
			if got := m.Get(ev, lvl); got != row[i] {
				t.Errorf("template %s × %s = %s, want %s", ev, lvl, got, row[i])
			}
		}
	}
	if p := (model.ImpactPolicy{}).Effective(); !p.Enabled || p.UpstreamDepth != 3 || p.Configured {
		t.Errorf("an empty policy is the enabled template: %+v", p)
	}
}

func TestUpstreamServices(t *testing.T) {
	st := impactCatalog(policyWith(func(p *model.ImpactPolicy) {}))
	sev, im := eval(t, st, "db-01", "warning", nil)
	// Payments (high, direct), Checkout (critical, depends on Payments), Portal (low, depends on
	// Checkout); the cycle back to Payments and the retired Archive do not count.
	if im.Level != "critical" || len(im.Services) != 3 || im.Services[0].Name != "Checkout" || im.Services[0].Direct ||
		im.Services[1].Name != "Payments" || !im.Services[1].Direct || im.Services[2].Name != "Portal" {
		t.Fatalf("impact = %+v", im)
	}
	if sev != "error" || stepCodes(im) != "event_severity,impact_level,matrix" {
		t.Errorf("P3 × critical = %s (%s)", sev, stepCodes(im))
	}
	if im.Steps[1].Args["services"] != "Payments" || im.Steps[1].Args["upstream"] != "Checkout, Portal" {
		t.Errorf("impact step = %+v", im.Steps[1])
	}

	// Depth 0: only the services of the item.
	st = impactCatalog(policyWith(func(p *model.ImpactPolicy) { p.UpstreamDepth = 0 }))
	sev, im = eval(t, st, "db-01", "warning", nil)
	if im.Level != "high" || len(im.Services) != 1 || sev != "warning" {
		t.Errorf("depth 0: %s %+v", sev, im)
	}
	// Depth 1 reaches Checkout but not Portal.
	st = impactCatalog(policyWith(func(p *model.ImpactPolicy) { p.UpstreamDepth = 1 }))
	if _, im = eval(t, st, "db-01", "warning", nil); len(im.Services) != 2 {
		t.Errorf("depth 1: %+v", im.Services)
	}

	// An item of no service: no impact, the template lowers the priority by one.
	sev, im = eval(t, st, "spare", "error", nil)
	if im.Level != model.ImpactNone || sev != "warning" {
		t.Errorf("no impact: %s %+v", sev, im)
	}
	// An item not in the catalog: the impact is unknown and the event severity stays.
	sev, im = eval(t, st, "nowhere", "error", nil)
	if im.Level != "" || sev != "error" || stepCodes(im) != "event_severity,impact_unknown" {
		t.Errorf("unknown item: %s %+v", sev, im)
	}
	// A catalog without business services: the impact is unknown as well.
	st.Write(func(d *store.Data) { clear(d.Services) })
	if sev, im = eval(t, st, "spare", "error", nil); sev != "error" || im.Steps[1].Args["reason"] != "no_services" {
		t.Errorf("no services at all: %s %+v", sev, im)
	}
}

func TestImpactDisabled(t *testing.T) {
	st := impactCatalog(policyWith(func(p *model.ImpactPolicy) {
		p.Enabled = false
		p.Rules = []model.ClassificationRule{{ID: "r", Name: "r", Enabled: true, When: "true", Action: "set", Severity: "critical"}}
	}))
	sev, im := eval(t, st, "db-01", "low", nil)
	if sev != "low" || im.Level != "critical" || len(im.Services) != 3 || stepCodes(im) != "event_severity,impact_level,policy_disabled" {
		t.Errorf("disabled: %s %+v", sev, im)
	}
}

func TestClassificationRules(t *testing.T) {
	rule := func(id, when, action string, f func(r *model.ClassificationRule)) model.ClassificationRule {
		r := model.ClassificationRule{ID: id, Name: "Rule " + id, Enabled: true, When: when, Action: action}
		if f != nil {
			f(&r)
		}
		return r
	}
	cases := []struct {
		name   string
		rules  []model.ClassificationRule
		ci     string
		sev    string
		labels map[string]string
		want   string
		level  string
		rule   string
		steps  string
	}{
		{"set", []model.ClassificationRule{rule("a", "'env' in labels && labels['env'] == 'prod'", "set", func(r *model.ClassificationRule) { r.Severity = "critical" })},
			"db-01", "info", map[string]string{"env": "prod"}, "critical", "critical", "Rule a", "event_severity,impact_level,matrix,rule_set"},
		{"no match", []model.ClassificationRule{rule("a", "'env' in labels", "set", func(r *model.ClassificationRule) { r.Severity = "critical" })},
			"db-01", "info", nil, "info", "critical", "", "event_severity,impact_level,matrix"},
		{"raise and lower in order", []model.ClassificationRule{
			rule("a", "'Checkout' in services", "raise", func(r *model.ClassificationRule) { r.N = 4 }),
			rule("b", "severity == 'critical' && team == 'DBA'", "lower", func(r *model.ClassificationRule) { r.N = 2 }),
		}, "db-01", "low", nil, "warning", "critical", "Rule b", "event_severity,impact_level,matrix,rule_raise,rule_lower"},
		{"lower stays at P5", []model.ClassificationRule{rule("a", "true", "lower", func(r *model.ClassificationRule) { r.N = 4 })},
			"db-01", "warning", nil, "info", "critical", "Rule a", "event_severity,impact_level,matrix,rule_lower"},
		{"stop", []model.ClassificationRule{
			rule("a", "ci == 'db-01'", "set", func(r *model.ClassificationRule) { r.Severity = "low"; r.Stop = true }),
			rule("b", "true", "set", func(r *model.ClassificationRule) { r.Severity = "critical" }),
		}, "db-01", "error", nil, "low", "critical", "Rule a", "event_severity,impact_level,matrix,rule_set"},
		{"disabled rule", []model.ClassificationRule{rule("a", "true", "set", func(r *model.ClassificationRule) { r.Severity = "critical"; r.Enabled = false })},
			"spare", "error", nil, "warning", "none", "", "event_severity,impact_level,matrix"},
		{"impact level", []model.ClassificationRule{
			rule("a", "impact == 'none' && ci_kind == 'vm' && signal == 'disk'", "impact", func(r *model.ClassificationRule) { r.Level = "critical" }),
			rule("b", "impact == 'critical' && event_severity == 'warning'", "raise", func(r *model.ClassificationRule) { r.N = 1 }),
		}, "spare", "warning", nil, "critical", "critical", "Rule b", "event_severity,impact_level,matrix,rule_impact,rule_raise"},
		{"impact of an unknown item", []model.ClassificationRule{
			rule("a", "impact == 'unknown' && title.startsWith('disk')", "impact", func(r *model.ClassificationRule) { r.Level = "high" }),
		}, "nowhere", "low", nil, "warning", "high", "Rule a", "event_severity,impact_unknown,rule_impact"},
		{"evaluation error", []model.ClassificationRule{
			rule("a", "labels['missing'] == 'x'", "set", func(r *model.ClassificationRule) { r.Severity = "critical" }),
			rule("b", "size(direct_services) == 1 && service_criticalities[0] == 'critical'", "lower", func(r *model.ClassificationRule) { r.N = 1 }),
		}, "db-01", "error", nil, "warning", "critical", "Rule b", "event_severity,impact_level,matrix,rule_error,rule_lower"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := impactCatalog(policyWith(func(p *model.ImpactPolicy) { p.Rules = tc.rules }))
			sev, im := eval(t, st, tc.ci, tc.sev, tc.labels)
			if sev != tc.want || firstNonEmpty(im.Level, ImpactUnknown) != firstNonEmpty(tc.level, ImpactUnknown) || im.Rule != tc.rule || stepCodes(im) != tc.steps {
				t.Errorf("got %s level %q rule %q steps %s; want %s %q %q %s", sev, im.Level, im.Rule, stepCodes(im), tc.want, tc.level, tc.rule, tc.steps)
			}
		})
	}
}

func TestCompileImpact(t *testing.T) {
	for when, ok := range map[string]bool{
		"severity == 'critical'":                            true,
		"'Payments' in services && labels['env'] == 'prod'": true,
		"title.lowerAscii().contains('db')":                 true,
		// The examples of docs/impact.md.
		"'Payments' in direct_services && severity in ['warning', 'low', 'info']": true,
		"ci.startsWith('test-') || ('env' in labels && labels['env'] == 'test')":  true,
		"ci_kind == 'device' && ci.matches('^core-')":                             true,
		"method == 'red' && impact in ['critical', 'high']":                       true,
		"severity ==":        false,
		"unknown_var == 1":   false,
		"severity":           false, // not a bool
		"size(services) + 1": false,
	} {
		err := CompileImpact(model.ImpactPolicy{Rules: []model.ClassificationRule{{ID: "x", Name: "X", When: when}}})
		if (err == nil) != ok {
			t.Errorf("CompileImpact(%q) = %v", when, err)
		}
	}
	// The template compiles, its example rule too.
	if err := CompileImpact(model.DefaultImpactPolicy()); err != nil {
		t.Error(err)
	}
	// A heavy condition is stopped by the cost limit.
	st := impactCatalog(policyWith(func(p *model.ImpactPolicy) {
		p.Rules = []model.ClassificationRule{{ID: "x", Name: "X", Enabled: true, Action: "set", Severity: "critical",
			When: "[1,2,3,4,5,6,7,8,9,10].all(a, [1,2,3,4,5,6,7,8,9,10].all(b, [1,2,3,4,5,6,7,8,9,10].all(c, [1,2,3,4,5,6,7,8,9,10].all(d, [1,2,3,4,5,6,7,8,9,10].all(e, title.size() > 0)))))"}}
	}))
	if sev, im := eval(t, st, "db-01", "info", nil); sev != "info" || !strings.Contains(stepCodes(im), "rule_error") {
		t.Errorf("cost limit: %s %s", sev, stepCodes(im))
	}
}
