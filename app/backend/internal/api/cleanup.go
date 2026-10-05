package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

func (s *Server) deleteIncident(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	removed := -1
	s.st.Write(func(d *store.Data) {
		if d.Alerts[id] == nil {
			return
		}
		delete(d.Alerts, id)
		kept := d.Events[:0]
		removed = 0
		for _, ev := range d.Events {
			if ev.AlertID == id {
				removed++
				continue
			}
			kept = append(kept, ev)
		}
		d.Events = kept
		for _, a := range d.Alerts {
			if a.RelatedID == id {
				a.RelatedID = ""
			}
		}
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: "incident.delete", Object: id})
	})
	if removed < 0 {
		writeErr(w, 404, alert.ErrNotFound)
		return
	}
	writeJSON(w, 200, map[string]int{"events": removed})
}

func (s *Server) deleteParseErrors(w http.ResponseWriter, r *http.Request) {
	conn := r.URL.Query().Get("connector")
	if conn == "" {
		writeErr(w, 400, errors.New("укажите connector"))
		return
	}
	removed := 0
	s.st.Write(func(d *store.Data) {
		kept := d.ParseErrors[:0]
		for _, p := range d.ParseErrors {
			if p.ConnectorID == conn {
				removed++
				continue
			}
			kept = append(kept, p)
		}
		d.ParseErrors = kept
		if c := d.Connectors[conn]; c != nil {
			c.ErrorsTotal = 0
		}
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: "parse_errors.delete", Object: conn})
	})
	writeJSON(w, 200, map[string]int{"removed": removed})
}
