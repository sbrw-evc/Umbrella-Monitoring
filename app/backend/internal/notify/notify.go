// Package notify is backup notification: when PagerDuty has not taken an error or critical
// incident in time, the people of its route get it by e-mail and from a Telegram bot, with a
// link that acknowledges the incident.
package notify

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/textproto"
	"net/url"
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
	queueSize       = 1000
)

var (
	ErrDisabled = errors.New("the channel is turned off")
	ErrNoSecret = errors.New("secrets are not available")
)

type Resolver interface {
	Resolve(ref string) (string, error)
}

// Results records what was sent on the timeline of the incident.
type Results interface {
	Note(ctx context.Context, id, kind, code string, args map[string]string) error
}

type nopResults struct{}

func (nopResults) Note(context.Context, string, string, string, map[string]string) error { return nil }

type Service struct {
	st      *store.Store
	sec     Resolver
	client  *http.Client
	results Results
	queue   chan alert.Alert

	mu    sync.RWMutex
	links *Links

	Retries int
	Backoff time.Duration
	now     func() time.Time
}

func New(st *store.Store, sec Resolver) *Service {
	return &Service{st: st, sec: sec, client: &http.Client{Timeout: 15 * time.Second}, results: nopResults{},
		queue: make(chan alert.Alert, queueSize), Retries: 3, Backoff: 2 * time.Second, now: func() time.Time { return time.Now().UTC() }}
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

// Fallback queues backup notification for an incident; the engine calls it once per incident.
func (s *Service) Fallback(a alert.Alert) {
	select {
	case s.queue <- a:
	default:
		slog.Warn("backup notification queue is full", "alert", a.ID)
	}
}

func (s *Service) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case a := <-s.queue:
			s.Deliver(ctx, a)
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
	set      model.Alerting
	locale   string
	email    string // the SMTP password
	emailErr error
	token    string // the Telegram bot token
	tokenErr error
}

func (s *Service) config() config {
	var c config
	s.st.Read(func(d *store.Data) {
		c.set = d.Settings.Alerting
		c.set.Notify.ExtraEmails = slices.Clone(d.Settings.Alerting.Notify.ExtraEmails)
		c.set.Notify.ExtraTelegram = slices.Clone(d.Settings.Alerting.Notify.ExtraTelegram)
		c.locale = d.Settings.DefaultLocale
	})
	n := c.set.Notify
	if n.Email.Enabled && n.Email.PasswordRef != "" {
		c.email, c.emailErr = s.resolve(n.Email.PasswordRef)
	}
	if n.Telegram.Enabled {
		if n.Telegram.TokenRef == "" {
			c.tokenErr = errors.New("the Telegram bot token is not set")
		} else {
			c.token, c.tokenErr = s.resolve(n.Telegram.TokenRef)
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
// nobody) with their contacts, then the extra addresses; each address once.
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
	for _, p := range a.Route.Recipients() {
		loc := def
		if tz[p.UserID] != "" {
			loc = loadTZ(tz[p.UserID])
		}
		if n.Email.Enabled && ValidEmail(p.Email) {
			add(target{channel: ChannelEmail, address: p.Email, recipient: UserRecipient(p.UserID), tz: loc})
		}
		if n.Telegram.Enabled && ValidChat(p.Telegram) {
			add(target{channel: ChannelTelegram, address: p.Telegram, recipient: UserRecipient(p.UserID), tz: loc})
		}
	}
	if n.Email.Enabled {
		for _, e := range n.ExtraEmails {
			add(target{channel: ChannelEmail, address: e, recipient: EmailRecipient(e), tz: def})
		}
	}
	if n.Telegram.Enabled {
		for _, chat := range n.ExtraTelegram {
			add(target{channel: ChannelTelegram, address: chat, recipient: TelegramRecipient(chat), tz: def})
		}
	}
	return out
}

// Deliver sends backup notification for an incident and records the outcome on its timeline.
func (s *Service) Deliver(ctx context.Context, a alert.Alert) {
	c := s.config()
	targets := s.targets(c, a)
	if len(targets) == 0 {
		s.note(ctx, a.ID, "notify_none", nil)
		return
	}
	sent := map[string][]string{}
	for _, t := range targets {
		err := s.send(ctx, c, a, t)
		if err != nil {
			slog.Warn("backup notification not sent", "alert", a.ID, "channel", t.channel, "to", t.address, "err", err)
			s.note(ctx, a.ID, "notify_failed", map[string]string{"channel": t.channel, "to": t.address, "error": err.Error()})
			continue
		}
		sent[t.channel] = append(sent[t.channel], t.address)
	}
	for _, ch := range []string{ChannelEmail, ChannelTelegram} {
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

func (s *Service) send(ctx context.Context, c config, a alert.Alert, t target) error {
	m := s.compose(c, a, t)
	attempt := func() error {
		switch t.channel {
		case ChannelEmail:
			if c.emailErr != nil {
				return errPermanent{c.emailErr}
			}
			return sendMail(ctx, c.set.Notify.Email, c.email, t.address, m.subject, m.text)
		default:
			if c.tokenErr != nil {
				return errPermanent{c.tokenErr}
			}
			return sendTelegram(ctx, s.client, c.set.Notify.Telegram.APIURL, c.token, t.address, m.html)
		}
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

type composed struct{ subject, text, html string }

var words = map[string]map[string]string{
	"ru": {
		"head": "PagerDuty не принял инцидент — резервное оповещение", "severity": "Важность", "ci": "КЕ", "signal": "Сигнал",
		"service": "Сервис", "team": "Команда", "opened": "Открыт", "pd": "PagerDuty", "ack": "Подтвердить", "open": "Открыть в Umbrella",
		"critical": "критично", "error": "ошибка", "warning": "предупреждение", "info": "инфо",
		"pd_failed": "ошибка доставки", "pd_pending": "не ответил", "why": "Вы получили это письмо, потому что входите в команду, отвечающую за сервис, или отвечаете за КЕ.",
		"test": "Проверка резервного оповещения Umbrella", "test_body": "Это проверочное сообщение. Если вы его видите, канал настроен.",
	},
	"en": {
		"head": "PagerDuty did not take the incident — backup notification", "severity": "Severity", "ci": "CI", "signal": "Signal",
		"service": "Service", "team": "Team", "opened": "Opened", "pd": "PagerDuty", "ack": "Acknowledge", "open": "Open in Umbrella",
		"critical": "critical", "error": "error", "warning": "warning", "info": "info",
		"pd_failed": "delivery failed", "pd_pending": "no answer", "why": "You get this because you are in the team that owns the service or you are responsible for the CI.",
		"test": "Umbrella backup notification test", "test_body": "This is a test message. If you see it, the channel is set up.",
	},
}

func lang(locale string) map[string]string {
	if locale == "en" {
		return words["en"]
	}
	return words["ru"]
}

func (s *Service) compose(c config, a alert.Alert, t target) composed {
	w := lang(c.locale)
	base := strings.TrimRight(c.set.PublicURL, "/")
	type line struct{ k, v string }
	lines := []line{{w["severity"], w[a.Severity]}, {w["ci"], a.CIName}}
	if a.Signal != "" && a.Signal != a.Title {
		lines = append(lines, line{w["signal"], a.Signal})
	}
	if len(a.Route.Services) > 0 {
		names := make([]string, 0, len(a.Route.Services))
		for _, r := range a.Route.Services {
			names = append(names, r.Name)
		}
		lines = append(lines, line{w["service"], strings.Join(names, ", ")})
	}
	if a.Route.Team != nil {
		lines = append(lines, line{w["team"], a.Route.Team.Name})
	}
	lines = append(lines, line{w["opened"], a.OpenedAt.In(t.tz).Format("02.01.2006 15:04 MST")})
	pd := w["pd_pending"]
	if a.PD.State == alert.PDFailed {
		pd = w["pd_failed"]
		if a.PD.Error != "" {
			pd += ": " + a.PD.Error
		}
	}
	lines = append(lines, line{w["pd"], pd})
	var open, ack string
	if base != "" {
		open = base + "/incidents?id=" + url.QueryEscape(a.ID)
		if l := s.Links(); l != nil {
			ack = base + "/ack/" + l.Sign(a.ID, t.recipient, s.now().Add(LinkTTL))
		}
	}
	title := a.ID + " · " + a.Title
	var tb, hb strings.Builder
	tb.WriteString(w["head"] + "\n\n" + title + "\n\n")
	hb.WriteString("🚨 <b>" + html.EscapeString(w["head"]) + "</b>\n\n<b>" + html.EscapeString(title) + "</b>\n")
	for _, l := range lines {
		tb.WriteString(l.k + ": " + l.v + "\n")
		hb.WriteString(html.EscapeString(l.k) + ": " + html.EscapeString(l.v) + "\n")
	}
	if ack != "" || open != "" {
		tb.WriteString("\n")
		hb.WriteString("\n")
	}
	if ack != "" {
		tb.WriteString(w["ack"] + ": " + ack + "\n")
		hb.WriteString(`<a href="` + html.EscapeString(ack) + `">` + html.EscapeString(w["ack"]) + "</a>")
		if open != "" {
			hb.WriteString(" · ")
		}
	}
	if open != "" {
		tb.WriteString(w["open"] + ": " + open + "\n")
		hb.WriteString(`<a href="` + html.EscapeString(open) + `">` + html.EscapeString(w["open"]) + "</a>")
	}
	tb.WriteString("\n-- \n" + w["why"] + "\n")
	return composed{subject: "[Umbrella] " + title + " (" + w[a.Severity] + ")", text: tb.String(), html: hb.String()}
}

// TestEmail sends a test message with the saved settings.
func (s *Service) TestEmail(ctx context.Context, to string) error {
	c := s.config()
	if !c.set.Notify.Email.Enabled {
		return ErrDisabled
	}
	if c.emailErr != nil {
		return c.emailErr
	}
	w := lang(c.locale)
	return sendMail(ctx, c.set.Notify.Email, c.email, to, w["test"], w["test_body"]+"\n")
}

// TestTelegram sends a test message to a chat with the saved settings and returns the name of
// the bot.
func (s *Service) TestTelegram(ctx context.Context, chat string) (string, error) {
	c := s.config()
	if !c.set.Notify.Telegram.Enabled {
		return "", ErrDisabled
	}
	if c.tokenErr != nil {
		return "", c.tokenErr
	}
	me, err := telegramCall(ctx, s.client, c.set.Notify.Telegram.APIURL, c.token, "getMe", map[string]any{})
	if err != nil {
		return "", unwrap(err)
	}
	w := lang(c.locale)
	if err := sendTelegram(ctx, s.client, c.set.Notify.Telegram.APIURL, c.token, chat,
		"✅ <b>"+html.EscapeString(w["test"])+"</b>\n"+html.EscapeString(w["test_body"])); err != nil {
		return me.Result.Username, unwrap(err)
	}
	return me.Result.Username, nil
}

func unwrap(err error) error {
	var perm errPermanent
	if errors.As(err, &perm) {
		return perm.error
	}
	return err
}
