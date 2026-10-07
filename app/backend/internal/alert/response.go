package alert

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Impact tells how the priority of an alert was found: the severity the events gave, the
// business services the item affects and the rules that changed the result.
type Impact struct {
	// Level is the business impact: the highest criticality (model.Criticalities) of the
	// affected services, model.ImpactNone when the item affects none, empty when it is not
	// known (the item is not in the catalog, or the catalog has no services).
	Level string `json:"level"`
	// Services are the affected services, most critical first: those the item runs (Direct)
	// and those that depend on them.
	Services []ImpactService `json:"services,omitempty"`
	// Rule is the classification rule that changed the priority last, if any.
	Rule string `json:"rule,omitempty"`
	// Steps explain the result in the order they applied; codes and args are translated by the
	// interface like timeline entries.
	Steps []ImpactStep `json:"steps,omitempty"`
	At    time.Time    `json:"at"`
}

type ImpactService struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Criticality string `json:"criticality"`
	// Direct: the item is bound to the service; otherwise the service depends on one that is.
	Direct bool `json:"direct"`
}

type ImpactStep struct {
	Code string            `json:"code"`
	Args map[string]string `json:"args,omitempty"`
}

// Response is what incident response did for an alert: the plan that runs, the war room, the
// conference bridges and the tracker issues. The response package keeps it.
type Response struct {
	// Plan is the severity whose response plan runs (the alert's severity when it last ran).
	Plan    string  `json:"plan,omitempty"`
	WarRoom *Link   `json:"war_room,omitempty"`
	Bridges []Link  `json:"bridges,omitempty"`
	Issues  []Issue `json:"issues,omitempty"`
}

// Link is a chat or a meeting made for the alert. Kind names the provider (teams, zoom).
type Link struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	URL  string `json:"url"`
}

// Issue is a tracker issue made for the alert. Kind is resolution or postmortem.
type Issue struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
	URL  string `json:"url"`
}

const (
	IssueResolution = "resolution"
	IssuePostmortem = "postmortem"
)

// UpdateResponse changes the response record of an alert under its row lock and adds the
// timeline entries (kind "response"), so concurrent changes of the engine are not lost.
func (e *Engine) UpdateResponse(ctx context.Context, id string, f func(r *Response), entries ...Entry) error {
	return pgx.BeginFunc(ctx, e.db, func(tx pgx.Tx) error {
		a, err := lockByID(ctx, tx, id)
		if err != nil {
			return err
		}
		if a == nil {
			return ErrNotFound
		}
		if a.Response == nil {
			a.Response = &Response{}
		}
		f(a.Response)
		now := e.now()
		for i := range entries {
			if entries[i].At.IsZero() {
				entries[i].At = now
			}
			if entries[i].Kind == "" {
				entries[i].Kind = "response"
			}
		}
		return save(ctx, tx, a, entries)
	})
}

// ResponseCandidates are the alerts incident response looks at: the active ones and those
// resolved since the given time, most recent first, at most limit.
func (e *Engine) ResponseCandidates(ctx context.Context, resolvedSince time.Time, limit int) ([]Alert, error) {
	rows, err := e.db.Query(ctx, `SELECT doc FROM alerts WHERE `+sqlActive+` OR resolved_at >= $1
		ORDER BY last_seen DESC LIMIT $2`, resolvedSince, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Alert
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		if a != nil {
			out = append(out, *a)
		}
	}
	return out, rows.Err()
}
