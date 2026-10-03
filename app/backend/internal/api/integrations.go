package api

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/connector"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/integration"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

func (s *Server) integrationTypes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"items": integration.Types, "openbao": s.vault.Enabled()})
}

func (s *Server) listIntegrations(w http.ResponseWriter, _ *http.Request) {
	list := s.integrations.List()
	sort.Slice(list, func(i, j int) bool { return idNum(list[i].ID) < idNum(list[j].ID) })
	if list == nil {
		list = []integration.View{}
	}
	writeJSON(w, 200, map[string]any{"items": list})
}

func (s *Server) getIntegration(w http.ResponseWriter, r *http.Request) {
	v, ok := s.integrations.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, integration.ErrNotFound)
		return
	}
	writeJSON(w, 200, v)
}

func integrationErr(w http.ResponseWriter, err error) {
	if errors.Is(err, integration.ErrNotFound) {
		writeErr(w, 404, err)
		return
	}
	writeErr(w, 400, err)
}

func (s *Server) createIntegration(w http.ResponseWriter, r *http.Request) {
	var in integration.Input
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err)
		return
	}
	v, err := s.integrations.Create(r.Context(), in, actor(r))
	if err != nil {
		integrationErr(w, err)
		return
	}
	writeJSON(w, 201, v)
}

func (s *Server) updateIntegration(w http.ResponseWriter, r *http.Request) {
	var in integration.Input
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err)
		return
	}
	v, err := s.integrations.Update(r.Context(), r.PathValue("id"), in, actor(r))
	if err != nil {
		integrationErr(w, err)
		return
	}
	writeJSON(w, 200, v)
}

func (s *Server) deleteIntegration(w http.ResponseWriter, r *http.Request) {
	note, err := s.integrations.Delete(r.Context(), r.PathValue("id"), r.URL.Query().Get("teardown") == "1", actor(r))
	if err != nil {
		integrationErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"note": note})
}

func (s *Server) checkIntegration(w http.ResponseWriter, r *http.Request) {
	res, err := s.integrations.Check(r.Context(), r.PathValue("id"))
	if err != nil {
		integrationErr(w, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) setupIntegration(w http.ResponseWriter, r *http.Request) {
	res, err := s.integrations.Setup(r.Context(), r.PathValue("id"), actor(r))
	if err != nil {
		integrationErr(w, err)
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) revealIntegrationToken(w http.ResponseWriter, r *http.Request) {
	tok, err := s.integrations.RevealToken(r.PathValue("id"))
	if err != nil {
		integrationErr(w, err)
		return
	}
	s.audit(r, "integration.token.reveal", r.PathValue("id"))
	writeJSON(w, 200, map[string]string{"token": tok})
}

var secretKeyRe = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)

func (s *Server) connectorSecret(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	if !secretKeyRe.MatchString(b.Key) || b.Value == "" {
		writeErr(w, 400, errors.New("нужны ключ (латиница, цифры, _) и значение"))
		return
	}
	id := r.PathValue("id")
	exists := false
	s.st.Read(func(d *store.Data) { exists = d.Connectors[id] != nil })
	if !exists {
		writeErr(w, 404, connector.ErrNotFound)
		return
	}
	ref, err := s.vault.PutRef(r.Context(), "connectors/"+id, b.Key, b.Value)
	if err != nil {
		writeErr(w, 503, err)
		return
	}
	s.audit(r, "connector.secret "+b.Key, id)
	writeJSON(w, 200, map[string]string{"ref": ref})
}

func (s *Server) getSettings(w http.ResponseWriter, _ *http.Request) {
	var set model.Settings
	s.st.Read(func(d *store.Data) { set = d.Settings })
	writeJSON(w, 200, map[string]any{"settings": set, "public_url": s.cfg.PublicURL})
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var b struct {
		GrafanaURL    *string `json:"grafana_url"`
		DefaultTheme  *string `json:"default_theme"`
		DefaultLocale *string `json:"default_locale"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	if b.DefaultTheme != nil || b.DefaultLocale != nil {
		theme, locale := s.uiDefaults()
		if b.DefaultTheme != nil {
			theme = *b.DefaultTheme
		}
		if b.DefaultLocale != nil {
			locale = *b.DefaultLocale
		}
		if err := validDefaults(theme, locale); err != nil {
			writeErr(w, 400, err)
			return
		}
		s.st.Write(func(d *store.Data) { d.Settings.DefaultTheme, d.Settings.DefaultLocale = theme, locale })
		s.audit(r, "settings.defaults "+theme+"/"+locale, "settings")
	}
	if b.GrafanaURL != nil {
		v := strings.TrimSpace(*b.GrafanaURL)
		if v != "" {
			u, err := url.Parse(v)
			if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || !strings.Contains(u.Path, "/d/") {
				writeErr(w, 400, errors.New("укажите адрес дашборда Grafana вида https://grafana/d/<uid>/<имя>"))
				return
			}
		}
		now := time.Now()
		s.st.Write(func(d *store.Data) {
			d.Settings.GrafanaURL = v
			d.Settings.UpdatedAt, d.Settings.UpdatedBy = &now, actor(r)
			d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: actor(r), Action: "settings.grafana", Object: "settings"})
		})
	}
	s.getSettings(w, r)
}

func (s *Server) openbaoStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.vault.Status(r.Context()))
}
