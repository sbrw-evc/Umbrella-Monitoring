// Package model holds the Umbrella domain types shared by the API, the
// pipeline and the alert engine.
package model

import "time"

// Severity follows the four PagerDuty levels.
type Severity string

const (
	SevCritical Severity = "critical"
	SevError    Severity = "error"
	SevWarning  Severity = "warning"
	SevInfo     Severity = "info"
)

// Rank orders severities: a higher rank is worse.
func (s Severity) Rank() int {
	switch s {
	case SevCritical:
		return 4
	case SevError:
		return 3
	case SevWarning:
		return 2
	case SevInfo:
		return 1
	}
	return 0
}

// Valid reports whether s is one of the four known levels.
func (s Severity) Valid() bool { return s.Rank() > 0 }

// MaxSeverity returns the worse of a and b.
func MaxSeverity(a, b Severity) Severity {
	if b.Rank() > a.Rank() {
		return b
	}
	return a
}

// Method marks how an alert was formed.
type Method string

const (
	MethodRED   Method = "red"
	MethodUSE   Method = "use"
	MethodOther Method = "other"
)

// EventStatus is what the source says about the signal.
type EventStatus string

const (
	EventFiring   EventStatus = "firing"
	EventResolved EventStatus = "resolved"
)

// Event is one immutable signal from a source after normalization.
type Event struct {
	ID          string            `json:"id"`
	ConnectorID string            `json:"connector_id"`
	Source      string            `json:"source"`
	ExternalID  string            `json:"external_id"`
	CIName      string            `json:"ci_name"`
	CIID        string            `json:"ci_id,omitempty"`
	Signal      string            `json:"signal"`
	Method      Method            `json:"method"`
	Severity    Severity          `json:"severity"`
	Status      EventStatus       `json:"status"`
	Title       string            `json:"title"`
	Value       string            `json:"value,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Raw         string            `json:"raw"`
	ReceivedAt  time.Time         `json:"received_at"`
	AlertID     string            `json:"alert_id,omitempty"`
	Suppressed  bool              `json:"suppressed,omitempty"`
}

// AlertStatus is the lifecycle of an alert (incident).
type AlertStatus string

const (
	AlertOpen         AlertStatus = "open"
	AlertAcknowledged AlertStatus = "acknowledged"
	AlertResolved     AlertStatus = "resolved"
)

// Active reports whether the alert still needs attention.
func (s AlertStatus) Active() bool { return s == AlertOpen || s == AlertAcknowledged }

// PDState is the delivery state of an alert in PagerDuty.
type PDState string

const (
	PDPending  PDState = "pending"  // queued in the outbox
	PDAccepted PDState = "accepted" // Events API answered 202
	PDAcked    PDState = "acked"    // acknowledged in PagerDuty
	PDFailed   PDState = "failed"   // not accepted after retries
	PDSkipped  PDState = "skipped"  // not sent (suppressed or below threshold)
)

// TimelineEntry is one line of the incident history.
type TimelineEntry struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"` // event, status, pagerduty, comment, maintenance
	Text   string    `json:"text"`
	Author string    `json:"author,omitempty"`
}

// Alert groups every event with the same dedup key. In the MVP an incident
// on the dashboard is an alert sent to PagerDuty.
type Alert struct {
	ID         string            `json:"id"`
	DedupKey   string            `json:"dedup_key"`
	Title      string            `json:"title"`
	CIID       string            `json:"ci_id,omitempty"`
	CIName     string            `json:"ci_name"`
	CIType     string            `json:"ci_type,omitempty"`
	Service    string            `json:"service,omitempty"`
	Team       string            `json:"team,omitempty"`
	Signal     string            `json:"signal"`
	Method     Method            `json:"method"`
	Severity   Severity          `json:"severity"`
	Status     AlertStatus       `json:"status"`
	Sources    map[string]string `json:"sources"` // connector -> firing|resolved
	Count      int               `json:"count"`
	FirstSeen  time.Time         `json:"first_seen"`
	LastSeen   time.Time         `json:"last_seen"`
	ResolvedAt *time.Time        `json:"resolved_at,omitempty"`
	AckedBy    string            `json:"acked_by,omitempty"`
	Suppressed bool              `json:"suppressed"`
	PDState    PDState           `json:"pd_state"`
	PDKey      string            `json:"pd_dedup_key"`
	PDError    string            `json:"pd_error,omitempty"`
	Fallback   bool              `json:"fallback"`
	RelatedID  string            `json:"related_id,omitempty"`
	Timeline   []TimelineEntry   `json:"timeline"`
}

// CIType values used by the demo model; the set is open.
const (
	CIBusinessService = "business_service"
	CIITService       = "it_service"
	CIHost            = "host"
	CIDatabase        = "database"
	CICloudGroup      = "cloud_group"
	CIDeployment      = "deployment"
	CINetwork         = "network"
)

// Identity is one way a source can name a CI (host name, cloud instance ID,
// IP). Cloud instance IDs change after reboot, so they are history, not the
// identity of the CI.
type Identity struct {
	Kind  string     `json:"kind"`
	Value string     `json:"value"`
	Since time.Time  `json:"since"`
	Until *time.Time `json:"until,omitempty"`
}

// CI is a configuration item of the CMDB map.
type CI struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Team         string            `json:"team"`
	Description  string            `json:"description,omitempty"`
	LogicalGroup string            `json:"logical_group,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Identities   []Identity        `json:"identities"`
	Origin       string            `json:"origin"` // manual, discovery
	CreatedAt    time.Time         `json:"created_at"`
}

// Relation links two CIs: From depends on To.
type Relation struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"` // depends_on, runs_on, part_of
}

// Node is one block of a low-code graph (connector).
type Node struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"` // block kind, see pipeline.Blocks
	X        float64           `json:"x"`
	Y        float64           `json:"y"`
	Config   map[string]string `json:"config"`
	Disabled bool              `json:"disabled,omitempty"`
}

// Edge connects two nodes.
type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

// Graph is a versioned low-code definition.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// ConnectorStatus is whether the runtime executes the connector.
type ConnectorStatus string

const (
	ConnectorRunning ConnectorStatus = "running"
	ConnectorStopped ConnectorStatus = "stopped"
)

// Connector is a source connection assembled in the block builder.
type Connector struct {
	ID          string          `json:"id"`
	Slug        string          `json:"slug,omitempty"` // stable ingest name: /api/ingest/<slug>
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Team        string          `json:"team"`
	Status      ConnectorStatus `json:"status"`
	Version     int             `json:"version"`
	Draft       Graph           `json:"draft"`
	Published   *Graph          `json:"published,omitempty"`
	DraftDirty  bool            `json:"draft_dirty"`
	SampleInput string          `json:"sample_input,omitempty"`
	UpdatedAt   time.Time       `json:"updated_at"`
	UpdatedBy   string          `json:"updated_by"`
	EventsTotal int             `json:"events_total"`
	ErrorsTotal int             `json:"errors_total"`
	LastEventAt *time.Time      `json:"last_event_at,omitempty"`
}

// ParseError is an event the connector could not parse (events.dlq).
type ParseError struct {
	ID          string    `json:"id"`
	ConnectorID string    `json:"connector_id"`
	Connector   string    `json:"connector"`
	Block       string    `json:"block"`
	Error       string    `json:"error"`
	Raw         string    `json:"raw"`
	At          time.Time `json:"at"`
}

// Maintenance is a window during which alerts on a CI are suppressed.
type Maintenance struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CIID      string    `json:"ci_id"`
	CIName    string    `json:"ci_name"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// State returns planned, active or finished at moment now.
func (m Maintenance) State(now time.Time) string {
	switch {
	case now.Before(m.Start):
		return "planned"
	case now.After(m.End):
		return "finished"
	}
	return "active"
}

// Rule is a RED or USE alerting rule.
type Rule struct {
	ID        string `json:"id"`
	Method    Method `json:"method"`
	Signal    string `json:"signal"`
	Name      string `json:"name"`
	Condition string `json:"condition"`
	AppliesTo string `json:"applies_to"`
	Severity  Severity `json:"severity"`
	Enabled   bool   `json:"enabled"`
}

// Team is a directory group that scopes what a user sees.
type Team struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
