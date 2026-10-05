package app

import (
	"net/http"
	"slices"
	"strings"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/access"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type RoleRef struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	System bool   `json:"system"`
}

type TeamRef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
}

type UserRef struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Source   string `json:"source"`
	Disabled bool   `json:"disabled"`
	RoleID   string `json:"role_id"`
	TeamID   string `json:"team_id"`
}

type Refs struct {
	Roles    []RoleRef    `json:"roles"`
	Teams    []TeamRef    `json:"teams"`
	Users    []UserRef    `json:"users"`
	Services []ServiceRef `json:"services"`
}

type CatalogView struct {
	Groups []access.Group `json:"groups"`
	Pages  []access.Page  `json:"pages"`
}

func (a *App) registerRefs(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/refs", a.authed(a.refs))
	mux.HandleFunc("GET /api/access/catalog", a.authed(a.catalog))
}

func (a *App) refs(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, a.collectRefs())
}

func (a *App) catalog(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, CatalogView{Groups: access.Groups(), Pages: access.Pages()})
}

func byName(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) }

func (a *App) collectRefs() Refs {
	out := Refs{Roles: []RoleRef{}, Teams: []TeamRef{}, Users: []UserRef{}, Services: []ServiceRef{}}
	a.deps.Store.Read(func(d *store.Data) {
		for _, r := range d.Roles {
			out.Roles = append(out.Roles, RoleRef{ID: r.ID, Name: r.Name, System: r.System})
		}
		for _, t := range d.Teams {
			out.Teams = append(out.Teams, TeamRef{ID: t.ID, Name: t.Name, ParentID: t.ParentID})
		}
		for _, u := range d.Users {
			out.Users = append(out.Users, UserRef{ID: u.ID, Username: u.Username, Name: u.Profile.DisplayName(u.Username), Source: u.Source,
				Disabled: u.Disabled, RoleID: d.RoleOf(u).ID, TeamID: u.TeamID})
		}
		for _, s := range d.Services {
			out.Services = append(out.Services, ServiceRef{ID: s.ID, Name: s.Name})
		}
	})
	slices.SortFunc(out.Roles, func(x, y RoleRef) int { return byName(x.Name, y.Name) })
	slices.SortFunc(out.Teams, func(x, y TeamRef) int { return byName(x.Name, y.Name) })
	slices.SortFunc(out.Users, func(x, y UserRef) int { return byName(x.Name, y.Name) })
	slices.SortFunc(out.Services, func(x, y ServiceRef) int { return byName(x.Name, y.Name) })
	return out
}
