package model

import (
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/directory"
)

const (
	SourceLocal = "local"
	SourceLDAP  = "ldap"

	RoleAdmin = "admin"
	RoleUser  = "user"

	ThemeLight = "light"
	ThemeDark  = "dark"

	LocaleEN = "en"
	LocaleRU = "ru"
)

func ValidTheme(v string) bool  { return v == ThemeLight || v == ThemeDark }
func ValidLocale(v string) bool { return v == LocaleEN || v == LocaleRU }

type User struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	Name         string     `json:"name"`
	Email        string     `json:"email,omitempty"`
	Source       string     `json:"source"`
	Role         string     `json:"role"`
	PasswordHash string     `json:"-"`
	Disabled     bool       `json:"disabled"`
	CreatedAt    time.Time  `json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

type Settings struct {
	DefaultTheme  string           `json:"default_theme"`
	DefaultLocale string           `json:"default_locale"`
	LDAP          directory.Config `json:"ldap"`
	SetupAt       time.Time        `json:"setup_at"`
	SetupBy       string           `json:"setup_by"`
}
