package alert

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

const (
	DefaultWindow        = 10 * time.Minute
	DefaultFallbackAfter = 2 * time.Minute
	DefaultRetryEvery    = time.Minute
	// Retention: resolved alerts older than this are deleted with their timelines.
	Retention = 90 * 24 * time.Hour
	tickBatch = 1000
)

// ErrPDSkipped: the alert is below the PagerDuty severity threshold. The gateway reports it
// so the alert is not retried.
var ErrPDSkipped = errors.New("below the PagerDuty severity threshold")

type Engine struct {
	db     *pgxpool.Pool
	st     *store.Store
	pd     Sender
	notify Notifier
	now    func() time.Time

	// Window: an alert resolved this long ago opens again on a new firing event instead of a
	// new alert, and RED and USE alerts of one service opened within it are linked.
	Window time.Duration
	// FallbackAfter: an error or critical alert PagerDuty has not taken this long after it
	// opened goes to backup notification.
	FallbackAfter time.Duration
	RetryEvery    time.Duration

	mu        sync.Mutex
	cached    *world
	cachedVer uint64
	purgedAt  time.Time
}

type nopSender struct{}

func (nopSender) Send(Command) {}

type nopNotifier struct{}

func (nopNotifier) Fallback(Alert) {}

func New(db *pgxpool.Pool, st *store.Store) *Engine {
	return &Engine{db: db, st: st, pd: nopSender{}, notify: nopNotifier{}, now: func() time.Time { return time.Now().UTC() },
		Window: DefaultWindow, FallbackAfter: DefaultFallbackAfter, RetryEvery: DefaultRetryEvery}
}

func (e *Engine) SetSender(s Sender)            { e.pd = s }
func (e *Engine) SetNotifier(n Notifier)        { e.notify = n }
func (e *Engine) SetClock(now func() time.Time) { e.now = now }
func (e *Engine) DB() *pgxpool.Pool             { return e.db }

func (e *Engine) world() *world {
	v := e.st.Version()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cached == nil || e.cachedVer != v {
		e.cached, e.cachedVer = snapshot(e.st), v
	}
	return e.cached
}

// change collects what happened to one alert in one transaction.
type change struct {
	a       *Alert
	entries []Entry
	dirty   bool
}

func (c *change) log(at time.Time, kind, code string, args map[string]string, author string) {
	c.entries = append(c.entries, Entry{At: at, Kind: kind, Code: code, Args: args, Author: author})
	c.dirty = true
}

func (c *change) save(ctx context.Context, tx pgx.Tx) error {
	if !c.dirty {
		return nil
	}
	return save(ctx, tx, c.a, c.entries)
}

func dedupKey(ci *model.ConfigItem, name, signal string) string {
	signal = strings.ToLower(strings.TrimSpace(signal))
	if ci != nil {
		return "ci:" + ci.ID + "|" + signal
	}
	return "name:" + strings.ToLower(strings.TrimSpace(name)) + "|" + signal
}

// Apply folds the events of one request into alerts inside the caller's transaction, so the
// events and the alerts are written together. after sends the PagerDuty commands and must be
// called once the transaction is committed.
func (e *Engine) Apply(ctx context.Context, tx pgx.Tx, events []Incoming) (after func(), err error) {
	if len(events) == 0 {
		return func() {}, nil
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(lockKey)); err != nil {
		return nil, err
	}
	w := e.world()
	now := e.now()
	var cmds []Command
	for _, in := range events {
		c, err := e.fold(ctx, tx, w, in, now)
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, c...)
	}
	return func() {
		for _, c := range cmds {
			e.pd.Send(c)
		}
	}, nil
}

// Ingest folds events that do not come through the intake queue (rules) in a transaction of
// its own.
func (e *Engine) Ingest(ctx context.Context, events []Incoming) error {
	var after func()
	err := pgx.BeginFunc(ctx, e.db, func(tx pgx.Tx) error {
		var err error
		after, err = e.Apply(ctx, tx, events)
		return err
	})
	if err == nil && after != nil {
		after()
	}
	return err
}

func (e *Engine) fold(ctx context.Context, tx pgx.Tx, w *world, in Incoming, now time.Time) ([]Command, error) {
	ci := w.resolve(in.CI, in.Labels)
	name := strings.TrimSpace(in.CI)
	if v := strings.TrimSpace(in.Labels["ci"]); v != "" {
		name = v
	}
	signal := in.Signal
	if strings.TrimSpace(signal) == "" {
		signal = in.Title
	}
	key := dedupKey(ci, name, signal)
	a, err := current(ctx, tx, key, now.Add(-e.Window))
	if err != nil {
		return nil, err
	}
	c := &change{a: a}
	if a == nil && ci != nil {
		// An alert opened while the item was not in the catalog is bound to it now.
		if a, err = activeByKey(ctx, tx, dedupKey(nil, name, signal)); err != nil {
			return nil, err
		}
		if a != nil {
			c.a = a
			a.DedupKey = key
			e.bind(c, w, ci, now)
		}
	}
	srcKey := in.ConnectorID + "/" + in.Key
	var cmds []Command
	add := func(cmd *Command) {
		if cmd != nil {
			cmds = append(cmds, *cmd)
		}
	}
	eventArgs := map[string]string{"connector": in.ConnectorID, "title": in.Title, "severity": in.Severity, "status": in.Status}
	if in.Value != "" {
		eventArgs["value"] = in.Value
	}

	if in.Status == SourceResolved {
		if a == nil {
			return nil, nil
		}
		src := a.Sources[srcKey]
		if src == nil || src.Status == SourceResolved {
			if src != nil {
				src.LastSeen = now
				c.dirty = true
			}
			return nil, c.save(ctx, tx)
		}
		src.Status, src.LastSeen, src.Value = SourceResolved, now, in.Value
		a.Count++
		a.LastSeen = now
		if !Active(a.Status) {
			c.dirty = true
			return nil, c.save(ctx, tx)
		}
		c.log(now, KindEvent, "event", eventArgs, "")
		a.Severity = firingSeverity(a, a.Severity)
		if allResolved(a) {
			e.resolve(c, now, "sources_resolved", "")
			add(e.pdCmd(a, PDResolve, now))
		}
		return cmds, c.save(ctx, tx)
	}

	opened, reopened := false, false
	if a == nil {
		id, _, err := nextID(ctx, tx)
		if err != nil {
			return nil, err
		}
		a = &Alert{ID: id, DedupKey: key, Title: in.Title, CIName: name, Signal: signal, Method: in.Method, Severity: in.Severity,
			Status: StatusOpen, Sources: map[string]*Source{}, Labels: map[string]string{}, FirstSeen: now, OpenedAt: now, LastSeen: now,
			PD: PD{State: PDPending, Key: "umb-" + id}}
		if a.Method == "" {
			a.Method = "other"
		}
		c.a = a
		c.log(now, KindStatus, "opened", nil, "")
		if ci != nil {
			e.bind(c, w, ci, now)
		} else {
			a.Route = w.route(nil, now)
			c.log(now, KindRoute, "ci_unknown", map[string]string{"ci": name}, "")
		}
		if err := e.link(ctx, tx, c, now); err != nil {
			return nil, err
		}
		opened = true
	} else if !Active(a.Status) {
		a.Status, a.ResolvedAt, a.ResolvedBy, a.AckedBy, a.AckedAt = StatusOpen, nil, "", "", nil
		a.OpenedAt, a.Fallback, a.FallbackAt = now, false, nil
		a.PD.State, a.PD.Error, a.PD.Retry, a.PD.AttemptAt = PDPending, "", "", nil
		c.log(now, KindStatus, "reopened", map[string]string{"window": e.Window.String()}, "")
		reopened = true
	}

	src := a.Sources[srcKey]
	if src != nil && src.Status == SourceFiring && src.Severity == in.Severity && !opened && !reopened {
		// A repeated delivery: the alert is only touched.
		src.LastSeen, src.Value = now, in.Value
		a.Count++
		a.LastSeen = now
		c.dirty = true
		return nil, c.save(ctx, tx)
	}
	if src == nil {
		src = &Source{ConnectorID: in.ConnectorID, Key: in.Key, FirstSeen: now}
		a.Sources[srcKey] = src
	}
	src.Status, src.Severity, src.Title, src.Value, src.LastSeen = SourceFiring, in.Severity, in.Title, in.Value, now
	maps.Copy(a.Labels, in.Labels)
	a.Count++
	a.LastSeen = now
	c.log(now, KindEvent, "event", eventArgs, "")
	prev := a.Severity
	a.Severity = firingSeverity(a, in.Severity)
	raised := SeverityRank(a.Severity) > SeverityRank(prev)
	if raised && !opened {
		c.log(now, KindStatus, "severity_raised", map[string]string{"from": prev, "to": a.Severity}, "")
	}

	if m := w.maintenanceFor(a, now); m != nil {
		if !a.Suppressed {
			c.log(now, KindMaintenance, "suppressed", map[string]string{"window": m.Title, "id": m.ID}, "")
		}
		a.Suppressed, a.MaintenanceID = true, m.ID
		if a.PD.State != PDAccepted && a.PD.State != PDAcked {
			a.PD.State = PDSkipped
		}
		return nil, c.save(ctx, tx)
	}
	wasSuppressed := a.Suppressed
	a.Suppressed, a.MaintenanceID = false, ""
	if opened || reopened || raised || wasSuppressed || a.PD.State == PDSkipped || a.PD.State == PDFailed {
		if a.PD.State == PDSkipped {
			a.PD.State = PDPending
		}
		add(e.pdCmd(a, PDTrigger, now))
	}
	return cmds, c.save(ctx, tx)
}

// firingSeverity is the highest severity of the sources that still fire.
func firingSeverity(a *Alert, fallback string) string {
	best := ""
	for _, s := range a.Sources {
		if s.Status == SourceFiring && SeverityRank(s.Severity) > SeverityRank(best) {
			best = s.Severity
		}
	}
	if best == "" {
		return fallback
	}
	return best
}

func allResolved(a *Alert) bool {
	for _, s := range a.Sources {
		if s.Status != SourceResolved {
			return false
		}
	}
	return true
}

// bind attaches the alert to a configuration item and routes it.
func (e *Engine) bind(c *change, w *world, ci *model.ConfigItem, now time.Time) {
	a := c.a
	a.CIID, a.CIName, a.CIKind = ci.ID, ci.Name, ci.Kind
	if len(a.Sources) > 0 {
		c.log(now, KindRoute, "ci_bound", map[string]string{"ci": ci.Name, "ci_id": ci.ID}, "")
	}
	e.reroute(c, w, ci, now)
}

func (e *Engine) reroute(c *change, w *world, ci *model.ConfigItem, now time.Time) {
	a := c.a
	r := w.route(ci, now)
	a.Route = r
	args := map[string]string{"via": r.Via, "people": fmt.Sprint(len(r.Recipients()))}
	if len(r.Services) > 0 {
		names := make([]string, 0, len(r.Services))
		for _, s := range r.Services {
			names = append(names, s.Name)
		}
		args["services"] = strings.Join(names, ", ")
	}
	if r.Team != nil {
		args["team"] = r.Team.Name
	}
	c.log(now, KindRoute, "routed", args, "")
}

// link marks RED and USE alerts of the same service opened within the window: the USE alert
// is the probable cause of the RED one.
func (e *Engine) link(ctx context.Context, tx pgx.Tx, c *change, now time.Time) error {
	a := c.a
	if len(a.Route.Services) == 0 || (a.Method != "red" && a.Method != "use") {
		return nil
	}
	other := "use"
	if a.Method == "use" {
		other = "red"
	}
	o, err := scanAlert(tx.QueryRow(ctx, `SELECT doc FROM alerts WHERE status <> 'resolved' AND method = $1 AND service_ids && $2
		AND first_seen >= $3 AND id <> $4 ORDER BY first_seen DESC LIMIT 1 FOR UPDATE`, other, a.Route.ServiceIDs(), now.Add(-e.Window), a.ID))
	if err != nil || o == nil {
		return err
	}
	oc := &change{a: o}
	a.RelatedID, o.RelatedID = o.ID, a.ID
	red, use, redC, useC := a, o, c, oc
	if a.Method == "use" {
		red, use, redC, useC = o, a, oc, c
	}
	redC.log(now, KindStatus, "probable_cause", map[string]string{"alert": use.ID, "ci": use.CIName}, "")
	useC.log(now, KindStatus, "affects_service", map[string]string{"alert": red.ID, "ci": red.CIName}, "")
	return oc.save(ctx, tx)
}

func (e *Engine) resolve(c *change, now time.Time, why, actor string) {
	a := c.a
	a.Status = StatusResolved
	t := now
	a.ResolvedAt, a.ResolvedBy = &t, actor
	c.log(now, KindStatus, "resolved", map[string]string{"why": why}, actor)
}

func (e *Engine) pdCmd(a *Alert, action Action, now time.Time) *Command {
	if a.Suppressed {
		return nil
	}
	t := now
	a.PD.AttemptAt = &t
	return &Command{Action: action, Alert: a.Clone()}
}

var (
	ErrNotActive    = errors.New("the alert is already resolved")
	ErrNotOpen      = errors.New("only an open alert can be acknowledged")
	ErrEmptyComment = errors.New("the comment is empty")
	ErrBadAction    = errors.New("unknown action")
)

// Act applies an action of a person: ack, resolve or comment.
func (e *Engine) Act(ctx context.Context, id, action, actor, text string) (Alert, error) {
	return e.ActIn(ctx, id, action, actor, text, nil)
}

// ActIn is Act for a person who sees only the alerts of some business services (see
// Alert.InScope): any other alert is ErrNotFound.
func (e *Engine) ActIn(ctx context.Context, id, action, actor, text string, scope []string) (Alert, error) {
	now := e.now()
	var out Alert
	var cmd *Command
	err := pgx.BeginFunc(ctx, e.db, func(tx pgx.Tx) error {
		a, err := lockByID(ctx, tx, id)
		if err != nil {
			return err
		}
		if a == nil || !a.InScope(scope) {
			return ErrNotFound
		}
		c := &change{a: a}
		switch action {
		case "ack":
			if a.Status != StatusOpen {
				return ErrNotOpen
			}
			t := now
			a.Status, a.AckedBy, a.AckedAt = StatusAcknowledged, actor, &t
			c.log(now, KindStatus, "acknowledged", nil, actor)
			cmd = e.pdCmd(a, PDAcknowledge, now)
		case "resolve":
			if !Active(a.Status) {
				return ErrNotActive
			}
			e.resolve(c, now, "manual", actor)
			cmd = e.pdCmd(a, PDResolve, now)
		case "comment":
			text = strings.TrimSpace(text)
			if text == "" {
				return ErrEmptyComment
			}
			if len(text) > 4000 {
				text = text[:4000]
			}
			c.log(now, KindComment, "comment", map[string]string{"text": text}, actor)
		default:
			return ErrBadAction
		}
		out = a.Clone()
		return c.save(ctx, tx)
	})
	if err != nil {
		return out, err
	}
	if cmd != nil {
		e.pd.Send(*cmd)
	}
	return out, nil
}

// PDResult records the outcome of a delivery to PagerDuty.
func (e *Engine) PDResult(ctx context.Context, alertID string, action Action, route string, deliveryErr error) {
	now := e.now()
	var follow []Command
	err := pgx.BeginFunc(ctx, e.db, func(tx pgx.Tx) error {
		a, err := lockByID(ctx, tx, alertID)
		if err != nil || a == nil {
			return err
		}
		c := &change{a: a, dirty: true}
		if route != "" {
			a.PD.Route = route
		}
		delivered := a.PD.State == PDAccepted || a.PD.State == PDAcked
		switch {
		case errors.Is(deliveryErr, ErrPDSkipped):
			if action == PDTrigger && !delivered {
				a.PD.State = PDSkipped
				c.log(now, KindPagerDuty, "pd_skipped", map[string]string{"reason": deliveryErr.Error()}, "")
			}
			a.PD.Retry, a.PD.Error = "", ""
		case deliveryErr != nil:
			if a.PD.Error != deliveryErr.Error() {
				c.log(now, KindPagerDuty, "pd_failed", map[string]string{"action": string(action), "error": deliveryErr.Error()}, "")
			}
			a.PD.Error, a.PD.Retry = deliveryErr.Error(), string(action)
			if action == PDTrigger && !delivered {
				a.PD.State = PDFailed
			}
		default:
			a.PD.Error, a.PD.Retry = "", ""
			switch action {
			case PDTrigger:
				if a.PD.State != PDAcked {
					a.PD.State = PDAccepted
				}
				switch a.Status {
				case StatusAcknowledged:
					if cmd := e.pdCmd(a, PDAcknowledge, now); cmd != nil {
						follow = append(follow, *cmd)
					}
				case StatusResolved:
					if cmd := e.pdCmd(a, PDResolve, now); cmd != nil {
						follow = append(follow, *cmd)
					}
				}
			case PDAcknowledge:
				a.PD.State = PDAcked
			}
			c.log(now, KindPagerDuty, "pd_accepted", map[string]string{"action": string(action), "key": a.PD.Key, "route": route}, "")
		}
		return c.save(ctx, tx)
	})
	if err != nil {
		slog.Error("pagerduty result not recorded", "alert", alertID, "err", err)
		return
	}
	for _, c := range follow {
		e.pd.Send(c)
	}
}

// PDUpdate is a change of a PagerDuty incident received by webhook.
type PDUpdate struct {
	DedupKey    string
	EventType   string
	Actor       string
	IncidentID  string
	IncidentURL string
	Detail      string
}

// PDKeys returns the dedup keys of the alerts bound to a PagerDuty incident or key.
func (e *Engine) PDKeys(ctx context.Context, incidentKey, incidentID string) ([]string, error) {
	rows, err := e.db.Query(ctx, "SELECT pd_key FROM alerts WHERE ($1 <> '' AND pd_key = $1) OR ($2 <> '' AND pd_incident = $2)", incidentKey, incidentID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// PDInbound applies a status change made in PagerDuty: acknowledging or resolving the
// incident there acknowledges or resolves the alert here.
func (e *Engine) PDInbound(ctx context.Context, u PDUpdate) error {
	now := e.now()
	return pgx.BeginFunc(ctx, e.db, func(tx pgx.Tx) error {
		a, err := scanAlert(tx.QueryRow(ctx, "SELECT doc FROM alerts WHERE pd_key = $1 ORDER BY last_seen DESC LIMIT 1 FOR UPDATE", u.DedupKey))
		if err != nil {
			return err
		}
		if a == nil {
			return ErrNotFound
		}
		c := &change{a: a, dirty: true}
		if u.IncidentID != "" {
			a.PD.IncidentID = u.IncidentID
		}
		if u.IncidentURL != "" {
			a.PD.IncidentURL = u.IncidentURL
		}
		args := map[string]string{}
		if u.Detail != "" {
			args["detail"] = u.Detail
		}
		add := func(code string) { c.log(now, KindPagerDuty, code, args, u.Actor) }
		switch u.EventType {
		case "incident.triggered":
			if a.PD.State == PDPending || a.PD.State == PDFailed {
				a.PD.State = PDAccepted
			}
			add("pd_incident_triggered")
		case "incident.acknowledged":
			if a.Status == StatusOpen {
				t := now
				a.Status, a.AckedBy, a.AckedAt = StatusAcknowledged, u.Actor, &t
			}
			a.PD.State = PDAcked
			add("pd_incident_acknowledged")
		case "incident.resolved":
			if Active(a.Status) {
				e.resolve(c, now, "pagerduty", u.Actor)
			}
		case "incident.unacknowledged", "incident.reopened":
			if a.Status == StatusAcknowledged {
				a.Status, a.AckedBy, a.AckedAt = StatusOpen, "", nil
			}
			a.PD.State = PDAccepted
			add("pd_incident_reopened")
		case "incident.reassigned", "incident.escalated", "incident.annotated", "incident.priority_updated":
			add("pd_" + strings.TrimPrefix(u.EventType, "incident."))
		default:
			args["event"] = u.EventType
			add("pd_other")
		}
		return c.save(ctx, tx)
	})
}

// Tick runs once in a while: backup notification for alerts PagerDuty has not taken, the
// start and end of maintenance windows, retries of failed deliveries and cleanup.
func (e *Engine) Tick(ctx context.Context) error {
	now := e.now()
	rows, err := e.db.Query(ctx, "SELECT id FROM alerts WHERE attention ORDER BY seq LIMIT $1", tickBatch)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	w := e.world()
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var cmds []Command
		var notify *Alert
		err := pgx.BeginFunc(ctx, e.db, func(tx pgx.Tx) error {
			a, err := lockByID(ctx, tx, id)
			if err != nil || a == nil {
				return err
			}
			c := &change{a: a}
			if Active(a.Status) {
				m := w.maintenanceFor(a, now)
				switch {
				case m != nil && !a.Suppressed:
					a.Suppressed, a.MaintenanceID = true, m.ID
					if a.PD.State == PDPending || a.PD.State == PDFailed {
						a.PD.State, a.PD.Retry = PDSkipped, ""
					}
					c.log(now, KindMaintenance, "suppressed", map[string]string{"window": m.Title, "id": m.ID}, "")
				case m == nil && a.Suppressed:
					a.Suppressed, a.MaintenanceID = false, ""
					if a.PD.State == PDSkipped {
						a.PD.State, a.PD.AttemptAt = PDPending, nil
					}
					c.log(now, KindMaintenance, "maintenance_over", nil, "")
				}
				if !a.Fallback && !a.Suppressed && SeverityRank(a.Severity) >= SeverityRank("error") &&
					(a.PD.State == PDPending || a.PD.State == PDFailed) && now.Sub(a.OpenedAt) >= e.FallbackAfter {
					t := now
					a.Fallback, a.FallbackAt = true, &t
					c.log(now, KindFallback, "fallback", map[string]string{"after": e.FallbackAfter.String(), "people": fmt.Sprint(len(a.Route.Recipients()))}, "")
					cp := a.Clone()
					notify = &cp
				}
			}
			if cmd := e.retry(a, now); cmd != nil {
				c.dirty = true
				cmds = append(cmds, *cmd)
			}
			if a.Status == StatusResolved && a.PD.Retry == "" {
				c.dirty = true // clears the attention flag
			}
			return c.save(ctx, tx)
		})
		if err != nil {
			slog.Error("alert tick", "alert", id, "err", err)
			continue
		}
		for _, cmd := range cmds {
			e.pd.Send(cmd)
		}
		if notify != nil {
			e.notify.Fallback(*notify)
		}
	}
	if now.Sub(e.purgedAt) >= time.Hour {
		e.purgedAt = now
		if _, err := e.db.Exec(ctx, "DELETE FROM alerts WHERE status = 'resolved' AND resolved_at < $1", now.Add(-Retention)); err != nil {
			slog.Error("old alerts not deleted", "err", err)
		}
	}
	return nil
}

func (e *Engine) retry(a *Alert, now time.Time) *Command {
	if a.Suppressed {
		return nil
	}
	if a.PD.AttemptAt != nil && now.Sub(*a.PD.AttemptAt) < e.RetryEvery {
		return nil
	}
	delivered := a.PD.State == PDAccepted || a.PD.State == PDAcked
	switch {
	case a.Status == StatusResolved:
		if !delivered || a.PD.Retry == "" {
			a.PD.Retry = ""
			return nil
		}
		return e.pdCmd(a, PDResolve, now)
	case !delivered:
		if a.PD.State != PDPending && a.PD.State != PDFailed {
			return nil
		}
		return e.pdCmd(a, PDTrigger, now)
	case a.PD.Retry == string(PDAcknowledge) && a.Status == StatusAcknowledged:
		return e.pdCmd(a, PDAcknowledge, now)
	case a.PD.Retry == string(PDTrigger):
		return e.pdCmd(a, PDTrigger, now)
	}
	return nil
}

// Reroute routes the active alerts of the given configuration items again, after the
// catalog changed (an item moved to another service, a team got people).
func (e *Engine) Reroute(ctx context.Context) error {
	rows, err := e.db.Query(ctx, "SELECT id FROM alerts WHERE status <> 'resolved' AND ci_id <> ''")
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	w := e.world()
	now := e.now()
	for _, id := range ids {
		err := pgx.BeginFunc(ctx, e.db, func(tx pgx.Tx) error {
			a, err := lockByID(ctx, tx, id)
			if err != nil || a == nil || !Active(a.Status) {
				return err
			}
			ci, ok := w.cis[a.CIID]
			if !ok {
				return nil
			}
			r := w.route(&ci, now)
			if sameRoute(a.Route, r) {
				return nil
			}
			c := &change{a: a}
			e.reroute(c, w, &ci, now)
			return c.save(ctx, tx)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func sameRoute(a, b Route) bool {
	if a.Via != b.Via || len(a.Services) != len(b.Services) || len(a.People) != len(b.People) || len(a.Owners) != len(b.Owners) {
		return false
	}
	if (a.Team == nil) != (b.Team == nil) || (a.Team != nil && *a.Team != *b.Team) {
		return false
	}
	for i := range a.Services {
		if a.Services[i] != b.Services[i] {
			return false
		}
	}
	for i := range a.People {
		if a.People[i] != b.People[i] {
			return false
		}
	}
	for i := range a.Owners {
		if a.Owners[i] != b.Owners[i] {
			return false
		}
	}
	return true
}

// Note adds a line to the timeline of an alert, for what happened outside the engine (backup
// notification).
func (e *Engine) Note(ctx context.Context, id, kind, code string, args map[string]string) error {
	return note(ctx, e.db, id, []Entry{{At: e.now(), Kind: kind, Code: code, Args: args}})
}

func (e *Engine) List(ctx context.Context, f Filter) (Page, error) { return list(ctx, e.db, f) }

func (e *Engine) Get(ctx context.Context, id string) (Alert, []Entry, error) {
	return get(ctx, e.db, id)
}

// Run ticks every 15 seconds until ctx ends. When the catalog changed, the active alerts are
// routed again.
func (e *Engine) Run(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	routed := e.st.Version()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := e.Tick(ctx); err != nil && ctx.Err() == nil {
				slog.Error("alert engine tick failed", "err", err)
			}
			if v := e.st.Version(); v != routed {
				if err := e.Reroute(ctx); err != nil && ctx.Err() == nil {
					slog.Error("alerts not routed again", "err", err)
				} else {
					routed = v
				}
			}
		}
	}
}
