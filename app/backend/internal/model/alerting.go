package model

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

const (
	PDRegionUS = "us"
	PDRegionEU = "eu"
)

// Alerting holds where alerts go: PagerDuty, backup notification and Grafana.
type Alerting struct {
	// PublicURL is the address people and PagerDuty reach Umbrella at, for links and webhooks.
	PublicURL string    `json:"public_url"`
	PagerDuty PagerDuty `json:"pagerduty"`
	Notify    Notify    `json:"notify"`
	Grafana   Grafana   `json:"grafana"`
}

// NormalizePublicURL checks the address Umbrella is reached at and drops trailing slashes. An
// empty address stays empty (not set).
func NormalizePublicURL(v string) (string, error) {
	v = strings.TrimRight(strings.TrimSpace(v), "/")
	if v == "" {
		return "", nil
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("the public address must be an http or https URL without a query")
	}
	return v, nil
}

// PagerDuty is the connection to PagerDuty. Keys and tokens are in OpenBao; only references
// are kept here.
type PagerDuty struct {
	Enabled bool   `json:"enabled"`
	Region  string `json:"region"`
	// EventsURL and APIURL replace the regional addresses (a proxy, tests).
	EventsURL             string     `json:"events_url,omitempty"`
	APIURL                string     `json:"api_url,omitempty"`
	RoutingKeyRef         string     `json:"-"`
	ServiceID             string     `json:"service_id,omitempty"`
	ServiceName           string     `json:"service_name,omitempty"`
	APITokenRef           string     `json:"-"`
	WebhookSecretRef      string     `json:"-"`
	WebhookSubscriptionID string     `json:"webhook_subscription_id,omitempty"`
	MinSeverity           string     `json:"min_severity"`
	Routes                []PDRoute  `json:"routes"`
	UpdatedAt             *time.Time `json:"updated_at,omitempty"`
	UpdatedBy             string     `json:"updated_by,omitempty"`
}

func (s PagerDuty) Events() string {
	if s.EventsURL != "" {
		return s.EventsURL
	}
	if s.Region == PDRegionEU {
		return "https://events.eu.pagerduty.com/v2/enqueue"
	}
	return "https://events.pagerduty.com/v2/enqueue"
}

func (s PagerDuty) API() string {
	if s.APIURL != "" {
		return s.APIURL
	}
	if s.Region == PDRegionEU {
		return "https://api.eu.pagerduty.com"
	}
	return "https://api.pagerduty.com"
}

// PDRoute sends the alerts of a team or a business service to a PagerDuty service of their
// own. Empty TeamID or ServiceID match any.
type PDRoute struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	TeamID        string `json:"team_id,omitempty"`
	ServiceID     string `json:"service_id,omitempty"`
	RoutingKeyRef string `json:"-"`
	PDServiceID   string `json:"pd_service_id,omitempty"`
	PDServiceName string `json:"pd_service_name,omitempty"`
}

// Notify is backup notification: when nobody has taken a severe enough alert (PagerDuty is
// off or did not take it in time), the people of its route get it by e-mail and Telegram, and
// a follow-up once it is acknowledged or resolved.
type Notify struct {
	Email    EmailChannel    `json:"email"`
	Telegram TelegramChannel `json:"telegram"`
	// DelaySeconds is how long an open alert waits before backup notification; 0 sends it at
	// once. Nil is automatic: 2 minutes while PagerDuty is on (time for it to take the alert),
	// at once while it is off.
	DelaySeconds *int `json:"delay_seconds"`
	// MinSeverity is the lowest severity that goes to backup notification; empty is error.
	MinSeverity string `json:"min_severity"`
	// Extra recipients that always get backup notification (a duty mailbox, a group chat).
	ExtraEmails   []string   `json:"extra_emails"`
	ExtraTelegram []string   `json:"extra_telegram"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
	UpdatedBy     string     `json:"updated_by,omitempty"`
}

const (
	SMTPStartTLS = "starttls"
	SMTPTLS      = "tls"
	SMTPNone     = "none"
)

type EmailChannel struct {
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Security    string `json:"security"`
	SkipVerify  bool   `json:"skip_verify"`
	Username    string `json:"username"`
	PasswordRef string `json:"-"`
	From        string `json:"from"`
}

type TelegramChannel struct {
	Enabled  bool   `json:"enabled"`
	TokenRef string `json:"-"`
	// APIURL replaces https://api.telegram.org (a proxy, tests).
	APIURL string `json:"api_url,omitempty"`
}

// Grafana opens the context of an incident: a dashboard with the time around the incident and
// variables for the item, the service, the team and the incident.
type Grafana struct {
	// DashboardURL is the dashboard address, for example https://grafana/d/abc/incident; its own
	// query (an organization, fixed variables) is kept.
	DashboardURL string `json:"dashboard_url"`
	// WindowMinute is how much time before the incident and after its end is shown.
	WindowMinute int        `json:"window_minutes"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	UpdatedBy    string     `json:"updated_by,omitempty"`
}
