package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/flow"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/rules"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

var errAlertsNotReady = errors.New("the alert tables are not ready yet")

// alertSink folds the events of a processed request into alerts in the same transaction.
func (a *App) alertSink(ctx context.Context, tx pgx.Tx, connectorID string, events []flow.Event) (func(), error) {
	in := make([]alert.Incoming, 0, len(events))
	for _, e := range events {
		in = append(in, alert.Incoming{ConnectorID: connectorID, Key: e.Key, Title: e.Title, CI: e.CI, Signal: e.Signal, Method: e.Method,
			Severity: e.Severity, Status: e.Status, Value: e.Value, Labels: e.Labels})
	}
	return a.alerts.Apply(ctx, tx, in)
}

func (a *App) registerIncidents(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/incidents", a.authed(a.can("incidents:view", a.listIncidents)))
	mux.HandleFunc("GET /api/incidents/{id}", a.authed(a.can("incidents:view", a.getIncident)))
	mux.HandleFunc("POST /api/incidents/{id}/{action}", a.authed(a.can("incidents:ack", a.actIncident)))
	mux.HandleFunc("POST /api/incidents/bulk", a.authed(a.can("incidents:ack", a.bulkIncidents)))
}

func (a *App) alertsReady(w http.ResponseWriter) bool {
	if a.alerts == nil || !a.ingestReady() {
		w.Header().Set("Retry-After", "5")
		httpx.Error(w, http.StatusServiceUnavailable, "alerts_unavailable", errAlertsNotReady)
		return false
	}
	return true
}

func incidentFilter(r *http.Request) alert.Filter {
	q := r.URL.Query()
	f := alert.Filter{Status: q.Get("status"), Method: q.Get("method"), TeamID: q.Get("team"), ServiceID: q.Get("service"),
		CIID: q.Get("ci"), Query: q.Get("q"), PD: q.Get("pd"), Fallback: q.Get("fallback") == "true", Suppressed: q.Get("suppressed") == "true"}
	for _, s := range strings.Split(q.Get("severity"), ",") {
		if s = strings.TrimSpace(s); alert.SeverityRank(s) > 0 {
			f.Severities = append(f.Severities, s)
		}
	}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil {
		f.Limit = n
	}
	if h, err := strconv.Atoi(q.Get("hours")); err == nil && h > 0 {
		f.Since = time.Now().Add(-time.Duration(h) * time.Hour)
	}
	return f
}

func (a *App) listIncidents(w http.ResponseWriter, r *http.Request) {
	if !a.alertsReady(w) {
		return
	}
	page, err := a.alerts.List(r.Context(), incidentFilter(r))
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

type incidentView struct {
	Alert    alert.Alert   `json:"alert"`
	Timeline []alert.Entry `json:"timeline"`
	Grafana  string        `json:"grafana_url,omitempty"`
	// Connectors names the connectors of the sources.
	Connectors map[string]string `json:"connectors"`
}

func (a *App) getIncident(w http.ResponseWriter, r *http.Request) {
	if !a.alertsReady(w) {
		return
	}
	al, entries, err := a.alerts.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		alertError(w, err)
		return
	}
	names := map[string]string{}
	a.deps.Store.Read(func(d *store.Data) {
		for _, src := range al.Sources {
			if c := d.Connectors[src.ConnectorID]; c != nil {
				names[src.ConnectorID] = c.Name
			} else if r := d.Rules[strings.TrimPrefix(src.ConnectorID, rules.ConnectorPrefix)]; r != nil && strings.HasPrefix(src.ConnectorID, rules.ConnectorPrefix) {
				names[src.ConnectorID] = r.Name
			}
		}
	})
	httpx.JSON(w, http.StatusOK, incidentView{Alert: al, Timeline: entries, Grafana: a.grafanaLink(al), Connectors: names})
}

type actInput struct {
	Text string `json:"text"`
}

func (a *App) actIncident(w http.ResponseWriter, r *http.Request) {
	if !a.alertsReady(w) {
		return
	}
	var in actInput
	if r.ContentLength != 0 && !httpx.Decode(w, r, &in) {
		return
	}
	out, err := a.alerts.Act(r.Context(), r.PathValue("id"), r.PathValue("action"), current(r).user.Username, in.Text)
	if err != nil {
		alertError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

type bulkInput struct {
	IDs    []string `json:"ids"`
	Action string   `json:"action"`
}

type bulkResult struct {
	Done   []string          `json:"done"`
	Failed map[string]string `json:"failed"`
}

func (a *App) bulkIncidents(w http.ResponseWriter, r *http.Request) {
	if !a.alertsReady(w) {
		return
	}
	var in bulkInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	if in.Action != "ack" && in.Action != "resolve" {
		httpx.Error(w, http.StatusBadRequest, "bad_action", nil)
		return
	}
	if len(in.IDs) > alert.MaxLimit {
		httpx.Error(w, http.StatusBadRequest, "too_many", nil)
		return
	}
	out := bulkResult{Done: []string{}, Failed: map[string]string{}}
	for _, id := range in.IDs {
		if _, err := a.alerts.Act(r.Context(), id, in.Action, current(r).user.Username, ""); err != nil {
			out.Failed[id] = alertCode(err)
			continue
		}
		out.Done = append(out.Done, id)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func alertCode(err error) string {
	switch {
	case errors.Is(err, alert.ErrNotFound):
		return "not_found"
	case errors.Is(err, alert.ErrNotOpen):
		return "not_open"
	case errors.Is(err, alert.ErrNotActive):
		return "not_active"
	case errors.Is(err, alert.ErrEmptyComment):
		return "empty_comment"
	case errors.Is(err, alert.ErrBadAction):
		return "bad_action"
	}
	return "internal"
}

func alertError(w http.ResponseWriter, err error) {
	switch code := alertCode(err); code {
	case "not_found":
		httpx.Error(w, http.StatusNotFound, code, nil)
	case "not_open", "not_active":
		httpx.Error(w, http.StatusConflict, code, nil)
	case "empty_comment", "bad_action":
		httpx.Error(w, http.StatusBadRequest, code, nil)
	default:
		writeError(w, err)
	}
}
