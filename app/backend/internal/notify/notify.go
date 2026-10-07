// Package notify is backup notification: when nobody has taken a severe enough incident
// (PagerDuty is off, or did not take it in time), the people of its route get it by e-mail and
// from a Telegram bot, team channels also in Microsoft Teams and Zoom, with a link that
// acknowledges the incident. Once the incident is
// acknowledged or resolved, the same addresses get a short follow-up.
package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/textproto"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

const (
	ChannelEmail    = "email"
	ChannelTelegram = "telegram"
	ChannelTeams    = "teams"
	ChannelZoom     = "zoom"
)

var (
	ErrDisabled = errors.New("the channel is turned off")
	ErrNoSecret = errors.New("secrets are not available")
)

type Resolver interface {
	Resolve(ref string) (string, error)
}

// Results records what was sent on the timeline of the incident, and that backup notification
// or a follow-up of the incident was attempted, so that it is not handed over again.
// FallbackDue is asked right before sending: an incident taken meanwhile is not sent.
type Results interface {
	Note(ctx context.Context, id, kind, code string, args map[string]string) error
	FallbackDue(ctx context.Context, id string) (bool, error)
	FallbackDone(ctx context.Context, id string, sent []alert.Notified) error
	FollowUpDone(ctx context.Context, id, event string) error
}

type nopResults struct{}

func (nopResults) Note(context.Context, string, string, string, map[string]string) error { return nil }
func (nopResults) FallbackDue(context.Context, string) (bool, error)                     { return true, nil }
func (nopResults) FallbackDone(context.Context, string, []alert.Notified) error          { return nil }
func (nopResults) FollowUpDone(context.Context, string, string) error                    { return nil }

// job is a queued notification: backup notification or the follow-up of an incident.
type job struct {
	a        alert.Alert
	followUp bool
}

func (j job) key() string {
	if j.followUp {
		return "f:" + j.a.ID
	}
	return "n:" + j.a.ID
}

type Service struct {
	st      *store.Store
	sec     Resolver
	client  *http.Client
	results Results
	queue   chan job

	// queued: incidents in the queue or being delivered; the engine hands a pending incident
	// over again until it is reported, and a second copy is not queued.
	qmu    sync.Mutex
	queued map[string]bool

	mu    sync.RWMutex
	links *Links

	Retries int
	Backoff time.Duration
	now     func() time.Time
}

func New(st *store.Store, sec Resolver) *Service {
	return &Service{st: st, sec: sec, client: &http.Client{Timeout: DefaultHTTPTimeout}, results: nopResults{},
		queue: make(chan job, DefaultQueueSize), queued: map[string]bool{}, Retries: DefaultRetries, Backoff: DefaultBackoff, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) SetResults(r Results) { s.results = r }

// SetLinks turns on acknowledgement links.
func (s *Service) SetLinks(l *Links) {
	s.mu.Lock()
	s.links = l
	s.mu.Unlock()
}

func (s *Service) Links() *Links {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.links
}

// Fallback queues backup notification for an incident. The engine keeps the incident pending
// until Deliver reports it, and hands it over again later when the queue is full or the
// process restarted in between.
func (s *Service) Fallback(a alert.Alert) { s.enqueue(job{a: a}) }

// FollowUp queues the follow-up of an incident that was acknowledged or resolved
// (a.FollowUp) to the addresses backup notification reached; like Fallback, the engine hands
// it over again until DeliverFollowUp reports it.
func (s *Service) FollowUp(a alert.Alert) { s.enqueue(job{a: a, followUp: true}) }

func (s *Service) enqueue(j job) {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	if s.queued[j.key()] {
		return
	}
	select {
	case s.queue <- j:
		s.queued[j.key()] = true
	default:
		slog.Warn("backup notification queue is full: it is retried later", "alert", j.a.ID)
	}
}

func (s *Service) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-s.queue:
			if j.followUp {
				s.DeliverFollowUp(ctx, j.a)
			} else {
				s.Deliver(ctx, j.a)
			}
			s.qmu.Lock()
			delete(s.queued, j.key())
			s.qmu.Unlock()
		}
	}
}

// target is one address a notification goes to.
type target struct {
	channel   string
	address   string
	recipient string
	tz        *time.Location
}

type config struct {
	set    model.Alerting
	locale string
	// secret is the resolved secret of each enabled channel (the SMTP password, the bot
	// token), secretErr why a channel has none it needs.
	secret    map[string]string
	secretErr map[string]error
	msgs      messages
}

// settings reads the settings of the config, without the secrets and the templates.
func (s *Service) settings() config {
	var c config
	s.st.Read(func(d *store.Data) {
		c.set = d.Settings.Alerting
		c.set.Notify.ExtraEmails = slices.Clone(d.Settings.Alerting.Notify.ExtraEmails)
		c.set.Notify.ExtraTelegram = slices.Clone(d.Settings.Alerting.Notify.ExtraTelegram)
		c.set.Notify.ExtraTeams = slices.Clone(d.Settings.Alerting.Notify.ExtraTeams)
		c.set.Notify.ExtraZoom = slices.Clone(d.Settings.Alerting.Notify.ExtraZoom)
		c.set.Notify.Templates = maps.Clone(d.Settings.Alerting.Notify.Templates)
		c.locale = d.Settings.DefaultLocale
	})
	return c
}

func (s *Service) config() config {
	c := s.settings()
	c.msgs = newMessages(c.locale, c.set.Notify.Templates)
	c.secret, c.secretErr = map[string]string{}, map[string]error{}
	for _, ch := range channels {
		if !ch.Enabled(c.set.Notify) {
			continue
		}
		switch ref, err := ch.Secret(c.set.Notify); {
		case err != nil:
			c.secretErr[ch.Kind()] = err
		case ref != "":
			c.secret[ch.Kind()], c.secretErr[ch.Kind()] = s.resolve(ref)
		}
	}
	return c
}

func (s *Service) resolve(ref string) (string, error) {
	if s.sec == nil {
		return "", ErrNoSecret
	}
	v, err := s.sec.Resolve(ref)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNoSecret, err)
	}
	return v, nil
}

func loadTZ(name string) *time.Location {
	if name == "" {
		return time.UTC
	}
	if l, err := time.LoadLocation(name); err == nil {
		return l
	}
	return time.UTC
}

// targets: the people of the route (the team, or the owners of the item when the team has
// nobody) with their contacts, the team channel, then the extra addresses; each address once.
func (s *Service) targets(c config, a alert.Alert) []target {
	n := c.set.Notify
	var out []target
	seen := map[string]bool{}
	add := func(t target) {
		k := t.channel + "|" + strings.ToLower(t.address)
		if t.address == "" || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, t)
	}
	tz := map[string]string{}
	defTZ := ""
	s.st.Read(func(d *store.Data) {
		defTZ = d.Settings.DefaultTZ
		for _, p := range a.Route.Recipients() {
			if u := d.Users[p.UserID]; u != nil {
				tz[p.UserID] = u.Timezone
			}
		}
	})
	def := loadTZ(defTZ)
	on := enabled(n)
	for _, p := range a.Route.Recipients() {
		loc := def
		if tz[p.UserID] != "" {
			loc = loadTZ(tz[p.UserID])
		}
		for _, ch := range on {
			if addr := ch.Person(p); ch.Valid(addr) {
				add(target{channel: ch.Kind(), address: addr, recipient: UserRecipient(p.UserID), tz: loc})
			}
		}
	}
	if team := a.Route.Channel; team != nil {
		for _, ch := range on {
			if addr := ch.Team(*team); ch.Valid(addr) {
				add(target{channel: ch.Kind(), address: addr, recipient: ch.Recipient(addr), tz: def})
			}
		}
	}
	for _, ch := range on {
		for _, addr := range ch.Extra(n) {
			add(target{channel: ch.Kind(), address: addr, recipient: ch.Recipient(addr), tz: def})
		}
	}
	return out
}

// Target is an address backup notification goes to; a webhook URL is redacted.
type Target struct {
	Channel string `json:"channel"`
	Address string `json:"address"`
}

// PreviewTargets are the addresses backup notification about an incident of the route would
// go to now, by the same rules as delivery; none while every channel is off.
func (s *Service) PreviewTargets(r alert.Route) []Target {
	out := []Target{}
	for _, t := range s.targets(s.settings(), alert.Alert{Route: r}) {
		out = append(out, Target{Channel: t.channel, Address: ShowAddress(t.channel, t.address)})
	}
	return out
}

// On tells whether any channel is turned on.
func On(n model.Notify) bool { return len(enabled(n)) > 0 }

// enabled are the channels turned on, in their order.
func enabled(n model.Notify) []channel {
	var out []channel
	for _, ch := range channels {
		if ch.Enabled(n) {
			out = append(out, ch)
		}
	}
	return out
}

// Deliver sends backup notification for an incident, records the outcome on its timeline and
// then reports the attempt with the addresses reached. Delivery cut short by shutdown is not
// reported: the incident stays pending and is sent after the restart. An incident acknowledged
// or resolved since it was queued is not sent.
func (s *Service) Deliver(ctx context.Context, a alert.Alert) {
	due, err := s.results.FallbackDue(ctx, a.ID)
	if err != nil {
		if !errors.Is(err, alert.ErrNotFound) {
			slog.Warn("backup notification not checked: it is retried later", "alert", a.ID, "err", err)
		}
		return
	}
	if !due {
		return
	}
	c := s.config()
	targets := s.targets(c, a)
	var reached []alert.Notified
	defer func() {
		if ctx.Err() != nil {
			return
		}
		if err := s.results.FallbackDone(ctx, a.ID, reached); err != nil && !errors.Is(err, alert.ErrNotFound) {
			slog.Warn("backup notification not marked as sent", "alert", a.ID, "err", err)
		}
	}()
	if len(targets) == 0 {
		s.note(ctx, a.ID, "notify_none", nil)
		return
	}
	sent := map[string][]string{}
	for _, t := range targets {
		err := s.send(ctx, c, s.compose(c, a, t), t)
		shown := ShowAddress(t.channel, t.address)
		if err != nil {
			slog.Warn("backup notification not sent", "alert", a.ID, "channel", t.channel, "to", shown, "err", err)
			s.note(ctx, a.ID, "notify_failed", map[string]string{"channel": t.channel, "to": shown, "error": err.Error()})
			continue
		}
		sent[t.channel] = append(sent[t.channel], shown)
		reached = append(reached, alert.Notified{Channel: t.channel, Address: t.address, Recipient: t.recipient})
	}
	for _, ch := range Channels() {
		if len(sent[ch]) > 0 {
			s.note(ctx, a.ID, "notify_sent", map[string]string{"channel": ch, "to": strings.Join(sent[ch], ", ")})
		}
	}
}

func (s *Service) note(ctx context.Context, id, code string, args map[string]string) {
	if err := s.results.Note(ctx, id, alert.KindFallback, code, args); err != nil && !errors.Is(err, alert.ErrNotFound) {
		slog.Warn("backup notification not recorded", "alert", id, "err", err)
	}
}

func (s *Service) send(ctx context.Context, c config, m composed, t target) error {
	ch := channelOf(t.channel)
	if ch == nil {
		return fmt.Errorf("%w: %s", ErrUnknownChannel, t.channel)
	}
	attempt := func() error {
		if err := c.secretErr[t.channel]; err != nil {
			return errPermanent{err}
		}
		return ch.Send(ctx, s, c.set.Notify, c.secret[t.channel], t.address, m)
	}
	wait := s.Backoff
	var err error
	for i := 1; i <= max(1, s.Retries); i++ {
		if err = attempt(); err == nil {
			return nil
		}
		var perm errPermanent
		if errors.As(err, &perm) {
			return perm.error
		}
		var smtpErr *textproto.Error
		if errors.As(err, &smtpErr) && smtpErr.Code >= 500 {
			return err
		}
		if i < s.Retries {
			select {
			case <-ctx.Done():
				return err
			case <-time.After(wait):
			}
			wait *= 2
		}
	}
	return err
}

// composed is a message: the parts of its template and its links, which Teams shows as buttons.
type composed struct {
	subject, text, html string
	links               []link
}

type link struct{ title, url string }

// withLinks adds the links of a message in the language of the messages.
func (c composed) withLinks(locale string, m Message) composed {
	w := lang(locale)
	for _, l := range []link{{w["ack"], m.Ack}, {w["open"], m.Open}, {"Grafana", m.Grafana}} {
		if l.url != "" {
			c.links = append(c.links, l)
		}
	}
	return c
}

// messages of the config: the built-in templates of its language with the overrides.
func (c config) messages() messages {
	if c.msgs.builtin != nil {
		return c.msgs
	}
	return newMessages(c.locale, nil)
}

// compose is backup notification about an incident for one address.
func (s *Service) compose(c config, a alert.Alert, t target) composed {
	m := incidentMessage(a, t.tz)
	if base := strings.TrimRight(c.set.PublicURL, "/"); base != "" {
		m.Open = model.IncidentURL(base, a.ID)
		if c.set.Grafana.DashboardURL != "" {
			m.Grafana = model.GrafanaHopURL(base, a.ID)
		}
		if l := s.Links(); l != nil {
			m.Ack = base + "/ack/" + l.Sign(a.ID, t.recipient, s.now().Add(LinkTTL))
		}
	}
	return c.messages().render("fallback", m).withLinks(c.locale, m)
}

// incidentMessage is what the templates see of an incident.
func incidentMessage(a alert.Alert, tz *time.Location) Message {
	m := Message{ID: a.ID, Title: a.Title, Severity: a.Severity, CI: a.CIName, Signal: a.Signal, Team: a.Route.Team,
		Opened: formatTime(&a.OpenedAt, tz), PDState: a.PD.State, PDErrorCode: a.PD.ErrorCode, PDError: a.PD.Error,
		Event: a.FollowUp, AckedBy: a.AckedBy, AckedAt: formatTime(a.AckedAt, tz), ResolvedBy: a.ResolvedBy, ResolvedAt: formatTime(a.ResolvedAt, tz)}
	for _, r := range a.Route.Services {
		m.Services = append(m.Services, r.Name)
	}
	return m
}

// composeFollowUp is the short message telling that the incident was taken (who and when) or
// resolved, with a link to it.
func (s *Service) composeFollowUp(c config, a alert.Alert, tz *time.Location) composed {
	m := incidentMessage(a, tz)
	if base := strings.TrimRight(c.set.PublicURL, "/"); base != "" {
		m.Open = model.IncidentURL(base, a.ID)
	}
	return c.messages().render("followup", m).withLinks(c.locale, m)
}

// DeliverFollowUp tells the addresses backup notification reached that the incident was
// acknowledged or resolved, records the outcome on its timeline and reports the attempt.
// Addresses of a channel turned off since are skipped.
func (s *Service) DeliverFollowUp(ctx context.Context, a alert.Alert) {
	if a.FollowUp == "" {
		return
	}
	c := s.config()
	defer func() {
		if ctx.Err() != nil {
			return
		}
		if err := s.results.FollowUpDone(ctx, a.ID, a.FollowUp); err != nil && !errors.Is(err, alert.ErrNotFound) {
			slog.Warn("follow-up not marked as sent", "alert", a.ID, "err", err)
		}
	}()
	tz := map[string]string{}
	defTZ := ""
	s.st.Read(func(d *store.Data) {
		defTZ = d.Settings.DefaultTZ
		for _, n := range a.Notified {
			if kind, id, _ := strings.Cut(n.Recipient, ":"); kind == "u" {
				if u := d.Users[id]; u != nil {
					tz[n.Recipient] = u.Timezone
				}
			}
		}
	})
	sent := map[string][]string{}
	for _, n := range a.Notified {
		if ch := channelOf(n.Channel); ch == nil || !ch.Enabled(c.set.Notify) {
			continue
		}
		loc := loadTZ(defTZ)
		if tz[n.Recipient] != "" {
			loc = loadTZ(tz[n.Recipient])
		}
		t := target{channel: n.Channel, address: n.Address, recipient: n.Recipient, tz: loc}
		shown := ShowAddress(t.channel, t.address)
		if err := s.send(ctx, c, s.composeFollowUp(c, a, loc), t); err != nil {
			slog.Warn("follow-up not sent", "alert", a.ID, "channel", t.channel, "to", shown, "err", err)
			s.note(ctx, a.ID, "notify_followup_failed", map[string]string{"channel": t.channel, "to": shown, "event": a.FollowUp, "error": err.Error()})
			continue
		}
		sent[t.channel] = append(sent[t.channel], shown)
	}
	for _, ch := range Channels() {
		if len(sent[ch]) > 0 {
			s.note(ctx, a.ID, "notify_followup", map[string]string{"channel": ch, "to": strings.Join(sent[ch], ", "), "event": a.FollowUp})
		}
	}
}

// Test sends a test message to an address of a channel with the saved settings; the map tells
// more for the interface (the name of the Telegram bot).
func (s *Service) Test(ctx context.Context, kind, to string) (map[string]string, error) {
	ch := channelOf(kind)
	if ch == nil {
		return nil, ErrUnknownChannel
	}
	c := s.config()
	if !ch.Enabled(c.set.Notify) {
		return nil, ErrDisabled
	}
	if err := c.secretErr[kind]; err != nil {
		return nil, err
	}
	info, err := ch.Test(ctx, s, c.set.Notify, c.secret[kind], to, s.composeTest(c))
	return info, unwrap(err)
}

func (s *Service) composeTest(c config) composed {
	return c.messages().render("test", Message{})
}

// TestEmail sends a test message with the saved settings.
func (s *Service) TestEmail(ctx context.Context, to string) error {
	_, err := s.Test(ctx, ChannelEmail, to)
	return err
}

// TestTelegram sends a test message to a chat with the saved settings and returns the name of
// the bot.
func (s *Service) TestTelegram(ctx context.Context, chat string) (string, error) {
	info, err := s.Test(ctx, ChannelTelegram, chat)
	return info["bot"], err
}

func unwrap(err error) error {
	var perm errPermanent
	if errors.As(err, &perm) {
		return perm.error
	}
	return err
}
