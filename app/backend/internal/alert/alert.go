// Package alert turns connector events into alerts: one alert per configuration item and
// signal, folded across sources and connectors, with a lifecycle (open, acknowledged,
// resolved), routing to the owning team, maintenance windows, delivery to PagerDuty and
// backup notification when PagerDuty does not take the alert.
package alert

import (
	"maps"
	"slices"
	"time"
)

const (
	StatusOpen         = "open"
	StatusAcknowledged = "acknowledged"
	StatusResolved     = "resolved"

	// PagerDuty delivery states of an alert.
	PDPending  = "pending"
	PDAccepted = "accepted"
	PDAcked    = "acked"
	PDFailed   = "failed"
	PDSkipped  = "skipped"

	SourceFiring   = "firing"
	SourceResolved = "resolved"

	// Route.Via: where the people of the route come from.
	ViaService  = "service"
	ViaCIOwners = "ci_owners"
	ViaNone     = "none"
)

var severityRank = map[string]int{"critical": 4, "error": 3, "warning": 2, "info": 1}

// SeverityRank orders severities; unknown ones rank 0.
func SeverityRank(s string) int { return severityRank[s] }

func Active(status string) bool { return status == StatusOpen || status == StatusAcknowledged }

// Source is one event of one connector that feeds the alert.
type Source struct {
	ConnectorID string    `json:"connector_id"`
	Key         string    `json:"key"`
	Status      string    `json:"status"`
	Severity    string    `json:"severity"`
	Title       string    `json:"title"`
	Value       string    `json:"value,omitempty"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
}

type Ref struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Person is someone the alert is routed to, with the contacts backup notification uses.
type Person struct {
	UserID   string `json:"user_id"`
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	Telegram string `json:"telegram,omitempty"`
	// Role: lead or member of the team, or the NetBox role of a CI owner.
	Role string `json:"role,omitempty"`
}

// Route is who the alert belongs to: the business services of its configuration item, the
// owning team of the most critical one and its people. Owners are the people responsible for
// the item in NetBox; they get the alert when the team has nobody.
type Route struct {
	Services []Ref     `json:"services"`
	Team     *Ref      `json:"team,omitempty"`
	People   []Person  `json:"people"`
	Owners   []Person  `json:"owners"`
	Via      string    `json:"via"`
	At       time.Time `json:"at"`
}

// Recipients are the people backup notification goes to.
func (r Route) Recipients() []Person {
	if len(r.People) > 0 {
		return r.People
	}
	return r.Owners
}

func (r Route) ServiceIDs() []string {
	out := make([]string, 0, len(r.Services))
	for _, s := range r.Services {
		out = append(out, s.ID)
	}
	return out
}

// PD is the PagerDuty side of an alert.
type PD struct {
	State string `json:"state"`
	// Key is the dedup_key of the Events API: umb-<alert id>.
	Key         string     `json:"key"`
	Route       string     `json:"route,omitempty"`
	Error       string     `json:"error,omitempty"`
	Retry       string     `json:"retry,omitempty"`
	AttemptAt   *time.Time `json:"attempt_at,omitempty"`
	IncidentID  string     `json:"incident_id,omitempty"`
	IncidentURL string     `json:"incident_url,omitempty"`
}

type Alert struct {
	ID       string `json:"id"`
	DedupKey string `json:"dedup_key"`
	Title    string `json:"title"`
	// CIID is empty while the item named by the events is not in the catalog; CIName keeps the
	// name the events use.
	CIID      string             `json:"ci_id,omitempty"`
	CIName    string             `json:"ci_name"`
	CIKind    string             `json:"ci_kind,omitempty"`
	Signal    string             `json:"signal"`
	Method    string             `json:"method"`
	Severity  string             `json:"severity"`
	Status    string             `json:"status"`
	Sources   map[string]*Source `json:"sources"`
	Labels    map[string]string  `json:"labels"`
	Count     int                `json:"count"`
	FirstSeen time.Time          `json:"first_seen"`
	// OpenedAt is when the alert last opened: the first event or a reopening in the window.
	OpenedAt   time.Time  `json:"opened_at"`
	LastSeen   time.Time  `json:"last_seen"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy string     `json:"resolved_by,omitempty"`
	AckedBy    string     `json:"acked_by,omitempty"`
	AckedAt    *time.Time `json:"acked_at,omitempty"`
	// Suppressed: a maintenance window covers the item or its service; nothing is sent.
	Suppressed    bool       `json:"suppressed"`
	MaintenanceID string     `json:"maintenance_id,omitempty"`
	Route         Route      `json:"route"`
	PD            PD         `json:"pd"`
	Fallback      bool       `json:"fallback"`
	FallbackAt    *time.Time `json:"fallback_at,omitempty"`
	RelatedID     string     `json:"related_id,omitempty"`
}

func (a *Alert) Clone() Alert {
	c := *a
	c.Sources = make(map[string]*Source, len(a.Sources))
	for k, v := range a.Sources {
		s := *v
		c.Sources[k] = &s
	}
	c.Labels = maps.Clone(a.Labels)
	c.Route.Services = slices.Clone(a.Route.Services)
	c.Route.People = slices.Clone(a.Route.People)
	c.Route.Owners = slices.Clone(a.Route.Owners)
	return c
}

// Entry is a line of the alert timeline. Code and Args are translated by the interface.
type Entry struct {
	ID     int64             `json:"id"`
	At     time.Time         `json:"at"`
	Kind   string            `json:"kind"`
	Code   string            `json:"code"`
	Args   map[string]string `json:"args,omitempty"`
	Author string            `json:"author,omitempty"`
}

// Timeline kinds.
const (
	KindStatus      = "status"
	KindEvent       = "event"
	KindPagerDuty   = "pagerduty"
	KindMaintenance = "maintenance"
	KindFallback    = "fallback"
	KindComment     = "comment"
	KindRoute       = "route"
)

// Action is what the PagerDuty Events API is asked to do with an alert.
type Action string

const (
	PDTrigger     Action = "trigger"
	PDAcknowledge Action = "acknowledge"
	PDResolve     Action = "resolve"
)

// Command asks the PagerDuty gateway to deliver an action for a copy of the alert.
type Command struct {
	Action Action
	Alert  Alert
}

// Sender delivers commands to PagerDuty and reports the outcome back with Engine.PDResult.
type Sender interface {
	Send(cmd Command)
}

// Notifier sends backup notification for an alert PagerDuty did not take.
type Notifier interface {
	Fallback(a Alert)
}

// Incoming is an event handed to the engine.
type Incoming struct {
	ConnectorID string
	Key         string
	Title       string
	CI          string
	Signal      string
	Method      string
	Severity    string
	Status      string
	Value       string
	Labels      map[string]string
}
