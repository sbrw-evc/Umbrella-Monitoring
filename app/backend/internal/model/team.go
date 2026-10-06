package model

import "time"

// MaxTeamDepth is the deepest a team hierarchy goes: a team and its ancestors are at most this
// many teams.
const MaxTeamDepth = 8

type Team struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ParentID    string `json:"parent_id"`
	LeadID      string `json:"lead_id"`
	// Email and Telegram are the team's own channel (a duty mailbox, a group chat). With either
	// set, backup notification goes to the channel and the lead instead of every member.
	Email     string    `json:"email"`
	Telegram  string    `json:"telegram"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
