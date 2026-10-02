package model

import "time"

// Permissions a role can grant. The UI hides what the user lacks; the API
// checks every call.
const (
	PermIncidentsView   = "incidents.view"   // incident list, card, heatmap, ops view
	PermIncidentsAct    = "incidents.act"    // ack, resolve, comment, bulk actions
	PermCMDBView        = "cmdb.view"        // CMDB map and CI cards
	PermCMDBEdit        = "cmdb.edit"        // create CIs and relations
	PermConnectorsView  = "connectors.view"  // connector list and graphs
	PermConnectorsEdit  = "connectors.edit"  // build, publish, start, stop, delete
	PermEventsView      = "events.view"      // raw events and parse errors
	PermMaintenanceEdit = "maintenance.edit" // plan and delete maintenance windows
	PermRulesView       = "rules.view"       // RED/USE rules
	PermNotifyEdit      = "notify.edit"      // notification channels (Teams, Zoom)
	PermSelfcheckView   = "selfcheck.view"   // self-check page
	PermSelfcheckAdmin  = "selfcheck.admin"  // PagerDuty outage drill
	PermAuditView       = "audit.view"       // audit log
	PermUsersAdmin      = "users.admin"      // users, roles, API tokens of others
)

// AllPermissions lists every permission in UI order.
var AllPermissions = []string{
	PermIncidentsView, PermIncidentsAct, PermCMDBView, PermCMDBEdit,
	PermConnectorsView, PermConnectorsEdit, PermEventsView, PermMaintenanceEdit,
	PermRulesView, PermNotifyEdit, PermSelfcheckView, PermSelfcheckAdmin,
	PermAuditView, PermUsersAdmin,
}

// Role is a named set of permissions. AllServices lets its users see every
// incident; otherwise they see only the business services bound to them and
// everything those services depend on.
type Role struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Permissions []string  `json:"permissions"`
	AllServices bool      `json:"all_services"`
	BuiltIn     bool      `json:"built_in"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// User is a local account.
type User struct {
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	Name               string     `json:"name"`
	Email              string     `json:"email,omitempty"`
	Roles              []string   `json:"roles"`
	BusinessServices   []string   `json:"business_services"` // CI ids of business services
	Disabled           bool       `json:"disabled"`
	Service            bool       `json:"service"` // service account: API tokens only, no password sign-in
	MustChangePassword bool       `json:"must_change_password"`
	CreatedAt          time.Time  `json:"created_at"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	PasswordChangedAt  *time.Time `json:"password_changed_at,omitempty"`
	LockedUntil        *time.Time `json:"locked_until,omitempty"`

	PasswordHash string `json:"-"`
	FailedLogins int    `json:"-"`
}

// APIToken lets scripts and Grafana call the API as a user. Only the SHA-256
// of the token is kept; the token itself is shown once.
type APIToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"` // first characters, to recognise the token
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`

	Hash string `json:"-"`
}

// Channel types.
const (
	ChannelTeams = "teams" // Microsoft Teams incoming webhook (Workflows), Adaptive Card
	ChannelZoom  = "zoom"  // Zoom Team Chat incoming webhook
)

// Channel modes.
const (
	ChannelAlways   = "always"   // every selected event
	ChannelFallback = "fallback" // only when PagerDuty did not take the incident
)

// Notification events.
const (
	NotifyOpen     = "open"
	NotifyEscalate = "escalate" // severity went up
	NotifyAck      = "ack"
	NotifyResolve  = "resolve"
	NotifyFallback = "fallback" // PagerDuty did not accept in time
)

// Channel is a messenger webhook that receives incident notifications.
type Channel struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	Mode        string     `json:"mode"`
	Enabled     bool       `json:"enabled"`
	MinSeverity Severity   `json:"min_severity"`
	Events      []string   `json:"events"`
	Services    []string   `json:"services"`            // business or IT service CI ids; empty = all
	URLRef      string     `json:"url_ref,omitempty"`   // openbao://... reference (target design)
	TokenRef    string     `json:"token_ref,omitempty"` // Zoom verification token reference
	URLSet      bool       `json:"url_set"`             // a direct URL is stored (write-only)
	TokenSet    bool       `json:"token_set"`
	URLHint     string     `json:"url_hint,omitempty"` // host part, for display
	Sent        int        `json:"sent"`
	Failed      int        `json:"failed"`
	LastStatus  string     `json:"last_status,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
	LastAt      *time.Time `json:"last_at,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
	UpdatedBy   string     `json:"updated_by"`

	URL   string `json:"-"`
	Token string `json:"-"`
}

// Delivery is one notification attempt series.
type Delivery struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	Channel   string    `json:"channel"`
	AlertID   string    `json:"alert_id,omitempty"`
	Event     string    `json:"event"`
	OK        bool      `json:"ok"`
	Status    int       `json:"status"`
	Attempts  int       `json:"attempts"`
	Error     string    `json:"error,omitempty"`
	At        time.Time `json:"at"`
}
