package api

import (
	"errors"
	"sort"
	"strings"
	"time"

	"net/http"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/notify"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

var allNotifyEvents = []string{model.NotifyOpen, model.NotifyEscalate, model.NotifyAck, model.NotifyResolve, model.NotifyFallback}

// GET /api/channels
func (s *Server) listChannels(w http.ResponseWriter, _ *http.Request) {
	out := []model.Channel{}
	s.st.Read(func(d *store.Data) {
		for _, c := range d.Channels {
			out = append(out, *c)
		}
	})
	sort.Slice(out, func(i, j int) bool { return idNum(out[i].ID) < idNum(out[j].ID) })
	writeJSON(w, 200, map[string]any{"items": out, "events": allNotifyEvents, "allow_http": s.cfg.AllowHTTPWebhooks})
}

type channelBody struct {
	Name        *string   `json:"name"`
	Type        *string   `json:"type"`
	Mode        *string   `json:"mode"`
	Enabled     *bool     `json:"enabled"`
	MinSeverity *string   `json:"min_severity"`
	Events      *[]string `json:"events"`
	Services    *[]string `json:"services"`
	URL         *string   `json:"url"` // write-only; "" keeps the stored one
	URLRef      *string   `json:"url_ref"`
	Token       *string   `json:"token"`
	TokenRef    *string   `json:"token_ref"`
	ClearToken  bool      `json:"clear_token"`
}

// apply validates and copies the body into c. Call inside a store write.
func (s *Server) applyChannel(d *store.Data, c *model.Channel, b *channelBody) error {
	if b.Name != nil {
		c.Name = strings.TrimSpace(*b.Name)
	}
	if b.Type != nil {
		c.Type = *b.Type
	}
	if b.Mode != nil {
		c.Mode = *b.Mode
	}
	if b.Enabled != nil {
		c.Enabled = *b.Enabled
	}
	if b.MinSeverity != nil {
		c.MinSeverity = model.Severity(*b.MinSeverity)
	}
	if b.Events != nil {
		c.Events = dedupe(*b.Events)
	}
	if b.Services != nil {
		c.Services = dedupe(*b.Services)
	}
	if b.URL != nil && strings.TrimSpace(*b.URL) != "" {
		if err := notify.CheckURL(*b.URL, s.cfg.AllowHTTPWebhooks); err != nil {
			return err
		}
		c.URL = strings.TrimSpace(*b.URL)
	}
	if b.URLRef != nil {
		c.URLRef = strings.TrimSpace(*b.URLRef)
	}
	if b.Token != nil && *b.Token != "" {
		c.Token = *b.Token
	}
	if b.ClearToken {
		c.Token = ""
	}
	if b.TokenRef != nil {
		c.TokenRef = strings.TrimSpace(*b.TokenRef)
	}
	if c.Name == "" {
		return errors.New("нужно название канала")
	}
	if c.Type != model.ChannelTeams && c.Type != model.ChannelZoom {
		return errors.New("тип канала: teams или zoom")
	}
	if c.Mode != model.ChannelAlways && c.Mode != model.ChannelFallback {
		return errors.New("режим: always или fallback")
	}
	if !c.MinSeverity.Valid() {
		return errors.New("минимальная важность: critical, error, warning или info")
	}
	for _, e := range c.Events {
		if !contains(allNotifyEvents, e) {
			return errors.New("неизвестное событие " + e)
		}
	}
	if len(c.Events) == 0 {
		return errors.New("выберите хотя бы одно событие")
	}
	for _, id := range c.Services {
		ci := d.CIs[id]
		if ci == nil || (ci.Type != model.CIBusinessService && ci.Type != model.CIITService) {
			return errors.New("КЕ " + id + " не бизнес-услуга и не ИТ-сервис")
		}
	}
	for _, ref := range []string{c.URLRef, c.TokenRef} {
		if ref != "" && !strings.Contains(ref, "://") {
			return errors.New("ссылка на секрет имеет вид openbao://путь")
		}
	}
	if c.URL == "" && c.URLRef == "" {
		return errors.New("укажите адрес webhook или ссылку на секрет с ним")
	}
	c.URLSet, c.TokenSet = c.URL != "", c.Token != ""
	c.URLHint = notify.Host(c.URL)
	if c.URLHint == "" && c.URLRef != "" {
		c.URLHint = c.URLRef
	}
	return nil
}

// POST /api/channels
func (s *Server) createChannel(w http.ResponseWriter, r *http.Request) {
	var b channelBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	var c model.Channel
	var err error
	s.st.Write(func(d *store.Data) {
		c = model.Channel{Mode: model.ChannelAlways, Enabled: true, MinSeverity: model.SevError,
			Events: []string{model.NotifyOpen, model.NotifyEscalate, model.NotifyResolve}, Services: []string{}}
		if err = s.applyChannel(d, &c, &b); err != nil {
			return
		}
		c.ID = d.NextID("CH")
		c.UpdatedAt, c.UpdatedBy = time.Now(), actor(r)
		cc := c
		d.Channels[c.ID] = &cc
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: "channel.create", Object: c.ID})
	})
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 201, c)
}

// PUT /api/channels/{id}
func (s *Server) updateChannel(w http.ResponseWriter, r *http.Request) {
	var b channelBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err)
		return
	}
	var out model.Channel
	var err error
	code := 400
	s.st.Write(func(d *store.Data) {
		c := d.Channels[r.PathValue("id")]
		if c == nil {
			err, code = errors.New("канал не найден"), 404
			return
		}
		next := *c
		if err = s.applyChannel(d, &next, &b); err != nil {
			return
		}
		next.UpdatedAt, next.UpdatedBy = time.Now(), actor(r)
		*c = next
		out = next
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: "channel.update", Object: c.ID})
	})
	if err != nil {
		writeErr(w, code, err)
		return
	}
	writeJSON(w, 200, out)
}

// DELETE /api/channels/{id}
func (s *Server) deleteChannel(w http.ResponseWriter, r *http.Request) {
	s.st.Write(func(d *store.Data) {
		delete(d.Channels, r.PathValue("id"))
		d.AddAudit(store.AuditEntry{At: time.Now().Format(time.RFC3339), Actor: actor(r), Action: "channel.delete", Object: r.PathValue("id")})
	})
	w.WriteHeader(204)
}

// POST /api/channels/{id}/test sends a test message now.
func (s *Server) testChannel(w http.ResponseWriter, r *http.Request) {
	var ch *model.Channel
	s.st.Read(func(d *store.Data) {
		if c := d.Channels[r.PathValue("id")]; c != nil {
			cc := *c
			ch = &cc
		}
	})
	if ch == nil {
		writeErr(w, 404, errors.New("канал не найден"))
		return
	}
	if s.notify == nil {
		writeErr(w, 503, errors.New("отправка уведомлений не запущена"))
		return
	}
	writeJSON(w, 200, s.notify.Test(*ch, actor(r)))
}

// GET /api/deliveries?channel=
func (s *Server) listDeliveries(w http.ResponseWriter, r *http.Request) {
	ch := r.URL.Query().Get("channel")
	out := []model.Delivery{}
	s.st.Read(func(d *store.Data) {
		for i := len(d.Deliveries) - 1; i >= 0 && len(out) < 300; i-- {
			if ch == "" || d.Deliveries[i].ChannelID == ch {
				out = append(out, *d.Deliveries[i])
			}
		}
	})
	writeJSON(w, 200, map[string]any{"items": out})
}
