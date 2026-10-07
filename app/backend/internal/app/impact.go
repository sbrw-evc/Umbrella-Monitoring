package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// The page «Важность и влияние»: the impact policy that finds the priority of incidents from
// the severity of their events and the business impact (see docs/impact.md).

// ImpactView is the impact policy as the page shows it: the policy that applies (the template
// while it was never saved), the template, the impact levels, the severity scale, the rule
// variables and actions.
type ImpactView struct {
	Policy     model.ImpactPolicy `json:"policy"`
	Defaults   model.ImpactPolicy `json:"defaults"`
	Levels     []string           `json:"levels"`
	Severities []model.Severity   `json:"severities"`
	Variables  []string           `json:"variables"`
	Actions    []string           `json:"actions"`
}

// ImpactPreviewInput is a sample event and, optionally, a policy not saved yet to try.
type ImpactPreviewInput struct {
	Event  alert.ImpactEvent   `json:"event"`
	Policy *model.ImpactPolicy `json:"policy,omitempty"`
}

func (a *App) registerImpact(mux *http.ServeMux) {
	a.route(mux, "GET /api/impact", "impact:view", show(a.impactView))
	a.route(mux, "PUT /api/impact", "impact:edit", submit(http.StatusOK, func(r *http.Request, in model.ImpactPolicy) (ImpactView, error) {
		return a.saveImpact(r.Context(), actorName(r), in, "settings.impact")
	}))
	a.route(mux, "POST /api/impact/reset-defaults", "impact:edit", fetch(func(r *http.Request) (ImpactView, error) {
		return a.saveImpact(r.Context(), actorName(r), model.DefaultImpactPolicy(), "settings.impact_reset")
	}))
	a.route(mux, "POST /api/impact/preview", "impact:view", submit(http.StatusOK, func(r *http.Request, in ImpactPreviewInput) (alert.ImpactPreview, error) {
		return a.previewImpact(in)
	}))
}

func (a *App) impactView() ImpactView {
	var p model.ImpactPolicy
	a.deps.Store.Read(func(d *store.Data) { p = d.Settings.Impact.Clone() })
	return ImpactView{Policy: p.Effective(), Defaults: model.DefaultImpactPolicy(), Levels: model.ImpactLevels, Severities: model.Severities,
		Variables: alert.ImpactVariables, Actions: model.RuleActions}
}

// checkImpact normalizes a policy and compiles its rule conditions.
func checkImpact(in model.ImpactPolicy) (model.ImpactPolicy, error) {
	p, err := in.Normalize()
	if err == nil {
		err = alert.CompileImpact(p)
	}
	if err != nil {
		return p, invalid("invalid_impact_policy", err)
	}
	return p, nil
}

// saveImpact saves the policy and finds the priority of the active incidents again at once
// (the engine would do it on its own within a tick).
func (a *App) saveImpact(ctx context.Context, actor string, in model.ImpactPolicy, action string) (ImpactView, error) {
	p, err := checkImpact(in)
	if err != nil {
		return ImpactView{}, err
	}
	a.deps.Store.Write(func(d *store.Data) {
		d.Settings.Impact = p
		d.AddAudit(store.AuditEntry{Actor: actor, Action: action, Detail: describeImpact(p)})
	})
	if a.alerts != nil {
		if err := a.alerts.Reroute(context.WithoutCancel(ctx)); err != nil {
			slog.Error("priorities not found again after an impact policy change", "err", err)
		}
	}
	return a.impactView(), nil
}

func describeImpact(p model.ImpactPolicy) string {
	enabled := 0
	for _, r := range p.Rules {
		if r.Enabled {
			enabled++
		}
	}
	return fmt.Sprintf("enabled %t, upstream depth %d, rules %d (%d enabled)", p.Enabled, p.UpstreamDepth, len(p.Rules), enabled)
}

func (a *App) previewImpact(in ImpactPreviewInput) (alert.ImpactPreview, error) {
	if !model.ValidSeverity(in.Event.Severity) {
		return alert.ImpactPreview{}, invalid("invalid_severity", fmt.Errorf("unknown severity %q", in.Event.Severity))
	}
	var policy *model.ImpactPolicy
	if in.Policy != nil {
		p, err := checkImpact(*in.Policy)
		if err != nil {
			return alert.ImpactPreview{}, err
		}
		policy = &p
	}
	return alert.PreviewImpact(a.deps.Store, in.Event, policy, time.Now().UTC()), nil
}
