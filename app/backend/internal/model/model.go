package model

import (
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/directory"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/entra"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/netbox"
)

const (
	SourceLocal = "local"
	SourceLDAP  = "ldap"
	SourceEntra = "entra"

	RoleAdmin = "admin"
	RoleUser  = "user"

	ThemeLight = "light"
	ThemeDark  = "dark"

	LocaleEN = "en"
	LocaleRU = "ru"
)

func ValidTheme(v string) bool  { return v == ThemeLight || v == ThemeDark }
func ValidLocale(v string) bool { return v == LocaleEN || v == LocaleRU }

func ValidTimezone(v string) bool {
	if v == "" || v == "Local" || len(v) > 64 || strings.ContainsAny(v, " .\\") {
		return false
	}
	_, err := time.LoadLocation(v)
	return err == nil
}

const (
	AvatarUpload = "upload"
	AvatarLDAP   = "ldap"
)

type Profile struct {
	LastName   string `json:"last_name"`
	FirstName  string `json:"first_name"`
	MiddleName string `json:"middle_name"`
	Title      string `json:"title"`
	Department string `json:"department"`
	Manager    string `json:"manager"`
	Email      string `json:"email"`
}

const MaxProfileField = 200

func (p Profile) Normalize() (Profile, error) {
	fields := []*string{&p.LastName, &p.FirstName, &p.MiddleName, &p.Title, &p.Department, &p.Manager, &p.Email}
	for _, f := range fields {
		*f = strings.Join(strings.Fields(*f), " ")
		if len([]rune(*f)) > MaxProfileField || strings.ContainsFunc(*f, unicode.IsControl) {
			return p, errors.New("profile field is too long or contains control characters")
		}
	}
	return p, nil
}

func ValidEmail(v string) bool {
	if v == "" {
		return true
	}
	a, err := mail.ParseAddress(v)
	return err == nil && a.Address == v
}

func (p Profile) DisplayName(fallback string) string {
	parts := []string{}
	for _, v := range []string{p.LastName, p.FirstName, p.MiddleName} {
		if v != "" {
			parts = append(parts, v)
		}
	}
	if len(parts) == 0 {
		return fallback
	}
	return strings.Join(parts, " ")
}

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Profile
	Avatar       []byte     `json:"-"`
	AvatarSource string     `json:"avatar_source,omitempty"`
	AvatarAt     *time.Time `json:"avatar_at,omitempty"`
	Source       string     `json:"source"`
	Timezone     string     `json:"timezone"`
	Role         string     `json:"role"`
	TeamID       string     `json:"team_id"`
	// MappedRole and MappedTeam are the values a group mapping last gave the user. When the
	// mapping stops matching, a value still equal to them is withdrawn; a value changed by hand
	// since then is kept.
	MappedRole         string     `json:"mapped_role,omitempty"`
	MappedTeam         string     `json:"mapped_team,omitempty"`
	MustChangePassword bool       `json:"must_change_password"`
	PasswordRef        string     `json:"-"`
	ExternalID         string     `json:"-"`
	PasswordHash       string     `json:"-"`
	PasswordChangedAt  time.Time  `json:"password_changed_at,omitzero"`
	Disabled           bool       `json:"disabled"`
	CreatedAt          time.Time  `json:"created_at"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
}

type Settings struct {
	DefaultTheme  string           `json:"default_theme"`
	DefaultLocale string           `json:"default_locale"`
	DefaultTZ     string           `json:"default_timezone"`
	Password      PasswordPolicy   `json:"password_policy"`
	LDAP          directory.Config `json:"ldap"`
	Entra         entra.Config     `json:"entra"`
	NetBox        netbox.Config    `json:"netbox"`
	Groups        GroupMappings    `json:"groups"`
	SetupAt       time.Time        `json:"setup_at"`
	SetupBy       string           `json:"setup_by"`
}
