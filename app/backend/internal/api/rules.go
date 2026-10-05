package api

import (
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/integration"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/rules"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

func ruleView(r *model.Rule) model.Rule {
	c := *r
	c.State = map[string]*model.RuleSeries{}
	for k, v := range r.State {
		x := *v
		c.State[k] = &x
	}
	return c
}

func (s *Server) listRules(w http.ResponseWriter, _ *http.Request) {
	out := []model.Rule{}
	sources := []map[string]string{}
	s.st.Read(func(d *store.Data) {
		for _, r := range d.Rules {
			out = append(out, ruleView(r))
		}
		for _, it := range d.Integrations {
			if it.Type == integration.TypePrometheus {
				sources = append(sources, map[string]string{"id": it.ID, "name": it.Name, "url": it.URL})
			}
		}
	})
	sort.Slice(out, func(i, j int) bool { return idNum(out[i].ID) < idNum(out[j].ID) })
	sort.Slice(sources, func(i, j int) bool { return idNum(sources[i]["id"]) < idNum(sources[j]["id"]) })
	writeJSON(w, 200, map[string]any{"items": out, "sources": sources, "templates": rules.Templates(), "ops": rules.Ops})
}

func (s *Server) sourceExists(id string) bool {
	ok := false
	s.st.Read(func(d *store.Data) {
		it := d.Integrations[id]
		ok = it != nil && it.Type == integration.TypePrometheus
	})
	return ok
}

func (s *Server) createRule(w http.ResponseWriter, r *http.Request) {
	var b model.Rule
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := rules.Normalize(&b); err != nil {
		writeErr(w, 400, err)
		return
	}
	if !s.sourceExists(b.SourceID) {
		writeErr(w, 400, errors.New("источник метрик не найден"))
		return
	}
	now := time.Now()
	s.st.Write(func(d *store.Data) {
		b.ID = d.NextID("RUL")
		b.CreatedAt, b.UpdatedAt, b.UpdatedBy = now, now, actor(r)
		b.State, b.LastEvalAt, b.LastError, b.SeriesCount, b.Pending, b.Firing = nil, nil, "", 0, 0, 0
		rr := b
		d.Rules[b.ID] = &rr
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: actor(r), Action: "rule.create", Object: b.ID})
	})
	writeJSON(w, 201, b)
}

func (s *Server) updateRule(w http.ResponseWriter, r *http.Request) {
	var b model.Rule
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := rules.Normalize(&b); err != nil {
		writeErr(w, 400, err)
		return
	}
	if !s.sourceExists(b.SourceID) {
		writeErr(w, 400, errors.New("источник метрик не найден"))
		return
	}
	id := r.PathValue("id")
	var released *model.Rule
	var out model.Rule
	found := false
	now := time.Now()
	s.st.Write(func(d *store.Data) {
		cur := d.Rules[id]
		if cur == nil {
			return
		}
		found = true
		changed := cur.Query != b.Query || cur.SourceID != b.SourceID || cur.Op != b.Op || cur.Threshold != b.Threshold ||
			cur.CILabel != b.CILabel || cur.Signal != b.Signal || cur.Method != b.Method || !b.Enabled
		if changed && len(cur.State) > 0 {
			c := ruleView(cur)
			released = &c
			cur.State = nil
			cur.Firing, cur.Pending = 0, 0
		}
		b.ID, b.CreatedAt, b.UpdatedAt, b.UpdatedBy = id, cur.CreatedAt, now, actor(r)
		b.State, b.LastEvalAt, b.LastError, b.SeriesCount, b.Pending, b.Firing = cur.State, nil, cur.LastError, cur.SeriesCount, cur.Pending, cur.Firing
		*cur = b
		out = ruleView(cur)
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: actor(r), Action: "rule.update", Object: id})
	})
	if !found {
		writeErr(w, 404, errors.New("правило не найдено"))
		return
	}
	if released != nil {
		s.rules.Release(*released)
	}
	writeJSON(w, 200, out)
}

func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var gone *model.Rule
	s.st.Write(func(d *store.Data) {
		if cur := d.Rules[id]; cur != nil {
			c := ruleView(cur)
			gone = &c
			delete(d.Rules, id)
			d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: "rule.delete", Object: id})
		}
	})
	if gone == nil {
		writeErr(w, 404, errors.New("правило не найдено"))
		return
	}
	s.rules.Release(*gone)
	w.WriteHeader(204)
}

func (s *Server) previewRule(w http.ResponseWriter, r *http.Request) {
	var b model.Rule
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, s.rules.Preview(r.Context(), b))
}

func (s *Server) evaluateRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.rules.Evaluate(r.Context(), id)
	var out *model.Rule
	s.st.Read(func(d *store.Data) {
		if p := d.Rules[id]; p != nil {
			c := ruleView(p)
			out = &c
		}
	})
	if out == nil {
		writeErr(w, 404, errors.New("правило не найдено"))
		return
	}
	if err != nil {
		out.LastError = err.Error()
	}
	writeJSON(w, 200, out)
}

func (s *Server) syncIntegration(w http.ResponseWriter, r *http.Request) {
	res, err := s.integrations.Sync(r.Context(), r.PathValue("id"), actor(r))
	if err != nil {
		integrationErr(w, err)
		return
	}
	writeJSON(w, 200, res)
}
