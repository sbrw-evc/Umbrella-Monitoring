package alert

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/ext"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// Priority: the severity of an alert is its priority, found from the severity of its events
// (EventSeverity) and the business impact (Impact) by the impact policy of the settings
// (model.ImpactPolicy): the impact matrix, then the classification rules.

const (
	// impactCostLimit bounds the work of one rule condition, so a rule cannot stall the engine.
	impactCostLimit = 100_000
	// impactTimeout bounds all rule conditions of one evaluation.
	impactTimeout = 250 * time.Millisecond
	// ImpactUnknown is the impact the rules see when it is not known: the item is not in the
	// catalog, or the catalog has no business services at all.
	ImpactUnknown = "unknown"
)

// Steps of Impact.Steps.
const (
	StepEventSeverity  = "event_severity"
	StepImpactLevel    = "impact_level"
	StepImpactUnknown  = "impact_unknown"
	StepPolicyDisabled = "policy_disabled"
	StepMatrix         = "matrix"
	StepRuleSet        = "rule_set"
	StepRuleRaise      = "rule_raise"
	StepRuleLower      = "rule_lower"
	StepRuleImpact     = "rule_impact"
	StepRuleError      = "rule_error"
)

var (
	impactEnvOnce sync.Once
	impactEnv     *cel.Env
	impactEnvErr  error
)

// ImpactVariables are the variables a rule condition sees.
var ImpactVariables = []string{"event_severity", "severity", "impact", "title", "ci", "ci_kind", "signal", "method", "labels",
	"services", "service_criticalities", "direct_services", "team"}

func impactCEL() (*cel.Env, error) {
	impactEnvOnce.Do(func() {
		opts := []cel.EnvOption{ext.Strings(), cel.OptionalTypes()}
		for _, v := range ImpactVariables {
			t := cel.StringType
			switch v {
			case "labels":
				t = cel.MapType(cel.StringType, cel.StringType)
			case "services", "service_criticalities", "direct_services":
				t = cel.ListType(cel.StringType)
			}
			opts = append(opts, cel.Variable(v, t))
		}
		impactEnv, impactEnvErr = cel.NewEnv(opts...)
	})
	return impactEnv, impactEnvErr
}

// compileCondition compiles the condition of a rule; it must return a bool.
func compileCondition(src string) (cel.Program, error) {
	env, err := impactCEL()
	if err != nil {
		return nil, err
	}
	ast, iss := env.Compile(src)
	if iss != nil && iss.Err() != nil {
		return nil, errors.New(strings.TrimSpace(iss.Err().Error()))
	}
	if t := ast.OutputType(); t != cel.BoolType && t != cel.DynType {
		return nil, fmt.Errorf("the condition returns %s, not bool", t)
	}
	return env.Program(ast, cel.CostLimit(impactCostLimit), cel.InterruptCheckFrequency(64))
}

type compiledRule struct {
	model.ClassificationRule
	prg cel.Program
	err error
}

// impactPolicy is an effective impact policy with its rule conditions compiled.
type impactPolicy struct {
	model.ImpactPolicy
	rules []compiledRule
}

// The compiled policy is kept for the settings it was made of, so the rules compile once per
// change of the policy, not per catalog change.
var impactCache struct {
	sync.Mutex
	key string
	p   *impactPolicy
}

func compilePolicy(p model.ImpactPolicy) *impactPolicy {
	p = p.Effective()
	key, _ := json.Marshal(p)
	impactCache.Lock()
	defer impactCache.Unlock()
	if impactCache.p != nil && impactCache.key == string(key) {
		return impactCache.p
	}
	out := &impactPolicy{ImpactPolicy: p.Clone()}
	for _, r := range p.Rules {
		if !r.Enabled {
			continue
		}
		prg, err := compileCondition(r.When)
		out.rules = append(out.rules, compiledRule{ClassificationRule: r, prg: prg, err: err})
	}
	impactCache.key, impactCache.p = string(key), out
	return out
}

// CompileImpact checks the rule conditions of a policy to be saved: the error names the first
// rule (enabled or not) whose condition does not compile.
func CompileImpact(p model.ImpactPolicy) error {
	for _, r := range p.Rules {
		if _, err := compileCondition(r.When); err != nil {
			return fmt.Errorf("rule %q: %w", firstNonEmpty(r.Name, r.ID), err)
		}
	}
	return nil
}

// upstream lists the services an incident of the item affects: the active and planned ones it
// runs (direct), then those that depend on them through depends_on up to depth steps, most
// critical first. Dependency cycles are safe: a service is visited once.
func (w *world) affected(ciID string, depth int) []ImpactService {
	if ciID == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []ImpactService
	var frontier []string
	for _, s := range w.servicesOf(ciID) {
		seen[s.ID] = true
		frontier = append(frontier, s.ID)
		out = append(out, ImpactService{ID: s.ID, Name: s.Name, Criticality: s.Criticality, Direct: true})
	}
	for d := 0; d < depth && len(frontier) > 0; d++ {
		var next []string
		for _, id := range frontier {
			for _, s := range w.dependents[id] {
				if seen[s.ID] {
					continue
				}
				seen[s.ID] = true
				next = append(next, s.ID)
				out = append(out, ImpactService{ID: s.ID, Name: s.Name, Criticality: s.Criticality})
			}
		}
		frontier = next
	}
	slices.SortStableFunc(out, func(a, b ImpactService) int {
		if c := cmp.Compare(model.CriticalityRank(b.Criticality), model.CriticalityRank(a.Criticality)); c != 0 {
			return c
		}
		if a.Direct != b.Direct {
			if a.Direct {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out
}

// indexDependents makes the reverse of depends_on: the services that depend on each service.
// Retired services neither affect nor are affected.
func (w *world) indexDependents() {
	w.dependents = map[string][]model.Service{}
	for _, s := range w.services {
		if s.Status == model.ServiceRetired {
			continue
		}
		w.liveServices++
		for _, dep := range s.DependsOn {
			if dep != s.ID {
				w.dependents[dep] = append(w.dependents[dep], s)
			}
		}
	}
}

// evaluate finds the priority of an alert from its event severity (EventSeverity, or Severity
// for an alert stored before priorities) and the business impact of its item.
func (w *world) evaluate(a *Alert, now time.Time) (string, *Impact) {
	p := w.impact
	event := firstNonEmpty(a.EventSeverity, a.Severity)
	im := &Impact{At: now, Services: []ImpactService{}}
	step := func(code string, args map[string]string) {
		im.Steps = append(im.Steps, ImpactStep{Code: code, Args: args})
	}
	step(StepEventSeverity, map[string]string{"severity": event})

	level := ImpactUnknown
	switch {
	case a.CIID == "":
		step(StepImpactUnknown, map[string]string{"reason": "ci_unknown"})
	case w.liveServices == 0:
		step(StepImpactUnknown, map[string]string{"reason": "no_services"})
	default:
		im.Services = w.affected(a.CIID, p.UpstreamDepth)
		level = model.ImpactNone
		if len(im.Services) > 0 && model.ValidCriticality(im.Services[0].Criticality) {
			level = im.Services[0].Criticality
		}
		args := map[string]string{"level": level}
		var direct, upstream []string
		for _, s := range im.Services {
			if s.Direct {
				direct = append(direct, s.Name)
			} else {
				upstream = append(upstream, s.Name)
			}
		}
		if len(direct) > 0 {
			args["services"] = strings.Join(direct, ", ")
		}
		if len(upstream) > 0 {
			args["upstream"] = strings.Join(upstream, ", ")
		}
		step(StepImpactLevel, args)
	}
	setLevel := func() {
		im.Level = level
		if level == ImpactUnknown {
			im.Level = ""
		}
	}
	setLevel()
	if !p.Enabled {
		step(StepPolicyDisabled, map[string]string{"severity": event})
		return event, im
	}
	sev := event
	if level != ImpactUnknown {
		sev = p.Matrix.Get(event, level)
		step(StepMatrix, map[string]string{"event": event, "level": level, "severity": sev})
	}
	if len(p.rules) == 0 {
		return sev, im
	}
	vars := impactVars(a, im, event, level)
	ctx, cancel := context.WithTimeout(context.Background(), impactTimeout)
	defer cancel()
	for _, r := range p.rules {
		name := firstNonEmpty(r.Name, r.ID)
		vars["severity"], vars["impact"] = sev, level
		ok, err := r.match(ctx, vars)
		if err != nil {
			step(StepRuleError, map[string]string{"rule": name, "error": err.Error()})
			continue
		}
		if !ok {
			continue
		}
		from := sev
		switch r.Action {
		case model.RuleActionSet:
			sev = r.Severity
			step(StepRuleSet, map[string]string{"rule": name, "from": from, "severity": sev})
		case model.RuleActionRaise:
			sev = model.ShiftSeverity(sev, r.N)
			step(StepRuleRaise, map[string]string{"rule": name, "n": fmt.Sprint(r.N), "from": from, "severity": sev})
		case model.RuleActionLower:
			sev = model.ShiftSeverity(sev, -r.N)
			step(StepRuleLower, map[string]string{"rule": name, "n": fmt.Sprint(r.N), "from": from, "severity": sev})
		case model.RuleActionImpact:
			// The matrix applies again to the level the rule gives.
			level = r.Level
			setLevel()
			sev = p.Matrix.Get(event, level)
			step(StepRuleImpact, map[string]string{"rule": name, "level": level, "from": from, "severity": sev})
		}
		im.Rule = name
		if r.Stop {
			break
		}
	}
	return sev, im
}

func (r compiledRule) match(ctx context.Context, vars map[string]any) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	out, _, err := r.prg.ContextEval(ctx, vars)
	if err != nil {
		return false, err
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("the condition returned %v, not bool", out.Value())
	}
	return b, nil
}

func impactVars(a *Alert, im *Impact, event, level string) map[string]any {
	services, crits, direct := []string{}, []string{}, []string{}
	for _, s := range im.Services {
		services = append(services, s.Name)
		crits = append(crits, s.Criticality)
		if s.Direct {
			direct = append(direct, s.Name)
		}
	}
	team := ""
	if a.Route.Team != nil {
		team = a.Route.Team.Name
	}
	labels := maps.Clone(a.Labels)
	if labels == nil {
		labels = map[string]string{}
	}
	return map[string]any{"event_severity": event, "severity": event, "impact": level, "title": a.Title,
		"ci": a.CIName, "ci_kind": a.CIKind, "signal": a.Signal, "method": a.Method, "labels": labels,
		"services": services, "service_criticalities": crits, "direct_services": direct, "team": team}
}

// sameImpact compares two impacts but for the time they were found.
func sameImpact(a, b *Impact) bool {
	if a == nil || b == nil {
		return a == b
	}
	x, y := *a, *b
	x.At, y.At = time.Time{}, time.Time{}
	ja, _ := json.Marshal(x)
	jb, _ := json.Marshal(y)
	return string(ja) == string(jb)
}

// eventSeverity is the severity the firing sources give; an alert stored before priorities has
// only Severity, which was the event severity then.
func (a *Alert) eventSeverity() string { return firstNonEmpty(a.EventSeverity, a.Severity) }

// prioritize finds the priority and the impact of the alert again and records a change of the
// priority on the timeline (priority_set) when the policy moves it away from the event severity
// or the result changes. It reports whether the priority rose.
func (e *Engine) prioritize(c *change, w *world, now time.Time) (raised bool) {
	a := c.a
	prev, prevEvent := a.Severity, a.eventSeverity()
	if a.EventSeverity == "" {
		a.EventSeverity = a.Severity
		c.dirty = true
	}
	sev, im := w.evaluate(a, now)
	if !sameImpact(a.Impact, im) {
		a.Impact = im
		c.dirty = true
	}
	if sev == prev {
		return false
	}
	a.Severity = sev
	c.dirty = true
	if sev != a.EventSeverity || prev != prevEvent {
		args := map[string]string{"event": a.EventSeverity, "impact": firstNonEmpty(im.Level, ImpactUnknown), "from": prev, "severity": sev}
		if im.Rule != "" {
			args["rule"] = im.Rule
		}
		c.log(now, KindStatus, "priority_set", args, "")
	}
	return SeverityRank(sev) > SeverityRank(prev)
}

// raisedCmd sends the alert to PagerDuty again after its priority rose outside the folding of
// an event (a catalog or a policy change): PagerDuty gets the new severity, and an alert it
// skipped below its threshold may now pass it.
func (e *Engine) raisedCmd(a *Alert, now time.Time) *Command {
	if !Active(a.Status) || a.Suppressed {
		return nil
	}
	if a.PD.State == PDSkipped || a.PD.State == PDOff {
		a.PD.State = PDPending
	}
	return e.pdCmd(a, PDTrigger, now)
}

// ImpactEvent is a sample event for PreviewImpact.
type ImpactEvent struct {
	Title    string            `json:"title"`
	CI       string            `json:"ci"`
	Signal   string            `json:"signal"`
	Severity string            `json:"severity"`
	Method   string            `json:"method"`
	Labels   map[string]string `json:"labels"`
}

// ImpactPreview is the priority an event would get now.
type ImpactPreview struct {
	EventSeverity string  `json:"event_severity"`
	Severity      string  `json:"severity"`
	Impact        *Impact `json:"impact"`
	CI            *Ref    `json:"ci,omitempty"`
	Team          *Ref    `json:"team,omitempty"`
}

// PreviewImpact finds the priority an incident of the event would get from the current catalog
// and the given policy (the saved one when nil), by the same rules as the engine.
func PreviewImpact(st *store.Store, ev ImpactEvent, policy *model.ImpactPolicy, now time.Time) ImpactPreview {
	w := snapshot(st)
	if policy != nil {
		w.impact = compilePolicy(*policy)
	}
	a := &Alert{Title: ev.Title, Signal: firstNonEmpty(ev.Signal, ev.Title), Method: firstNonEmpty(ev.Method, model.MethodOther),
		Severity: ev.Severity, EventSeverity: ev.Severity, Labels: ev.Labels}
	ci := w.resolve(ev.CI, ev.Labels)
	a.CIName = eventCIName(ev.CI, ev.Labels)
	if ci != nil {
		a.CIID, a.CIName, a.CIKind = ci.ID, ci.Name, ci.Kind
	}
	a.Route = w.route(ci, now)
	sev, im := w.evaluate(a, now)
	out := ImpactPreview{EventSeverity: ev.Severity, Severity: sev, Impact: im, Team: a.Route.Team}
	if ci != nil {
		out.CI = &Ref{ID: ci.ID, Name: ci.Name}
	}
	return out
}
