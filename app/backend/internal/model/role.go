package model

import "time"

type Role struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Permissions []string  `json:"permissions"`
	System      bool      `json:"system"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func SystemRoles(now time.Time) []*Role {
	return []*Role{
		{ID: RoleAdmin, Name: "Administrator", System: true, CreatedAt: now, UpdatedAt: now},
		{ID: RoleUser, Name: "User", System: true, Permissions: []string{}, CreatedAt: now, UpdatedAt: now},
	}
}
