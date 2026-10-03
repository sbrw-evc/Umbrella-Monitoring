package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

var ciTypes = []string{model.CIBusinessService, model.CIITService, model.CIHost, model.CIDatabase, model.CICloudGroup, model.CIDeployment, model.CINetwork}

var relationTypes = []string{"depends_on", "runs_on", "part_of"}

type ciBody struct {
	Name         *string            `json:"name"`
	Type         *string            `json:"type"`
	Team         *string            `json:"team"`
	Description  *string            `json:"description"`
	LogicalGroup *string            `json:"logical_group"`
	Labels       *map[string]string `json:"labels"`
	Identities   *[]model.Identity  `json:"identities"`
	Owners       *[]model.Owner     `json:"owners"`
}

func applyCI(d *store.Data, ci *model.CI, b ciBody, now time.Time) error {
	if b.Name != nil {
		ci.Name = strings.TrimSpace(*b.Name)
	}
	if b.Type != nil {
		ci.Type = *b.Type
	}
	if b.Team != nil {
		ci.Team = strings.TrimSpace(*b.Team)
	}
	if b.Description != nil {
		ci.Description = strings.TrimSpace(*b.Description)
	}
	if b.LogicalGroup != nil {
		ci.LogicalGroup = strings.TrimSpace(*b.LogicalGroup)
	}
	if b.Labels != nil {
		ci.Labels = map[string]string{}
		for k, v := range *b.Labels {
			if k = strings.TrimSpace(k); k != "" {
				ci.Labels[k] = strings.TrimSpace(v)
			}
		}
	}
	if b.Identities != nil {
		old := map[string]model.Identity{}
		for _, i := range ci.Identities {
			old[i.Kind+"\x00"+i.Value] = i
		}
		ids := []model.Identity{}
		for _, i := range *b.Identities {
			i.Kind, i.Value = strings.TrimSpace(i.Kind), strings.TrimSpace(i.Value)
			if i.Kind == "" || i.Value == "" {
				continue
			}
			if o, ok := old[i.Kind+"\x00"+i.Value]; ok {
				i.Since, i.Until = o.Since, o.Until
			}
			if i.Since.IsZero() {
				i.Since = now
			}
			ids = append(ids, i)
		}
		ci.Identities = ids
	}
	if b.Owners != nil {
		owners := []model.Owner{}
		for _, o := range *b.Owners {
			o.Name, o.Email, o.Phone, o.Role = strings.TrimSpace(o.Name), strings.TrimSpace(o.Email), strings.TrimSpace(o.Phone), strings.TrimSpace(o.Role)
			if o.Name == "" && o.Email == "" {
				continue
			}
			if o.From == "" {
				o.From = "manual"
			}
			owners = append(owners, o)
		}
		ci.Owners = owners
	}
	if ci.Name == "" {
		return errors.New("нужно название КЕ")
	}
	if !contains(ciTypes, ci.Type) {
		return errors.New("неизвестный тип КЕ " + ci.Type)
	}
	for _, o := range d.CIs {
		if o.ID != ci.ID && strings.EqualFold(o.Name, ci.Name) {
			return errors.New("КЕ с названием «" + ci.Name + "» уже есть: " + o.ID)
		}
	}
	return nil
}

func (s *Server) updateCI(w http.ResponseWriter, r *http.Request) {
	var b ciBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	id := r.PathValue("id")
	var out model.CI
	code, err := 0, error(nil)
	now := time.Now()
	s.st.Write(func(d *store.Data) {
		cur := d.CIs[id]
		if cur == nil || !me(r).SeesCI(id) {
			code, err = 404, errors.New("КЕ не найдена")
			return
		}
		next := *cur
		if err = applyCI(d, &next, b, now); err != nil {
			code = 400
			return
		}
		next.UpdatedAt = &now
		*cur = next
		for _, a := range d.Alerts {
			if a.CIID == id {
				a.CIName, a.CIType = next.Name, next.Type
			}
		}
		out = next
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: actor(r), Action: "ci.update", Object: id})
	})
	if err != nil {
		writeErr(w, code, err)
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) deleteCI(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	found := false
	now := time.Now()
	s.st.Write(func(d *store.Data) {
		if d.CIs[id] == nil || !me(r).SeesCI(id) {
			return
		}
		found = true
		d.DeleteCI(id)
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: actor(r), Action: "ci.delete", Object: id})
	})
	if !found {
		writeErr(w, 404, errors.New("КЕ не найдена"))
		return
	}
	w.WriteHeader(204)
}

func (s *Server) createRelation(w http.ResponseWriter, r *http.Request) {
	var b model.Relation
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	if b.Type == "" {
		b.Type = "depends_on"
	}
	var err error
	s.st.Write(func(d *store.Data) {
		switch {
		case b.From == b.To:
			err = errors.New("КЕ не может зависеть от себя")
		case d.CIs[b.From] == nil || d.CIs[b.To] == nil || !me(r).SeesCI(b.From) || !me(r).SeesCI(b.To):
			err = errors.New("КЕ не найдена")
		case !contains(relationTypes, b.Type):
			err = errors.New("тип связи: depends_on, runs_on или part_of")
		}
		if err != nil {
			return
		}
		for _, rel := range d.Relations {
			if rel.From == b.From && rel.To == b.To {
				err = errors.New("такая связь уже есть")
				return
			}
		}
		d.Relations = append(d.Relations, b)
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: "relation.create " + b.Type, Object: b.From + "→" + b.To})
	})
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 201, b)
}

func (s *Server) deleteRelation(w http.ResponseWriter, r *http.Request) {
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	found := false
	s.st.Write(func(d *store.Data) {
		if !me(r).SeesCI(from) || !me(r).SeesCI(to) {
			return
		}
		kept := d.Relations[:0]
		for _, rel := range d.Relations {
			if rel.From == from && rel.To == to {
				found = true
				continue
			}
			kept = append(kept, rel)
		}
		d.Relations = kept
		if found {
			d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: "relation.delete", Object: from + "→" + to})
		}
	})
	if !found {
		writeErr(w, 404, errors.New("связь не найдена"))
		return
	}
	w.WriteHeader(204)
}
