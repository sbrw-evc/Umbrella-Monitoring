package model

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Impact levels: the criticality of the most critical affected service, or none.
const ImpactNone = "none"

// ImpactLevels are the columns of the impact matrix, most critical first: the service
// criticalities and «no impact».
var ImpactLevels = append(slices.Clone(Criticalities), ImpactNone)

// ValidImpactLevel reports whether v is a column of the impact matrix.
func ValidImpactLevel(v string) bool { return slices.Contains(ImpactLevels, v) }

// Actions of a classification rule.
const (
	RuleActionSet    = "set"
	RuleActionRaise  = "raise"
	RuleActionLower  = "lower"
	RuleActionImpact = "impact"
)

var RuleActions = []string{RuleActionSet, RuleActionRaise, RuleActionLower, RuleActionImpact}

const (
	MaxImpactRules    = 100
	MaxImpactRuleWhen = 2000
	MaxImpactRuleName = 200
	MaxImpactRuleID   = 64
	MaxUpstreamDepth  = 10
)

// ImpactMatrix maps an event severity and an impact level to the priority:
// matrix[event severity][impact level] = severity.
type ImpactMatrix map[string]map[string]string

// Get is the priority of a cell; the event severity itself when the cell is missing.
func (m ImpactMatrix) Get(event, level string) string {
	if v := m[event][level]; ValidSeverity(v) {
		return v
	}
	return event
}

// ImpactPolicy finds the priority of an incident from the severity of its events and the
// business impact (the criticality of the affected services), then the classification rules.
//
// Configured is false until the policy is saved once: such a policy (the settings of an
// installation older than the policy, or one that never opened the page) is the built-in
// template (DefaultImpactPolicy), which is enabled. Effective gives the policy that applies.
type ImpactPolicy struct {
	Configured bool `json:"configured"`
	// Enabled: the priority is computed; otherwise it is the event severity and the impact is
	// only shown.
	Enabled bool `json:"enabled"`
	// UpstreamDepth: how many steps of depends_on count from the services of the item to the
	// services that depend on them; 0 counts only the services of the item.
	UpstreamDepth int                  `json:"upstream_depth"`
	Matrix        ImpactMatrix         `json:"matrix"`
	Rules         []ClassificationRule `json:"rules"`
}

// ClassificationRule changes the priority of the incidents its condition (When, CEL) matches.
// Action set takes Severity, raise and lower take N levels, impact takes Level. Stop: the
// rules after this one are not looked at when it matches.
type ClassificationRule struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	When     string `json:"when"`
	Action   string `json:"action"`
	Severity string `json:"severity,omitempty"`
	N        int    `json:"n,omitempty"`
	Level    string `json:"level,omitempty"`
	Stop     bool   `json:"stop,omitempty"`
}

//go:embed impact_default.json
var impactDefaultJSON []byte

// DefaultImpactPolicy is the built-in template: enabled, upstream depth 3, the matrix of
// impact_default.json and an example rule that is off.
func DefaultImpactPolicy() ImpactPolicy {
	var p ImpactPolicy
	if err := json.Unmarshal(impactDefaultJSON, &p); err != nil {
		panic("model: impact_default.json: " + err.Error())
	}
	return p
}

// Effective is the policy that applies: the template while the policy was never saved.
func (p ImpactPolicy) Effective() ImpactPolicy {
	if !p.Configured {
		return DefaultImpactPolicy()
	}
	return p
}

// Clone copies the matrix and the rules.
func (p ImpactPolicy) Clone() ImpactPolicy {
	c := p
	if p.Matrix != nil {
		c.Matrix = make(ImpactMatrix, len(p.Matrix))
		for k, row := range p.Matrix {
			c.Matrix[k] = maps.Clone(row)
		}
	}
	c.Rules = slices.Clone(p.Rules)
	return c
}

var ruleIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// Normalize checks a policy to be saved and returns it marked configured: missing matrix cells
// are taken from the template, rule fields an action does not use are dropped. The CEL
// conditions are compiled by the caller (alert.CompileImpact).
func (p ImpactPolicy) Normalize() (ImpactPolicy, error) {
	p = p.Clone()
	p.Configured = true
	if p.UpstreamDepth < 0 || p.UpstreamDepth > MaxUpstreamDepth {
		return p, fmt.Errorf("the upstream depth must be between 0 and %d", MaxUpstreamDepth)
	}
	def := DefaultImpactPolicy().Matrix
	for ev := range p.Matrix {
		if !ValidSeverity(ev) {
			return p, fmt.Errorf("unknown event severity %q in the matrix", ev)
		}
		for lvl, sev := range p.Matrix[ev] {
			if !ValidImpactLevel(lvl) {
				return p, fmt.Errorf("unknown impact level %q in the matrix", lvl)
			}
			if !ValidSeverity(sev) {
				return p, fmt.Errorf("unknown severity %q in the matrix (%s × %s)", sev, ev, lvl)
			}
		}
	}
	m := ImpactMatrix{}
	for _, ev := range SeverityNames() {
		m[ev] = map[string]string{}
		for _, lvl := range ImpactLevels {
			if v, ok := p.Matrix[ev][lvl]; ok {
				m[ev][lvl] = v
			} else {
				m[ev][lvl] = def[ev][lvl]
			}
		}
	}
	p.Matrix = m
	if len(p.Rules) > MaxImpactRules {
		return p, fmt.Errorf("at most %d rules", MaxImpactRules)
	}
	if p.Rules == nil {
		p.Rules = []ClassificationRule{}
	}
	seen := map[string]bool{}
	for i := range p.Rules {
		r := &p.Rules[i]
		r.ID, r.Name, r.When = strings.TrimSpace(r.ID), strings.TrimSpace(r.Name), strings.TrimSpace(r.When)
		label := fmt.Sprintf("rule %d", i+1)
		if r.Name != "" {
			label = fmt.Sprintf("rule %q", r.Name)
		}
		switch {
		case r.ID == "" || len(r.ID) > MaxImpactRuleID || !ruleIDPattern.MatchString(r.ID):
			return p, fmt.Errorf("%s: the ID must be 1 to %d letters, digits, dots, dashes or underscores", label, MaxImpactRuleID)
		case seen[r.ID]:
			return p, fmt.Errorf("%s: the ID %q is used twice", label, r.ID)
		case r.Name == "":
			return p, fmt.Errorf("rule %s: the name is empty", r.ID)
		case utf8.RuneCountInString(r.Name) > MaxImpactRuleName:
			return p, fmt.Errorf("%s: the name is longer than %d characters", label, MaxImpactRuleName)
		case r.When == "":
			return p, fmt.Errorf("%s: the condition is empty", label)
		case utf8.RuneCountInString(r.When) > MaxImpactRuleWhen:
			return p, fmt.Errorf("%s: the condition is longer than %d characters", label, MaxImpactRuleWhen)
		}
		seen[r.ID] = true
		switch r.Action {
		case RuleActionSet:
			if !ValidSeverity(r.Severity) {
				return p, fmt.Errorf("%s: unknown severity %q", label, r.Severity)
			}
			r.N, r.Level = 0, ""
		case RuleActionRaise, RuleActionLower:
			if r.N < 1 || r.N > len(Severities)-1 {
				return p, fmt.Errorf("%s: the number of levels must be between 1 and %d", label, len(Severities)-1)
			}
			r.Severity, r.Level = "", ""
		case RuleActionImpact:
			if !ValidImpactLevel(r.Level) {
				return p, fmt.Errorf("%s: unknown impact level %q", label, r.Level)
			}
			r.Severity, r.N = "", 0
		default:
			return p, fmt.Errorf("%s: unknown action %q", label, r.Action)
		}
	}
	return p, nil
}

// ShiftSeverity moves a severity n levels up (n > 0, towards P1) or down (n < 0), within P1..P5.
func ShiftSeverity(name string, n int) string {
	i := slices.IndexFunc(Severities, func(s Severity) bool { return s.Name == name })
	if i < 0 {
		return name
	}
	i = min(max(i-n, 0), len(Severities)-1)
	return Severities[i].Name
}
