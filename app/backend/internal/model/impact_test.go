package model

import (
	"strings"
	"testing"
)

func TestImpactPolicyNormalize(t *testing.T) {
	// A partial matrix is completed from the template and the policy is marked configured.
	p, err := ImpactPolicy{Enabled: true, Matrix: ImpactMatrix{"warning": {"none": "warning"}},
		Rules: []ClassificationRule{{ID: "a", Name: " A ", When: "true", Action: "raise", N: 2, Severity: "critical", Level: "high"}}}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if !p.Configured || p.Matrix.Get("warning", "none") != "warning" || p.Matrix.Get("warning", "critical") != "error" || len(p.Matrix) != 5 {
		t.Errorf("matrix = %+v", p.Matrix)
	}
	if r := p.Rules[0]; r.Name != "A" || r.Severity != "" || r.Level != "" || r.N != 2 {
		t.Errorf("rule = %+v", r)
	}
	if d, err := DefaultImpactPolicy().Normalize(); err != nil || !d.Configured {
		t.Errorf("the template is valid: %v", err)
	}

	rule := ClassificationRule{ID: "a", Name: "A", When: "true", Action: "set", Severity: "error"}
	bad := map[string]ImpactPolicy{
		"upstream depth":   {UpstreamDepth: 11},
		"depth must be":    {UpstreamDepth: -1},
		"event severity":   {Matrix: ImpactMatrix{"bogus": {}}},
		"impact level":     {Matrix: ImpactMatrix{"error": {"huge": "error"}}},
		"severity":         {Matrix: ImpactMatrix{"error": {"high": "P1"}}},
		"used twice":       {Rules: []ClassificationRule{rule, rule}},
		"ID":               {Rules: []ClassificationRule{{ID: "a b", Name: "A", When: "true", Action: "set", Severity: "error"}}},
		"name is empty":    {Rules: []ClassificationRule{{ID: "a", When: "true", Action: "set", Severity: "error"}}},
		"condition":        {Rules: []ClassificationRule{{ID: "a", Name: "A", Action: "set", Severity: "error"}}},
		"longer":           {Rules: []ClassificationRule{{ID: "a", Name: "A", When: strings.Repeat("x", MaxImpactRuleWhen+1), Action: "set", Severity: "error"}}},
		"unknown action":   {Rules: []ClassificationRule{{ID: "a", Name: "A", When: "true", Action: "drop"}}},
		"unknown severity": {Rules: []ClassificationRule{{ID: "a", Name: "A", When: "true", Action: "set"}}},
		"number of levels": {Rules: []ClassificationRule{{ID: "a", Name: "A", When: "true", Action: "lower", N: 5}}},
		"unknown impact":   {Rules: []ClassificationRule{{ID: "a", Name: "A", When: "true", Action: "impact", Level: "unknown"}}},
		"at most":          {Rules: make([]ClassificationRule, MaxImpactRules+1)},
	}
	for want, p := range bad {
		if _, err := p.Normalize(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", want, err)
		}
	}
}

func TestShiftSeverity(t *testing.T) {
	for _, c := range []struct {
		from string
		n    int
		want string
	}{{"warning", 1, "error"}, {"warning", -1, "low"}, {"error", 4, "critical"}, {"low", -3, "info"}, {"bogus", 1, "bogus"}} {
		if got := ShiftSeverity(c.from, c.n); got != c.want {
			t.Errorf("ShiftSeverity(%s, %d) = %s, want %s", c.from, c.n, got, c.want)
		}
	}
}
