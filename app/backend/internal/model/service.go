package model

import (
	"slices"
	"time"
)

const (
	CriticalityCritical = "critical"
	CriticalityHigh     = "high"
	CriticalityMedium   = "medium"
	CriticalityLow      = "low"

	ServiceActive  = "active"
	ServicePlanned = "planned"
	ServiceRetired = "retired"
)

var (
	Criticalities   = []string{CriticalityCritical, CriticalityHigh, CriticalityMedium, CriticalityLow}
	ServiceStatuses = []string{ServiceActive, ServicePlanned, ServiceRetired}
)

func ValidCriticality(v string) bool { return slices.Contains(Criticalities, v) }

func ValidServiceStatus(v string) bool { return slices.Contains(ServiceStatuses, v) }

type Link struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type Service struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	OwnerTeamID string    `json:"owner_team_id"`
	TeamIDs     []string  `json:"team_ids"`
	Criticality string    `json:"criticality"`
	Status      string    `json:"status"`
	Tags        []string  `json:"tags"`
	Links       []Link    `json:"links"`
	DependsOn   []string  `json:"depends_on"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s Service) Involves(teams map[string]bool) bool {
	if teams[s.OwnerTeamID] {
		return true
	}
	for _, id := range s.TeamIDs {
		if teams[id] {
			return true
		}
	}
	return false
}
