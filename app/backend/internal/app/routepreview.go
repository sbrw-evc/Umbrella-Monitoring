package app

import (
	"net/http"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pagerduty"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// RoutePreview is where an incident would go now: the services, the primary service and its
// team, the people (or the owners of the item when the team has nobody) and the PagerDuty
// route. It is computed by the alert engine's own rules.
type RoutePreview struct {
	Services []alert.Ref    `json:"services"`
	Service  *alert.Ref     `json:"service,omitempty"`
	Team     *alert.Ref     `json:"team,omitempty"`
	People   []alert.Person `json:"people"`
	Owners   []alert.Person `json:"owners"`
	Via      string         `json:"via"`
	// PagerDuty is null when PagerDuty is off.
	PagerDuty *PreviewPD `json:"pagerduty_route"`
	// Elsewhere (service preview only): items of the service routed by a more critical service.
	Elsewhere []alert.Elsewhere `json:"elsewhere,omitempty"`
}

type PreviewPD struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MinSeverity string `json:"min_severity"`
	// NoKey: the chosen route (or the default integration) has no integration key.
	NoKey bool `json:"no_key,omitempty"`
}

func (a *App) previewOf(r alert.Route) RoutePreview {
	out := RoutePreview{Services: r.Services, Service: r.Service, Team: r.Team, People: r.People, Owners: r.Owners, Via: r.Via}
	var p *PreviewPD
	a.deps.Store.Read(func(d *store.Data) {
		set := d.Settings.Alerting.PagerDuty
		if !set.Enabled {
			return
		}
		id, name := pagerduty.RouteOf(set, alert.Alert{Route: r})
		p = &PreviewPD{ID: id, Name: name, MinSeverity: set.MinSeverity, NoKey: id == pagerduty.DefaultRoute && set.RoutingKeyRef == ""}
	})
	out.PagerDuty = p
	return out
}

func (a *App) registerRoutePreview(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/cis/{id}/route", a.authed(a.can("cis:view", a.ciRoute)))
	mux.HandleFunc("GET /api/services/{id}/route", a.authed(a.can("services:view", a.serviceRoute)))
}

func (a *App) ciRoute(w http.ResponseWriter, r *http.Request) {
	rt, ok := alert.PreviewCI(a.deps.Store, r.PathValue("id"), time.Now().UTC())
	if !ok {
		writeError(w, ErrNotFound)
		return
	}
	httpx.JSON(w, http.StatusOK, a.previewOf(rt))
}

func (a *App) serviceRoute(w http.ResponseWriter, r *http.Request) {
	rt, elsewhere, ok := alert.PreviewService(a.deps.Store, r.PathValue("id"), time.Now().UTC())
	if !ok {
		writeError(w, ErrNotFound)
		return
	}
	out := a.previewOf(rt)
	out.Elsewhere = elsewhere
	httpx.JSON(w, http.StatusOK, out)
}
