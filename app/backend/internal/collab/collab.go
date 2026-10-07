// Package collab is what incident response needs from collaboration tools: a chat for the war
// room, conference bridges and an issue tracker. The providers (Microsoft Teams through Graph,
// Zoom, Jira Cloud) implement the interfaces; a nil one is not configured.
package collab

import (
	"context"
	"errors"
	"time"
)

// ErrNotConfigured: the provider is turned off or misses settings.
var ErrNotConfigured = errors.New("the integration is not configured")

// Incident is what providers are told about an incident.
type Incident struct {
	ID       string
	Title    string
	Severity string // critical, error, warning, low, info
	Priority string // P1..P5
	Status   string // open, acknowledged, resolved
	CI       string
	Signal   string
	Service  string   // primary business service
	Services []string // all affected services
	Team     string
	Impact   string // business impact level, empty when none
	OpenedAt time.Time
	// URL opens the incident in Umbrella; GrafanaURL its context dashboard (both may be empty).
	URL        string
	GrafanaURL string
	// Summary is a plain-text description: what happened, impact and the latest timeline.
	Summary string
}

// Person is someone to add to a chat or meeting, found by e-mail (the user principal name).
type Person struct {
	Name  string
	Email string
}

// Link is a chat or a meeting: ID for later calls, URL for people.
type Link struct {
	ID  string
	URL string
}

// Chat makes war rooms: a group chat for the incident.
type Chat interface {
	// CreateWarRoom makes the chat with the members and posts the first message (HTML).
	CreateWarRoom(ctx context.Context, inc Incident, members []Person, html string) (Link, error)
	// Post adds a message (HTML) to the chat.
	Post(ctx context.Context, chatID, html string) error
	// AddMembers adds people to the chat; people already in it are skipped.
	AddMembers(ctx context.Context, chatID string, members []Person) error
}

// Meetings makes conference bridges (an online meeting people join by a link).
type Meetings interface {
	// Kind names the provider: teams or zoom.
	Kind() string
	CreateMeeting(ctx context.Context, inc Incident) (Link, error)
}

// IssueSpec is an issue to create. Description is plain text; the tracker formats it.
type IssueSpec struct {
	Kind        string // resolution or postmortem
	Project     string // project key
	IssueType   string // issue type name
	Priority    string // priority name, empty: the project default
	Labels      []string
	Summary     string
	Description string
	// DueDays sets the due date this many days from now; 0: none.
	DueDays int
}

// Tracker files issues (Jira Cloud).
type Tracker interface {
	// CreateIssue returns the key in ID and the browse URL.
	CreateIssue(ctx context.Context, spec IssueSpec) (Link, error)
	Comment(ctx context.Context, key, text string) error
	// Transition moves the issue by the name of a transition or of the target status.
	Transition(ctx context.Context, key, name string) error
	// LinkIssues links two issues with a link type name (for example "Relates").
	LinkIssues(ctx context.Context, from, to, linkType string) error
}

// Registry holds the configured providers; nil fields are not configured.
type Registry struct {
	Chat     Chat
	Meetings map[string]Meetings // by Kind
	Tracker  Tracker
}
