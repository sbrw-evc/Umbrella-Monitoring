package model

import "time"

// TVBoard is a page of incidents for wall screens, opened without signing in at /tv/{slug}
// from the listed networks only. The scope (configuration items, business services, teams)
// selects incidents that match any of them; an empty scope shows every incident.
type TVBoard struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Slug       string   `json:"slug"`
	CIIDs      []string `json:"ci_ids"`
	ServiceIDs []string `json:"service_ids"`
	TeamIDs    []string `json:"team_ids"`
	// Severities limits the incidents shown; empty shows all.
	Severities       []string `json:"severities"`
	ShowAcknowledged bool     `json:"show_acknowledged"`
	ShowMaintenance  bool     `json:"show_maintenance"`
	// AllowedSources are the addresses and networks the board opens from.
	AllowedSources []string `json:"allowed_sources"`
	// Refresh is how often the screen asks for new data, in seconds.
	Refresh int `json:"refresh"`
	// Locale (en, ru) and Timezone (IANA name) of the screen; empty uses the system defaults.
	Locale    string    `json:"locale"`
	Timezone  string    `json:"timezone"`
	Theme     string    `json:"theme"`
	Disabled  bool      `json:"disabled"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedBy string    `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TVSettings: TrustedProxies are reverse proxies whose X-Forwarded-For is believed when the
// client address of a board is checked.
type TVSettings struct {
	TrustedProxies []string `json:"trusted_proxies"`
}
