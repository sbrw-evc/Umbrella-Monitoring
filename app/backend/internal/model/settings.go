package model

import "time"

const (
	PDRegionUS = "us"
	PDRegionEU = "eu"
)

type PDSettings struct {
	Enabled               bool       `json:"enabled"`
	Region                string     `json:"region"`
	EventsURL             string     `json:"events_url,omitempty"`
	APIURL                string     `json:"api_url,omitempty"`
	RoutingKeyRef         string     `json:"routing_key_ref,omitempty"`
	ServiceID             string     `json:"service_id,omitempty"`
	ServiceName           string     `json:"service_name,omitempty"`
	APITokenRef           string     `json:"api_token_ref,omitempty"`
	WebhookSecretRef      string     `json:"webhook_secret_ref,omitempty"`
	WebhookSubscriptionID string     `json:"webhook_subscription_id,omitempty"`
	MinSeverity           Severity   `json:"min_severity"`
	EscalationPolicies    []string   `json:"escalation_policies"`
	Routes                []PDRoute  `json:"routes"`
	UpdatedAt             *time.Time `json:"updated_at,omitempty"`
	UpdatedBy             string     `json:"updated_by,omitempty"`
}

func (s PDSettings) Events() string {
	if s.EventsURL != "" {
		return s.EventsURL
	}
	if s.Region == PDRegionEU {
		return "https://events.eu.pagerduty.com/v2/enqueue"
	}
	return "https://events.pagerduty.com/v2/enqueue"
}

func (s PDSettings) API() string {
	if s.APIURL != "" {
		return s.APIURL
	}
	if s.Region == PDRegionEU {
		return "https://api.eu.pagerduty.com"
	}
	return "https://api.pagerduty.com"
}

type PDRoute struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Team          string `json:"team,omitempty"`
	Service       string `json:"service,omitempty"`
	RoutingKeyRef string `json:"routing_key_ref"`
	ServiceID     string `json:"service_id,omitempty"`
	ServiceName   string `json:"service_name,omitempty"`
}

type OnCallEntry struct {
	PolicyID   string     `json:"policy_id"`
	PolicyName string     `json:"policy_name"`
	Level      int        `json:"level"`
	UserID     string     `json:"user_id"`
	UserName   string     `json:"user_name"`
	Email      string     `json:"email,omitempty"`
	Schedule   string     `json:"schedule,omitempty"`
	Start      *time.Time `json:"start,omitempty"`
	End        *time.Time `json:"end,omitempty"`
}

type OnCall struct {
	Entries  []OnCallEntry `json:"entries"`
	SyncedAt *time.Time    `json:"synced_at,omitempty"`
	Error    string        `json:"error,omitempty"`
}

type Settings struct {
	GrafanaURL     string     `json:"grafana_url"`
	DefaultTheme   string     `json:"default_theme"`
	DefaultLocale  string     `json:"default_locale"`
	SetupCompleted bool       `json:"setup_completed"`
	SetupAt        *time.Time `json:"setup_at,omitempty"`
	SetupBy        string     `json:"setup_by,omitempty"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
	UpdatedBy      string     `json:"updated_by,omitempty"`
}

type Integration struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	Team            string            `json:"team"`
	URL             string            `json:"url"`
	AuthType        string            `json:"auth_type"`
	Username        string            `json:"username,omitempty"`
	SecretRef       string            `json:"secret_ref,omitempty"`
	WebhookTokenRef string            `json:"webhook_token_ref,omitempty"`
	TLSSkipVerify   bool              `json:"tls_skip_verify"`
	Params          map[string]string `json:"params"`
	ConnectorID     string            `json:"connector_id"`
	LastCheckAt     *time.Time        `json:"last_check_at,omitempty"`
	LastCheckOK     bool              `json:"last_check_ok"`
	LastCheck       string            `json:"last_check,omitempty"`
	SetupAt         *time.Time        `json:"setup_at,omitempty"`
	SetupInfo       string            `json:"setup_info,omitempty"`
	Remote          map[string]string `json:"remote,omitempty"`
	SyncedAt        *time.Time        `json:"synced_at,omitempty"`
	SyncOK          bool              `json:"sync_ok"`
	SyncInfo        string            `json:"sync_info,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	UpdatedBy       string            `json:"updated_by"`
}

type Rule struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	Method       Method                 `json:"method"`
	Signal       string                 `json:"signal"`
	SourceID     string                 `json:"source_id"`
	Query        string                 `json:"query"`
	CILabel      string                 `json:"ci_label"`
	ServiceLabel string                 `json:"service_label,omitempty"`
	Op           string                 `json:"op"`
	Threshold    float64                `json:"threshold"`
	For          string                 `json:"for"`
	Interval     string                 `json:"interval"`
	Severity     Severity               `json:"severity"`
	Title        string                 `json:"title"`
	Team         string                 `json:"team,omitempty"`
	Enabled      bool                   `json:"enabled"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	UpdatedBy    string                 `json:"updated_by"`
	LastEvalAt   *time.Time             `json:"last_eval_at,omitempty"`
	LastError    string                 `json:"last_error,omitempty"`
	SeriesCount  int                    `json:"series"`
	Pending      int                    `json:"pending"`
	Firing       int                    `json:"firing"`
	State        map[string]*RuleSeries `json:"state,omitempty"`
}

type RuleSeries struct {
	CI       string            `json:"ci"`
	Labels   map[string]string `json:"labels"`
	Value    float64           `json:"value"`
	Since    time.Time         `json:"since"`
	Firing   bool              `json:"firing"`
	FiredAt  *time.Time        `json:"fired_at,omitempty"`
	LastSeen time.Time         `json:"last_seen"`
}
