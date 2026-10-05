package api

import (
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

var teamIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,39}$`)

type teamView struct {
	model.Team
	Managed    bool `json:"managed"`
	CIs        int  `json:"cis"`
	OpenAlerts int  `json:"open_alerts"`
	Connectors int  `json:"connectors"`
	Rules      int  `json:"rules"`
}

func teamViews(d *store.Data) []teamView {
	byID := map[string]*teamView{}
	get := func(id string) *teamView {
		v := byID[id]
		if v == nil {
			v = &teamView{Team: model.Team{ID: id, Name: id, Members: []string{}, Leads: []string{}}}
			byID[id] = v
		}
		return v
	}
	for _, t := range d.Teams {
		v := get(t.ID)
		v.Team, v.Managed = *t, true
	}
	for _, ci := range d.CIs {
		if ci.Team != "" {
			get(ci.Team).CIs++
		}
	}
	for _, a := range d.Alerts {
		if a.Team != "" && a.Status.Active() {
			get(a.Team).OpenAlerts++
		}
	}
	for _, c := range d.Connectors {
		if c.Team != "" {
			get(c.Team).Connectors++
		}
	}
	for _, r := range d.Rules {
		if r.Team != "" {
			get(r.Team).Rules++
		}
	}
	out := make([]teamView, 0, len(byID))
	for _, v := range byID {
		if v.Team.Members == nil {
			v.Team.Members = []string{}
		}
		if v.Team.Leads == nil {
			v.Team.Leads = []string{}
		}
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Managed != out[j].Managed {
			return out[i].Managed
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (s *Server) listTeams(w http.ResponseWriter, _ *http.Request) {
	var out []teamView
	s.st.Read(func(d *store.Data) { out = teamViews(d) })
	writeJSON(w, 200, map[string]any{"items": out})
}

type teamBody struct {
	ID          string    `json:"id"`
	Name        *string   `json:"name"`
	Description *string   `json:"description"`
	Email       *string   `json:"email"`
	Chat        *string   `json:"chat"`
	Members     *[]string `json:"members"`
	Leads       *[]string `json:"leads"`
}

func applyTeam(d *store.Data, t *model.Team, b teamBody) error {
	if b.Name != nil {
		t.Name = strings.TrimSpace(*b.Name)
	}
	if b.Description != nil {
		t.Description = strings.TrimSpace(*b.Description)
	}
	if b.Email != nil {
		t.Email = strings.TrimSpace(*b.Email)
	}
	if b.Chat != nil {
		t.Chat = strings.TrimSpace(*b.Chat)
	}
	if b.Members != nil {
		t.Members = dedupe(*b.Members)
	}
	if b.Leads != nil {
		t.Leads = dedupe(*b.Leads)
	}
	if t.Name == "" {
		return errors.New("нужно название команды")
	}
	if t.Email != "" && !strings.Contains(t.Email, "@") {
		return errors.New("неверный адрес почты команды")
	}
	if t.Members == nil {
		t.Members = []string{}
	}
	if t.Leads == nil {
		t.Leads = []string{}
	}
	for _, id := range append(append([]string{}, t.Members...), t.Leads...) {
		if d.Users[id] == nil {
			return errors.New("пользователь " + id + " не найден")
		}
	}
	for _, l := range t.Leads {
		if !contains(t.Members, l) {
			t.Members = append(t.Members, l)
		}
	}
	return nil
}

func (s *Server) createTeam(w http.ResponseWriter, r *http.Request) {
	var b teamBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	id := strings.ToLower(strings.TrimSpace(b.ID))
	if !teamIDRe.MatchString(id) {
		writeErr(w, 400, errors.New("код команды: латиница в нижнем регистре, цифры, точка, дефис, подчёркивание"))
		return
	}
	now := time.Now()
	var out model.Team
	var err error
	s.st.Write(func(d *store.Data) {
		if d.Teams[id] != nil {
			err = errors.New("команда " + id + " уже есть")
			return
		}
		t := model.Team{ID: id, Name: id, CreatedAt: &now, UpdatedAt: &now, UpdatedBy: actor(r)}
		if err = applyTeam(d, &t, b); err != nil {
			return
		}
		d.Teams[id] = &t
		out = t
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: actor(r), Action: "team.create", Object: id})
	})
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 201, out)
}

func (s *Server) updateTeam(w http.ResponseWriter, r *http.Request) {
	var b teamBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	id := r.PathValue("id")
	now := time.Now()
	var out model.Team
	var err error
	code := 400
	s.st.Write(func(d *store.Data) {
		cur := d.Teams[id]
		if cur == nil {
			err, code = errors.New("команда не найдена"), 404
			return
		}
		next := *cur
		if err = applyTeam(d, &next, b); err != nil {
			return
		}
		next.UpdatedAt, next.UpdatedBy = &now, actor(r)
		*cur = next
		out = next
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: actor(r), Action: "team.update", Object: id})
	})
	if err != nil {
		writeErr(w, code, err)
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) deleteTeam(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	detach := r.URL.Query().Get("detach") == "1"
	var err error
	code := 400
	s.st.Write(func(d *store.Data) {
		if d.Teams[id] == nil {
			err, code = errors.New("команда не найдена"), 404
			return
		}
		if n := d.TeamUsage()[id]; n > 0 && !detach {
			err, code = errors.New("команда используется в КЕ, коннекторах, интеграциях или правилах; удалите с отвязкой"), 409
			return
		}
		delete(d.Teams, id)
		if detach {
			for _, ci := range d.CIs {
				if ci.Team == id {
					ci.Team = ""
				}
			}
			for _, c := range d.Connectors {
				if c.Team == id {
					c.Team = ""
				}
			}
			for _, it := range d.Integrations {
				if it.Team == id {
					it.Team = ""
				}
			}
			for _, rl := range d.Rules {
				if rl.Team == id {
					rl.Team = ""
				}
			}
		}
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: "team.delete", Object: id})
	})
	if err != nil {
		writeErr(w, code, err)
		return
	}
	w.WriteHeader(204)
}
