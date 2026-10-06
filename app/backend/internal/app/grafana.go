package app

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

const (
	defaultGrafanaWindow = 10
	maxGrafanaWindow     = 24 * 60
)

// GrafanaLink is the dashboard of an incident: the time from the window before it opened to the
// window after it ended (now while it lasts) and the variables ci, ci_id, service, team, signal
// and incident.
func GrafanaLink(set model.Grafana, al alert.Alert, now time.Time) string {
	if set.DashboardURL == "" {
		return ""
	}
	u, err := url.Parse(set.DashboardURL)
	if err != nil {
		return ""
	}
	win := time.Duration(set.WindowMinute) * time.Minute
	if set.WindowMinute <= 0 {
		win = defaultGrafanaWindow * time.Minute
	}
	start := al.OpenedAt
	if start.IsZero() {
		start = al.FirstSeen
	}
	q := u.Query()
	q.Set("from", strconv.FormatInt(start.Add(-win).UnixMilli(), 10))
	if al.ResolvedAt != nil {
		end := al.ResolvedAt.Add(win)
		if end.After(now) {
			q.Set("to", "now")
		} else {
			q.Set("to", strconv.FormatInt(end.UnixMilli(), 10))
		}
	} else {
		q.Set("to", "now")
	}
	q.Set("var-incident", al.ID)
	if al.CIName != "" {
		q.Set("var-ci", al.CIName)
	}
	if al.CIID != "" {
		q.Set("var-ci_id", al.CIID)
	}
	if al.Signal != "" {
		q.Set("var-signal", al.Signal)
	}
	if len(al.Route.Services) > 0 {
		q.Set("var-service", al.Route.Services[0].Name)
	}
	if al.Route.Team != nil {
		q.Set("var-team", al.Route.Team.Name)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (a *App) grafanaSettings() model.Grafana {
	var g model.Grafana
	a.deps.Store.Read(func(d *store.Data) { g = d.Settings.Alerting.Grafana })
	return g
}

// grafanaLink is the address of the incident context in Grafana; empty until Grafana is set up.
func (a *App) grafanaLink(al alert.Alert) string {
	return GrafanaLink(a.grafanaSettings(), al, time.Now().UTC())
}

type GrafanaInput struct {
	DashboardURL  string `json:"dashboard_url"`
	WindowMinutes int    `json:"window_minutes"`
}

func (a *App) registerGrafana(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/grafana", a.authed(a.can("settings.alerting:view", a.grafanaView)))
	mux.HandleFunc("PUT /api/grafana", a.authed(a.can("settings.alerting:edit", a.grafanaSave)))
	mux.HandleFunc("GET /go/incidents/{id}/grafana", a.grafanaRedirect)
}

func (a *App) grafanaView(w http.ResponseWriter, r *http.Request) {
	g := a.grafanaSettings()
	if g.WindowMinute == 0 {
		g.WindowMinute = defaultGrafanaWindow
	}
	httpx.JSON(w, http.StatusOK, g)
}

func (a *App) grafanaSave(w http.ResponseWriter, r *http.Request) {
	var in GrafanaInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	dash, err := optionalURL(in.DashboardURL)
	if err != nil {
		writeError(w, invalid("url_invalid", err))
		return
	}
	if in.WindowMinutes < 0 || in.WindowMinutes > maxGrafanaWindow {
		writeError(w, invalid("window_invalid", nil))
		return
	}
	if in.WindowMinutes == 0 {
		in.WindowMinutes = defaultGrafanaWindow
	}
	now := time.Now().UTC()
	actor := current(r).user.Username
	g := model.Grafana{DashboardURL: dash, WindowMinute: in.WindowMinutes, UpdatedAt: &now, UpdatedBy: actor}
	a.deps.Store.Write(func(d *store.Data) {
		d.Settings.Alerting.Grafana = g
		d.AddAudit(store.AuditEntry{Actor: actor, Action: "settings.grafana", Detail: fmt.Sprintf("dashboard set: %v", dash != "")})
	})
	httpx.JSON(w, http.StatusOK, g)
}

// grafanaRedirect sends a signed-in user to the Grafana context of an incident; PagerDuty links
// point here, so the link stays right when Grafana settings change. Anybody else goes to the
// incident in Umbrella, which asks them to sign in.
func (a *App) grafanaRedirect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	fallback := "/incidents?id=" + url.QueryEscape(id)
	c, err := r.Cookie(CookieName)
	if err != nil || a.alerts == nil || !a.ingestReady() {
		http.Redirect(w, r, fallback, http.StatusFound)
		return
	}
	ss := a.deps.Sessions.Get(c.Value)
	var u *model.User
	if ss != nil {
		a.deps.Store.Read(func(d *store.Data) {
			if x := d.Users[ss.UserID]; x != nil && !x.Disabled {
				cp := *x
				u = &cp
			}
		})
	}
	if u == nil || !a.access.Permissions(*u).Has("incidents:view") {
		http.Redirect(w, r, fallback, http.StatusFound)
		return
	}
	al, _, err := a.alerts.Get(r.Context(), id)
	if err != nil || !al.InScope(a.incidentScope(*u)) {
		http.Redirect(w, r, fallback, http.StatusFound)
		return
	}
	link := a.grafanaLink(al)
	if link == "" || !strings.HasPrefix(link, "http") {
		http.Redirect(w, r, fallback, http.StatusFound)
		return
	}
	http.Redirect(w, r, link, http.StatusFound)
}
