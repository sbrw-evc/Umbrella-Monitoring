// Package alert is the Alert Engine: final CI binding, dedup of identical
// events across sources, the alert lifecycle, maintenance suppression,
// RED/USE linking and the hand-off to PagerDuty and the fallback notifier.
package alert

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pipeline"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// PDAction is what the PagerDuty Gateway must do.
type PDAction string

const (
	PDTrigger     PDAction = "trigger"
	PDAcknowledge PDAction = "acknowledge"
	PDResolve     PDAction = "resolve"
)

// PDCommand is queued for the PagerDuty Gateway (the outbox).
type PDCommand struct {
	Action PDAction
	Alert  model.Alert
}

// Sender is implemented by the PagerDuty Gateway.
type Sender interface {
	Send(cmd PDCommand)
}

// Engine processes normalized events into alerts.
type Engine struct {
	st     *store.Store
	pd     Sender
	notify func(kind string, v any)
	now    func() time.Time

	// Window is how long a resolved alert can be reopened by the same key
	// and how close RED and USE alerts must be to be linked.
	Window time.Duration
	// FallbackAfter: an error or critical alert not accepted by PagerDuty
	// within this time from opening goes to the fallback channels.
	FallbackAfter time.Duration
}

// New creates the engine. notify may be nil.
func New(st *store.Store, pd Sender, notify func(kind string, v any)) *Engine {
	if notify == nil {
		notify = func(string, any) {}
	}
	return &Engine{st: st, pd: pd, notify: notify, now: time.Now, Window: 10 * time.Minute, FallbackAfter: 2 * time.Minute}
}

// SetClock overrides time for tests.
func (e *Engine) SetClock(now func() time.Time) { e.now = now }

// Source identifies the connector that produced events.
type Source struct {
	ID   string
	Name string
}

// Ingest writes normalized drafts as events and folds them into alerts.
// It returns the stored events.
func (e *Engine) Ingest(src Source, drafts []pipeline.Draft) []model.Event {
	now := e.now()
	var events []model.Event
	var changed []model.Alert
	var cmds []PDCommand

	e.st.Write(func(d *store.Data) {
		for _, dr := range drafts {
			ev := &model.Event{
				ID:          d.NextID("EV"),
				ConnectorID: src.ID,
				Source:      src.Name,
				ExternalID:  dr.ExternalID,
				CIName:      dr.CI,
				Signal:      dr.Signal,
				Method:      dr.Method,
				Severity:    dr.Severity,
				Status:      dr.Status,
				Title:       dr.Title,
				Value:       dr.Value,
				Labels:      dr.Labels,
				Raw:         dr.Raw,
				ReceivedAt:  now,
			}
			if dup := findInbox(d, src.ID, dr.ExternalID, dr.Status, now); dup {
				// Same external_id and status already received: the source
				// retried because our ack was lost. Drop it (inbox dedup).
				continue
			}
			ci := ResolveCI(d, dr.CI, dr.Labels)
			if ci != nil {
				ev.CIID = ci.ID
				ev.CIName = ci.Name
			}
			a, cmd := e.fold(d, ev, ci, now)
			if a != nil {
				ev.AlertID = a.ID
				ev.Suppressed = a.Suppressed
				changed = append(changed, cloneAlert(a))
			}
			if cmd != nil {
				cmds = append(cmds, *cmd)
			}
			d.AddEvent(ev)
			events = append(events, *ev)
			if c := d.Connectors[src.ID]; c != nil {
				c.EventsTotal++
				t := now
				c.LastEventAt = &t
			}
		}
	})
	for _, ev := range events {
		e.notify("event", ev)
	}
	for _, a := range changed {
		e.notify("alert", a)
	}
	for _, c := range cmds {
		e.pd.Send(c)
	}
	return events
}

// findInbox reports whether the same event was already received recently.
func findInbox(d *store.Data, connectorID, externalID string, status model.EventStatus, now time.Time) bool {
	if externalID == "" {
		return false
	}
	for i := len(d.Events) - 1; i >= 0; i-- {
		ev := d.Events[i]
		if now.Sub(ev.ReceivedAt) > 7*24*time.Hour {
			break
		}
		if ev.ConnectorID == connectorID && ev.ExternalID == externalID {
			return ev.Status == status
		}
	}
	return false
}

// ResolveCI finds the CI by stable tag, name or any identity (host, IP,
// cloud instance id, including old ids kept as history).
func ResolveCI(d *store.Data, name string, labels map[string]string) *model.CI {
	if tag := labels["ci"]; tag != "" {
		name = tag
	}
	if name == "" {
		return nil
	}
	key := strings.ToLower(strings.TrimSpace(name))
	ids := make([]string, 0, len(d.CIs))
	for id := range d.CIs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		ci := d.CIs[id]
		if strings.ToLower(ci.Name) == key || strings.ToLower(ci.ID) == key {
			return ci
		}
	}
	for _, id := range ids {
		ci := d.CIs[id]
		for _, idn := range ci.Identities {
			if strings.ToLower(idn.Value) == key {
				return ci
			}
		}
	}
	return nil
}

// ServiceOf returns the nearest IT service above the CI (or the CI itself).
func ServiceOf(d *store.Data, ciID string) *model.CI {
	seen := map[string]bool{}
	queue := []string{ciID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		if ci := d.CIs[id]; ci != nil && ci.Type == model.CIITService {
			return ci
		}
		for _, r := range d.Relations {
			if r.To == id {
				queue = append(queue, r.From)
			}
		}
	}
	return nil
}

func inMaintenance(d *store.Data, ciID, serviceID string, now time.Time) *model.Maintenance {
	for _, m := range d.Maintenance {
		if (m.CIID == ciID || (serviceID != "" && m.CIID == serviceID)) && m.State(now) == "active" {
			return m
		}
	}
	return nil
}

func dedupKey(ev *model.Event) string {
	ci := ev.CIID
	if ci == "" {
		ci = "unresolved:" + strings.ToLower(ev.CIName)
	}
	return ci + "|" + ev.Signal
}

func (e *Engine) fold(d *store.Data, ev *model.Event, ci *model.CI, now time.Time) (*model.Alert, *PDCommand) {
	key := dedupKey(ev)
	var a *model.Alert
	for _, cand := range d.Alerts {
		if cand.DedupKey != key {
			continue
		}
		if cand.Status.Active() || (cand.ResolvedAt != nil && now.Sub(*cand.ResolvedAt) <= e.Window) {
			if a == nil || cand.LastSeen.After(a.LastSeen) {
				a = cand
			}
		}
	}

	if ev.Status == model.EventResolved {
		if a == nil || !a.Status.Active() {
			return a, nil
		}
		a.Sources[ev.ConnectorID] = string(model.EventResolved)
		a.LastSeen = now
		a.Count++
		a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "event", Text: fmt.Sprintf("%s: норма", ev.Source)})
		for _, st := range a.Sources {
			if st != string(model.EventResolved) {
				return a, nil
			}
		}
		e.resolve(a, now, "все источники вернулись в норму", "")
		return a, e.pdCmd(a, PDResolve)
	}

	if a == nil {
		a = &model.Alert{
			ID:        d.NextID("INC"),
			DedupKey:  key,
			Title:     ev.Title,
			CIName:    ev.CIName,
			Signal:    ev.Signal,
			Method:    ev.Method,
			Severity:  ev.Severity,
			Status:    model.AlertOpen,
			Sources:   map[string]string{},
			FirstSeen: now,
			LastSeen:  now,
			PDState:   model.PDPending,
		}
		a.PDKey = "umb-" + a.ID
		if ci != nil {
			a.CIID = ci.ID
			a.CIType = ci.Type
			a.Team = ci.Team
			if svc := ServiceOf(d, ci.ID); svc != nil {
				a.Service = svc.Name
				if a.Team == "" {
					a.Team = svc.Team
				}
			}
		}
		a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "status", Text: "тревога открыта"})
		if ci == nil {
			a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "status", Text: "КЕ не найдена в карте CMDB: тревога в очереди «без КЕ»"})
		}
		d.Alerts[a.ID] = a
		e.link(d, a, now)
	} else if !a.Status.Active() {
		a.Status = model.AlertOpen
		a.ResolvedAt = nil
		a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "status", Text: "тревога открыта повторно в окне склейки"})
	}

	a.Sources[ev.ConnectorID] = string(model.EventFiring)
	a.Count++
	a.LastSeen = now
	prev := a.Severity
	a.Severity = model.MaxSeverity(a.Severity, ev.Severity)
	a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "event", Text: fmt.Sprintf("%s: %s (%s)", ev.Source, ev.Title, ev.Severity)})
	if a.Severity != prev {
		a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "status", Text: fmt.Sprintf("severity повышена: %s → %s", prev, a.Severity)})
	}

	if m := inMaintenance(d, a.CIID, serviceID(d, a), now); m != nil {
		if !a.Suppressed {
			a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "maintenance", Text: "подавлена окном обслуживания «" + m.Title + "»"})
		}
		a.Suppressed = true
		a.PDState = model.PDSkipped
		return a, nil
	}
	a.Suppressed = false
	if a.Count == 1 || a.Severity != prev || a.PDState == model.PDSkipped || a.PDState == model.PDFailed {
		if a.PDState == model.PDSkipped {
			a.PDState = model.PDPending
		}
		return a, e.pdCmd(a, PDTrigger)
	}
	return a, nil
}

func serviceID(d *store.Data, a *model.Alert) string {
	if a.CIID == "" {
		return ""
	}
	if svc := ServiceOf(d, a.CIID); svc != nil {
		return svc.ID
	}
	return ""
}

// link connects a RED alert of a service with a USE alert of its CI opened
// within the window: the USE alert is the probable cause.
func (e *Engine) link(d *store.Data, a *model.Alert, now time.Time) {
	if a.Service == "" || (a.Method != model.MethodRED && a.Method != model.MethodUSE) {
		return
	}
	for _, o := range d.Alerts {
		if o.ID == a.ID || !o.Status.Active() || o.Service != a.Service || o.Method == a.Method {
			continue
		}
		if o.Method != model.MethodRED && o.Method != model.MethodUSE {
			continue
		}
		if now.Sub(o.FirstSeen) > e.Window {
			continue
		}
		a.RelatedID = o.ID
		o.RelatedID = a.ID
		red, use := a, o
		if a.Method == model.MethodUSE {
			red, use = o, a
		}
		red.Timeline = append(red.Timeline, model.TimelineEntry{At: now, Kind: "status", Text: "вероятная причина: USE-тревога " + use.ID + " на " + use.CIName})
		use.Timeline = append(use.Timeline, model.TimelineEntry{At: now, Kind: "status", Text: "влияет на сервис: RED-тревога " + red.ID})
		return
	}
}

func (e *Engine) resolve(a *model.Alert, now time.Time, why, actor string) {
	a.Status = model.AlertResolved
	t := now
	a.ResolvedAt = &t
	a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "status", Text: "тревога решена: " + why, Author: actor})
}

func (e *Engine) pdCmd(a *model.Alert, action PDAction) *PDCommand {
	if a.Suppressed {
		return nil
	}
	return &PDCommand{Action: action, Alert: cloneAlert(a)}
}

// ErrNotFound is returned for unknown alert ids.
var ErrNotFound = fmt.Errorf("тревога не найдена")

// Act applies a user action: ack, resolve or comment.
func (e *Engine) Act(id, action, actor, text string) (model.Alert, error) {
	now := e.now()
	var out model.Alert
	var cmd *PDCommand
	var err error
	e.st.Write(func(d *store.Data) {
		a := d.Alerts[id]
		if a == nil {
			err = ErrNotFound
			return
		}
		switch action {
		case "ack":
			if a.Status != model.AlertOpen {
				err = fmt.Errorf("подтвердить можно только открытую тревогу")
				return
			}
			a.Status = model.AlertAcknowledged
			a.AckedBy = actor
			a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "status", Text: "подтверждена в Umbrella", Author: actor})
			cmd = e.pdCmd(a, PDAcknowledge)
		case "resolve":
			if !a.Status.Active() {
				err = fmt.Errorf("тревога уже решена")
				return
			}
			e.resolve(a, now, "вручную", actor)
			cmd = e.pdCmd(a, PDResolve)
		case "comment":
			if strings.TrimSpace(text) == "" {
				err = fmt.Errorf("пустой комментарий")
				return
			}
			a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "comment", Text: text, Author: actor})
		default:
			err = fmt.Errorf("неизвестное действие %q", action)
			return
		}
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: actor, Action: "alert." + action, Object: id})
		out = cloneAlert(a)
	})
	if err != nil {
		return out, err
	}
	e.notify("alert", out)
	if cmd != nil {
		e.pd.Send(*cmd)
	}
	return out, nil
}

// PDResult is reported by the gateway after each delivery attempt.
func (e *Engine) PDResult(alertID string, action PDAction, deliveryErr error) {
	now := e.now()
	var out model.Alert
	ok := false
	e.st.Write(func(d *store.Data) {
		a := d.Alerts[alertID]
		if a == nil {
			return
		}
		ok = true
		if deliveryErr != nil {
			a.PDError = deliveryErr.Error()
			if action == PDTrigger && a.PDState != model.PDAccepted && a.PDState != model.PDAcked {
				a.PDState = model.PDFailed
			}
			a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "pagerduty", Text: string(action) + " не принят: " + deliveryErr.Error()})
		} else {
			a.PDError = ""
			switch action {
			case PDTrigger:
				if a.PDState != model.PDAcked {
					a.PDState = model.PDAccepted
				}
			case PDAcknowledge:
				a.PDState = model.PDAcked
			}
			a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "pagerduty", Text: string(action) + " принят PagerDuty, dedup_key " + a.PDKey})
		}
		out = cloneAlert(a)
	})
	if ok {
		e.notify("alert", out)
	}
}

// PDInbound applies a status change that came from PagerDuty Webhooks v3.
func (e *Engine) PDInbound(dedupKey, eventType, actor string) error {
	now := e.now()
	var out model.Alert
	var err error
	e.st.Write(func(d *store.Data) {
		var a *model.Alert
		for _, cand := range d.Alerts {
			if cand.PDKey == dedupKey {
				a = cand
			}
		}
		if a == nil {
			err = ErrNotFound
			return
		}
		switch eventType {
		case "incident.acknowledged":
			if a.Status == model.AlertOpen {
				a.Status = model.AlertAcknowledged
				a.AckedBy = actor
			}
			a.PDState = model.PDAcked
			a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "pagerduty", Text: "подтверждена в PagerDuty", Author: actor})
		case "incident.resolved":
			if a.Status.Active() {
				e.resolve(a, now, "в PagerDuty", actor)
			}
		case "incident.unacknowledged", "incident.reopened":
			if a.Status == model.AlertAcknowledged {
				a.Status = model.AlertOpen
			}
			a.PDState = model.PDAccepted
			a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "pagerduty", Text: "снова открыта в PagerDuty"})
		default:
			a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "pagerduty", Text: eventType})
		}
		out = cloneAlert(a)
	})
	if err == nil {
		e.notify("alert", out)
	}
	return err
}

// Tick runs periodic checks: fallback notification for alerts PagerDuty
// did not accept in time, and re-sending alerts whose maintenance ended.
func (e *Engine) Tick() {
	now := e.now()
	var changed []model.Alert
	var cmds []PDCommand
	e.st.Write(func(d *store.Data) {
		for _, a := range d.Alerts {
			if !a.Status.Active() {
				continue
			}
			if !a.Fallback && !a.Suppressed && a.Severity.Rank() >= model.SevError.Rank() &&
				(a.PDState == model.PDPending || a.PDState == model.PDFailed) &&
				now.Sub(a.FirstSeen) >= e.FallbackAfter {
				a.Fallback = true
				a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "fallback",
					Text: fmt.Sprintf("PagerDuty не принял тревогу за %s: резервное оповещение дежурным (почта, webhook)", e.FallbackAfter)})
				changed = append(changed, cloneAlert(a))
			}
			if a.Suppressed && inMaintenance(d, a.CIID, serviceID(d, a), now) == nil {
				a.Suppressed = false
				a.PDState = model.PDPending
				a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "maintenance", Text: "окно обслуживания закончилось, тревога активна"})
				changed = append(changed, cloneAlert(a))
				cmds = append(cmds, PDCommand{Action: PDTrigger, Alert: cloneAlert(a)})
			}
		}
	})
	for _, a := range changed {
		e.notify("alert", a)
	}
	for _, c := range cmds {
		e.pd.Send(c)
	}
}

// Get returns a copy of the alert.
func (e *Engine) Get(id string) (model.Alert, bool) {
	var out model.Alert
	ok := false
	e.st.Read(func(d *store.Data) {
		if a := d.Alerts[id]; a != nil {
			out, ok = cloneAlert(a), true
		}
	})
	return out, ok
}

func cloneAlert(a *model.Alert) model.Alert {
	c := *a
	c.Sources = make(map[string]string, len(a.Sources))
	for k, v := range a.Sources {
		c.Sources[k] = v
	}
	c.Timeline = append([]model.TimelineEntry(nil), a.Timeline...)
	if a.ResolvedAt != nil {
		t := *a.ResolvedAt
		c.ResolvedAt = &t
	}
	return c
}

// Clone exposes cloneAlert for readers that hold the store lock.
func Clone(a *model.Alert) model.Alert { return cloneAlert(a) }
