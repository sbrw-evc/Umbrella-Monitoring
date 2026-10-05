package model

import "time"

const (
	PermIncidentsView   = "incidents.view"
	PermIncidentsAct    = "incidents.act"
	PermCMDBView        = "cmdb.view"
	PermCMDBEdit        = "cmdb.edit"
	PermConnectorsView  = "connectors.view"
	PermConnectorsEdit  = "connectors.edit"
	PermEventsView      = "events.view"
	PermMaintenanceEdit = "maintenance.edit"
	PermRulesView       = "rules.view"
	PermRulesEdit       = "rules.edit"
	PermNotifyEdit      = "notify.edit"
	PermIntegrations    = "integrations.edit"
	PermSelfcheckView   = "selfcheck.view"
	PermAuditView       = "audit.view"
	PermUsersAdmin      = "users.admin"
)

var AllPermissions = []string{
	PermIncidentsView, PermIncidentsAct, PermCMDBView, PermCMDBEdit,
	PermConnectorsView, PermConnectorsEdit, PermEventsView, PermMaintenanceEdit,
	PermRulesView, PermRulesEdit, PermNotifyEdit, PermIntegrations, PermSelfcheckView,
	PermAuditView, PermUsersAdmin,
}

type Role struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Permissions []string  `json:"permissions"`
	AllServices bool      `json:"all_services"`
	BuiltIn     bool      `json:"built_in"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type User struct {
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	Name               string     `json:"name"`
	Email              string     `json:"email,omitempty"`
	Roles              []string   `json:"roles"`
	BusinessServices   []string   `json:"business_services"`
	Disabled           bool       `json:"disabled"`
	Service            bool       `json:"service"`
	MustChangePassword bool       `json:"must_change_password"`
	CreatedAt          time.Time  `json:"created_at"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	PasswordChangedAt  *time.Time `json:"password_changed_at,omitempty"`
	LockedUntil        *time.Time `json:"locked_until,omitempty"`

	PasswordHash string `json:"-"`
	FailedLogins int    `json:"-"`
}

type APIToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`

	Hash string `json:"-"`
}

const (
	ChannelTeams = "teams"
	ChannelZoom  = "zoom"
)

const (
	ChannelAlways   = "always"
	ChannelFallback = "fallback"
)

const (
	NotifyOpen     = "open"
	NotifyEscalate = "escalate"
	NotifyAck      = "ack"
	NotifyResolve  = "resolve"
	NotifyFallback = "fallback"
)

type Channel struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	Mode        string     `json:"mode"`
	Enabled     bool       `json:"enabled"`
	MinSeverity Severity   `json:"min_severity"`
	Events      []string   `json:"events"`
	Services    []string   `json:"services"`
	URLRef      string     `json:"url_ref,omitempty"`
	TokenRef    string     `json:"token_ref,omitempty"`
	URLHint     string     `json:"url_hint,omitempty"`
	Sent        int        `json:"sent"`
	Failed      int        `json:"failed"`
	LastStatus  string     `json:"last_status,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
	LastAt      *time.Time `json:"last_at,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
	UpdatedBy   string     `json:"updated_by"`
}

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
