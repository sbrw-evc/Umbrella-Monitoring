package alert

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pipeline"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type PDAction string

const (
	PDTrigger     PDAction = "trigger"
	PDAcknowledge PDAction = "acknowledge"
	PDResolve     PDAction = "resolve"
)

type PDCommand struct {
	Action PDAction
	Alert  model.Alert
}

type Sender interface {
	Send(cmd PDCommand)
}

type Engine struct {
	st     *store.Store
	pd     Sender
	notify func(kind string, v any)
	now    func() time.Time

	Window time.Duration

	FallbackAfter time.Duration
	RetryEvery    time.Duration

	AutoCMDB bool
}

func New(st *store.Store, pd Sender, notify func(kind string, v any)) *Engine {
	if notify == nil {
		notify = func(string, any) {}
	}
	return &Engine{st: st, pd: pd, notify: notify, now: time.Now, Window: 10 * time.Minute, FallbackAfter: 2 * time.Minute, RetryEvery: time.Minute}
}

func (e *Engine) SetClock(now func() time.Time) { e.now = now }

type Source struct {
	ID   string
	Name string
}

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

				continue
			}
			ci := ResolveCI(d, dr.CI, dr.Labels)
			if ci == nil && e.AutoCMDB {
				ci = autoCI(d, dr, now)
			}
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

func autoCI(d *store.Data, dr pipeline.Draft, now time.Time) *model.CI {
	name := strings.TrimSpace(dr.CI)
	if tag := dr.Labels["ci"]; tag != "" {
		name = tag
	}
	if name == "" || len(name) > 200 || len(d.CIs) >= 100000 {
		return nil
	}
	team := dr.Labels["team"]
	add := func(name, typ string) *model.CI {
		ci := &model.CI{ID: d.NextID("CI"), Name: name, Type: typ, Team: team, Origin: "auto", CreatedAt: now,
			Identities: []model.Identity{}, Labels: map[string]string{}}
		if typ == model.CIHost {
			ci.Identities = append(ci.Identities, model.Identity{Kind: "hostname", Value: name, Since: now})
			for _, k := range []string{"ip", "instance", "host"} {
				if v := dr.Labels[k]; v != "" && !strings.EqualFold(v, name) {
					ci.Identities = append(ci.Identities, model.Identity{Kind: k, Value: v, Since: now})
				}
			}
		}
		d.CIs[ci.ID] = ci
		return ci
	}
	ci := add(name, model.CIHost)
	if svc := strings.TrimSpace(dr.Labels["service"]); svc != "" && !strings.EqualFold(svc, name) {
		s := ResolveCI(d, svc, nil)
		if s == nil {
			s = add(svc, model.CIITService)
		}
		d.Relations = append(d.Relations, model.Relation{From: s.ID, To: ci.ID, Type: "runs_on"})
	}
	return ci
}

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
	t := e.now()
	a.PDAttemptAt = &t
	return &PDCommand{Action: action, Alert: cloneAlert(a)}
}

var ErrPDSkipped = errors.New("ниже порога важности PagerDuty")

var ErrNotFound = fmt.Errorf("тревога не найдена")

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

func (e *Engine) PDResult(alertID string, action PDAction, route string, deliveryErr error) {
	now := e.now()
	var out model.Alert
	var follow []PDCommand
	ok := false
	e.st.Write(func(d *store.Data) {
		a := d.Alerts[alertID]
		if a == nil {
			return
		}
		ok = true
		if route != "" {
			a.PDRoute = route
		}
		switch {
		case errors.Is(deliveryErr, ErrPDSkipped):
			if action == PDTrigger && a.PDState != model.PDAccepted && a.PDState != model.PDAcked {
				a.PDState = model.PDSkipped
				a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "pagerduty", Text: "не отправлена в PagerDuty: " + deliveryErr.Error()})
			}
			a.PDRetry, a.PDError = "", ""
		case deliveryErr != nil:
			if a.PDError != deliveryErr.Error() {
				a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "pagerduty", Text: string(action) + " не принят: " + deliveryErr.Error()})
			}
			a.PDError = deliveryErr.Error()
			a.PDRetry = string(action)
			if action == PDTrigger && a.PDState != model.PDAccepted && a.PDState != model.PDAcked {
				a.PDState = model.PDFailed
			}
		default:
			a.PDError, a.PDRetry = "", ""
			switch action {
			case PDTrigger:
				if a.PDState != model.PDAcked {
					a.PDState = model.PDAccepted
				}
				switch a.Status {
				case model.AlertAcknowledged:
					if c := e.pdCmd(a, PDAcknowledge); c != nil {
						follow = append(follow, *c)
					}
				case model.AlertResolved:
					if c := e.pdCmd(a, PDResolve); c != nil {
						follow = append(follow, *c)
					}
				}
			case PDAcknowledge:
				a.PDState = model.PDAcked
			}
			text := string(action) + " принят PagerDuty, dedup_key " + a.PDKey
			if route != "" {
				text += ", маршрут «" + route + "»"
			}
			a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "pagerduty", Text: text})
		}
		out = cloneAlert(a)
	})
	if ok {
		e.notify("alert", out)
	}
	for _, c := range follow {
		e.pd.Send(c)
	}
}

type PDUpdate struct {
	DedupKey    string
	EventType   string
	Actor       string
	IncidentID  string
	IncidentURL string
	Detail      string
}

func (e *Engine) PDInbound(u PDUpdate) error {
	now := e.now()
	var out model.Alert
	var err error
	e.st.Write(func(d *store.Data) {
		var a *model.Alert
		for _, cand := range d.Alerts {
			if cand.PDKey == u.DedupKey {
				a = cand
			}
		}
		if a == nil {
			err = ErrNotFound
			return
		}
		if u.IncidentID != "" {
			a.PDIncidentID = u.IncidentID
		}
		if u.IncidentURL != "" {
			a.PDIncidentURL = u.IncidentURL
		}
		add := func(text string) {
			a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "pagerduty", Text: text, Author: u.Actor})
		}
		switch u.EventType {
		case "incident.triggered":
			if a.PDState == model.PDPending || a.PDState == model.PDFailed {
				a.PDState = model.PDAccepted
			}
			add("инцидент создан в PagerDuty")
		case "incident.acknowledged":
			if a.Status == model.AlertOpen {
				a.Status = model.AlertAcknowledged
				a.AckedBy = u.Actor
			}
			a.PDState = model.PDAcked
			add("подтверждена в PagerDuty")
		case "incident.resolved":
			if a.Status.Active() {
				e.resolve(a, now, "в PagerDuty", u.Actor)
			}
		case "incident.unacknowledged", "incident.reopened":
			if a.Status == model.AlertAcknowledged {
				a.Status = model.AlertOpen
			}
			a.PDState = model.PDAccepted
			add("снова открыта в PagerDuty")
		case "incident.reassigned":
			add("переназначена в PagerDuty" + detail(u.Detail))
		case "incident.escalated":
			add("эскалирована в PagerDuty" + detail(u.Detail))
		case "incident.annotated":
			add("заметка в PagerDuty" + detail(u.Detail))
		case "incident.priority_updated":
			add("приоритет изменён в PagerDuty" + detail(u.Detail))
		default:
			add(u.EventType + detail(u.Detail))
		}
		out = cloneAlert(a)
	})
	if err == nil {
		e.notify("alert", out)
	}
	return err
}

func detail(s string) string {
	if s == "" {
		return ""
	}
	return ": " + s
}

func (e *Engine) Tick() {
	now := e.now()
	var changed []model.Alert
	var cmds []PDCommand
	e.st.Write(func(d *store.Data) {
		for _, a := range d.Alerts {
			if a.Status.Active() {
				if !a.Fallback && !a.Suppressed && a.Severity.Rank() >= model.SevError.Rank() &&
					(a.PDState == model.PDPending || a.PDState == model.PDFailed) &&
					now.Sub(a.FirstSeen) >= e.FallbackAfter {
					a.Fallback = true
					a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "fallback",
						Text: fmt.Sprintf("PagerDuty не принял тревогу за %s: включено резервное оповещение по каналам в режиме «при отказе PagerDuty»", e.FallbackAfter)})
					changed = append(changed, cloneAlert(a))
				}
				if a.Suppressed && inMaintenance(d, a.CIID, serviceID(d, a), now) == nil {
					a.Suppressed = false
					a.PDState = model.PDPending
					a.PDAttemptAt = nil
					a.Timeline = append(a.Timeline, model.TimelineEntry{At: now, Kind: "maintenance", Text: "окно обслуживания закончилось, тревога активна"})
					changed = append(changed, cloneAlert(a))
				}
			}
			if c := e.retry(a, now); c != nil {
				cmds = append(cmds, *c)
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

func (e *Engine) retry(a *model.Alert, now time.Time) *PDCommand {
	if a.Suppressed {
		return nil
	}
	if a.PDAttemptAt != nil && now.Sub(*a.PDAttemptAt) < e.RetryEvery {
		return nil
	}
	delivered := a.PDState == model.PDAccepted || a.PDState == model.PDAcked
	switch {
	case a.Status == model.AlertResolved:
		if !delivered || a.PDRetry == "" {
			a.PDRetry = ""
			return nil
		}
		return e.pdCmd(a, PDResolve)
	case !delivered:
		if a.PDState != model.PDPending && a.PDState != model.PDFailed {
			return nil
		}
		return e.pdCmd(a, PDTrigger)
	case a.PDRetry == string(PDAcknowledge) && a.Status == model.AlertAcknowledged:
		return e.pdCmd(a, PDAcknowledge)
	case a.PDRetry == string(PDTrigger):
		return e.pdCmd(a, PDTrigger)
	}
	return nil
}

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

func Clone(a *model.Alert) model.Alert { return cloneAlert(a) }
