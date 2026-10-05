package model

import "time"

type Severity string

const (
	SevCritical Severity = "critical"
	SevError    Severity = "error"
	SevWarning  Severity = "warning"
	SevInfo     Severity = "info"
)

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

func (s Severity) Valid() bool { return s.Rank() > 0 }

func MaxSeverity(a, b Severity) Severity {
	if b.Rank() > a.Rank() {
		return b
	}
	return a
}

type Method string

const (
	MethodRED   Method = "red"
	MethodUSE   Method = "use"
	MethodOther Method = "other"
)

type EventStatus string

const (
	EventFiring   EventStatus = "firing"
	EventResolved EventStatus = "resolved"
)

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

type AlertStatus string

const (
	AlertOpen         AlertStatus = "open"
	AlertAcknowledged AlertStatus = "acknowledged"
	AlertResolved     AlertStatus = "resolved"
)

func (s AlertStatus) Active() bool { return s == AlertOpen || s == AlertAcknowledged }

type PDState string

const (
	PDPending  PDState = "pending"
	PDAccepted PDState = "accepted"
	PDAcked    PDState = "acked"
	PDFailed   PDState = "failed"
	PDSkipped  PDState = "skipped"
)

type TimelineEntry struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Text   string    `json:"text"`
	Author string    `json:"author,omitempty"`
}

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
	Sources    map[string]string `json:"sources"`
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

	PDRoute       string     `json:"pd_route,omitempty"`
	PDIncidentID  string     `json:"pd_incident_id,omitempty"`
	PDIncidentURL string     `json:"pd_incident_url,omitempty"`
	PDRetry       string     `json:"pd_retry,omitempty"`
	PDAttemptAt   *time.Time `json:"pd_attempt_at,omitempty"`
}

const (
	CIBusinessService = "business_service"
	CIITService       = "it_service"
	CIHost            = "host"
	CIDatabase        = "database"
	CICloudGroup      = "cloud_group"
	CIDeployment      = "deployment"
	CINetwork         = "network"
)

type Identity struct {
	Kind  string     `json:"kind"`
	Value string     `json:"value"`
	Since time.Time  `json:"since"`
	Until *time.Time `json:"until,omitempty"`
}

type Owner struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
	Role  string `json:"role,omitempty"`
	From  string `json:"from,omitempty"`
}

type CI struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Team         string            `json:"team"`
	Description  string            `json:"description,omitempty"`
	LogicalGroup string            `json:"logical_group,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Identities   []Identity        `json:"identities"`
	Owners       []Owner           `json:"owners,omitempty"`
	Origin       string            `json:"origin"`
	Source       string            `json:"source,omitempty"`
	ExternalURL  string            `json:"external_url,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    *time.Time        `json:"updated_at,omitempty"`
}

type Relation struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

type Node struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"`
	X        float64           `json:"x"`
	Y        float64           `json:"y"`
	Config   map[string]string `json:"config"`
	Disabled bool              `json:"disabled,omitempty"`
}

type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type ConnectorStatus string

const (
	ConnectorRunning ConnectorStatus = "running"
	ConnectorStopped ConnectorStatus = "stopped"
)

type Connector struct {
	ID          string          `json:"id"`
	Slug        string          `json:"slug,omitempty"`
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

type ParseError struct {
	ID          string    `json:"id"`
	ConnectorID string    `json:"connector_id"`
	Connector   string    `json:"connector"`
	Block       string    `json:"block"`
	Error       string    `json:"error"`
	Raw         string    `json:"raw"`
	At          time.Time `json:"at"`
}

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

func (m Maintenance) State(now time.Time) string {
	switch {
	case now.Before(m.Start):
		return "planned"
	case now.After(m.End):
		return "finished"
	}
	return "active"
}

type Team struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Email       string     `json:"email,omitempty"`
	Chat        string     `json:"chat,omitempty"`
	Members     []string   `json:"members"`
	Leads       []string   `json:"leads"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
	UpdatedBy   string     `json:"updated_by,omitempty"`
}
