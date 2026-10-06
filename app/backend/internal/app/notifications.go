package app

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/credentials"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/notify"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

const (
	notifySecretPath = "notify"
	maxExtraTargets  = 50
	// maxFallbackDelay bounds the wait of backup notification: an hour.
	maxFallbackDelay = 3600
)

// NotificationsService keeps the settings of backup notification; the SMTP password and the
// bot token are kept in OpenBao.
type NotificationsService struct {
	st      *store.Store
	secrets Secrets
	n       *notify.Service
}

func NewNotificationsService(st *store.Store, secrets Secrets, n *notify.Service) *NotificationsService {
	return &NotificationsService{st: st, secrets: secrets, n: n}
}

type NotifyView struct {
	model.Notify
	HasPassword bool   `json:"has_password"`
	HasToken    bool   `json:"has_token"`
	PublicURL   string `json:"public_url"`
	// Links: acknowledgement links are put in messages (the public address is set and the
	// signing key is ready).
	Links bool `json:"links"`
	// PDEnabled: PagerDuty is on; AutoDelaySeconds is the wait of backup notification when no
	// delay is set (DelaySeconds is null): 2 minutes with PagerDuty, none without it.
	PDEnabled        bool `json:"pd_enabled"`
	AutoDelaySeconds int  `json:"auto_delay_seconds"`
}

func (s *NotificationsService) View() NotifyView {
	var n model.Notify
	var pub string
	var pdOn bool
	s.st.Read(func(d *store.Data) {
		n = d.Settings.Alerting.Notify
		n.ExtraEmails = slices.Clone(n.ExtraEmails)
		n.ExtraTelegram = slices.Clone(n.ExtraTelegram)
		pub = d.Settings.Alerting.PublicURL
		pdOn = d.Settings.Alerting.PagerDuty.Enabled
	})
	auto := 0
	if pdOn {
		auto = int(alert.DefaultFallbackAfter.Seconds())
	}
	if n.ExtraEmails == nil {
		n.ExtraEmails = []string{}
	}
	if n.ExtraTelegram == nil {
		n.ExtraTelegram = []string{}
	}
	if n.Email.Security == "" {
		n.Email.Security = model.SMTPStartTLS
	}
	if n.MinSeverity == "" {
		n.MinSeverity = alert.DefaultFallbackSeverity
	}
	return NotifyView{Notify: n, HasPassword: n.Email.PasswordRef != "", HasToken: n.Telegram.TokenRef != "", PublicURL: pub,
		Links: pub != "" && s.n.Links() != nil, PDEnabled: pdOn, AutoDelaySeconds: auto}
}

type EmailInput struct {
	Enabled       bool   `json:"enabled"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Security      string `json:"security"`
	SkipVerify    bool   `json:"skip_verify"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	ClearPassword bool   `json:"clear_password"`
	From          string `json:"from"`
}

type TelegramInput struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token"`
	APIURL  string `json:"api_url"`
}

type NotifyInput struct {
	Email         EmailInput    `json:"email"`
	Telegram      TelegramInput `json:"telegram"`
	ExtraEmails   []string      `json:"extra_emails"`
	ExtraTelegram []string      `json:"extra_telegram"`
	// DelaySeconds: null is automatic (2 minutes with PagerDuty, at once without it), 0 at once.
	DelaySeconds *int `json:"delay_seconds"`
	// MinSeverity: empty is error.
	MinSeverity string `json:"min_severity"`
}

func cleanList(in []string, valid func(string) bool, code string) ([]string, error) {
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || slices.Contains(out, v) {
			continue
		}
		if !valid(v) {
			return nil, invalid(code, fmt.Errorf("%q is not valid", v))
		}
		out = append(out, v)
	}
	if len(out) > maxExtraTargets {
		return nil, invalid("too_many_targets", nil)
	}
	return out, nil
}

func (s *NotificationsService) Save(ctx context.Context, actor string, in NotifyInput) (NotifyView, error) {
	var n model.Notify
	s.st.Read(func(d *store.Data) { n = d.Settings.Alerting.Notify })
	e := in.Email
	e.Host = strings.TrimSpace(e.Host)
	e.From = strings.TrimSpace(e.From)
	switch e.Security {
	case "", model.SMTPStartTLS, model.SMTPTLS, model.SMTPNone:
	default:
		return NotifyView{}, invalid("security_invalid", nil)
	}
	if e.Port < 0 || e.Port > 65535 {
		return NotifyView{}, invalid("port_invalid", nil)
	}
	if strings.ContainsAny(e.Host, " /\r\n") {
		return NotifyView{}, invalid("host_invalid", nil)
	}
	if e.From != "" && !validFrom(e.From) {
		return NotifyView{}, invalid("from_invalid", nil)
	}
	if e.Enabled && (e.Host == "" || e.From == "") {
		return NotifyView{}, invalid("email_incomplete", nil)
	}
	api, err := optionalURL(in.Telegram.APIURL)
	if err != nil {
		return NotifyView{}, invalid("url_invalid", err)
	}
	extraEmails, err := cleanList(in.ExtraEmails, notify.ValidEmail, "email_invalid")
	if err != nil {
		return NotifyView{}, err
	}
	extraTelegram, err := cleanList(in.ExtraTelegram, notify.ValidChat, "telegram_invalid")
	if err != nil {
		return NotifyView{}, err
	}
	if in.DelaySeconds != nil && (*in.DelaySeconds < 0 || *in.DelaySeconds > maxFallbackDelay) {
		return NotifyView{}, invalid("delay_invalid", nil)
	}
	if in.MinSeverity != "" && alert.SeverityRank(in.MinSeverity) == 0 {
		return NotifyView{}, invalid("severity_invalid", nil)
	}
	token := strings.TrimSpace(in.Telegram.Token)
	if in.Telegram.Enabled && token == "" && n.Telegram.TokenRef == "" {
		return NotifyView{}, invalid("token_required", nil)
	}
	pwRef, tokenRef := n.Email.PasswordRef, n.Telegram.TokenRef
	if in.Email.ClearPassword || strings.TrimSpace(e.Username) == "" {
		pwRef = ""
	}
	if e.Password != "" {
		if pwRef, err = s.put(ctx, "smtp_password", e.Password); err != nil {
			return NotifyView{}, err
		}
	}
	if token != "" {
		if tokenRef, err = s.put(ctx, "telegram_token", token); err != nil {
			return NotifyView{}, err
		}
	}
	n.Email = model.EmailChannel{Enabled: e.Enabled, Host: e.Host, Port: e.Port, Security: e.Security, SkipVerify: e.SkipVerify,
		Username: strings.TrimSpace(e.Username), PasswordRef: pwRef, From: e.From}
	if n.Email.Security == "" {
		n.Email.Security = model.SMTPStartTLS
	}
	n.Telegram = model.TelegramChannel{Enabled: in.Telegram.Enabled, TokenRef: tokenRef, APIURL: api}
	n.ExtraEmails, n.ExtraTelegram = extraEmails, extraTelegram
	n.DelaySeconds, n.MinSeverity = in.DelaySeconds, in.MinSeverity
	now := time.Now().UTC()
	n.UpdatedAt, n.UpdatedBy = &now, actor
	s.st.Write(func(d *store.Data) {
		d.Settings.Alerting.Notify = n
		d.AddAudit(store.AuditEntry{Actor: actor, Action: "settings.notify", Detail: fmt.Sprintf("email=%v telegram=%v delay=%s min_severity=%s", n.Email.Enabled, n.Telegram.Enabled, delayText(n.DelaySeconds), n.MinSeverity)})
	})
	return s.View(), nil
}

func delayText(v *int) string {
	if v == nil {
		return "auto"
	}
	return fmt.Sprintf("%ds", *v)
}

func validFrom(v string) bool {
	if strings.ContainsAny(v, "\r\n") {
		return false
	}
	i := strings.LastIndex(v, "<")
	if i >= 0 && strings.HasSuffix(v, ">") {
		v = v[i+1 : len(v)-1]
	}
	return notify.ValidEmail(v)
}

func (s *NotificationsService) put(ctx context.Context, key, value string) (string, error) {
	if s.secrets == nil {
		return "", credentials.ErrUnavailable
	}
	ref, err := s.secrets.PutRef(ctx, notifySecretPath, key, value)
	if err != nil {
		return "", fmt.Errorf("%w: %v", credentials.ErrUnavailable, err)
	}
	return ref, nil
}

func (a *App) registerNotifications(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/notifications", a.authed(a.can("settings.alerting:view", a.notifyView)))
	mux.HandleFunc("PUT /api/notifications", a.authed(a.can("settings.alerting:edit", a.notifySave)))
	mux.HandleFunc("POST /api/notifications/test", a.authed(a.can("settings.alerting:test", a.notifyTest)))
	mux.HandleFunc("GET /ack/{token}", a.ackPage)
	mux.HandleFunc("POST /ack/{token}", a.ackPage)
}

func (a *App) notifyView(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, a.notifications.View())
}

func (a *App) notifySave(w http.ResponseWriter, r *http.Request) {
	var in NotifyInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	out, err := a.notifications.Save(r.Context(), current(r).user.Username, in)
	settingsRespond(w, out, err)
}

type notifyTestInput struct {
	Channel string `json:"channel"`
	To      string `json:"to"`
}

// notifyTest sends a test message with the saved settings, by default to the user who asks.
func (a *App) notifyTest(w http.ResponseWriter, r *http.Request) {
	var in notifyTestInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	u := current(r).user
	to := strings.TrimSpace(in.To)
	out := map[string]any{"ok": true}
	var err error
	switch in.Channel {
	case notify.ChannelEmail:
		if to == "" {
			to = u.Email
		}
		if !notify.ValidEmail(to) {
			writeError(w, invalid("email_invalid", nil))
			return
		}
		err = a.notifier.TestEmail(r.Context(), to)
	case notify.ChannelTelegram:
		if to == "" {
			to = u.Telegram
		}
		if !notify.ValidChat(to) {
			writeError(w, invalid("telegram_invalid", nil))
			return
		}
		var bot string
		bot, err = a.notifier.TestTelegram(r.Context(), to)
		out["bot"] = bot
	default:
		writeError(w, invalid("channel_invalid", nil))
		return
	}
	switch {
	case errors.Is(err, notify.ErrDisabled):
		writeError(w, invalid("channel_disabled", nil))
	case errors.Is(err, notify.ErrNoSecret):
		writeError(w, credentials.ErrUnavailable)
	case err != nil:
		httpx.Error(w, http.StatusBadGateway, "notify_failed", err)
	default:
		out["to"] = to
		httpx.JSON(w, http.StatusOK, out)
	}
}

var ackTemplate = template.Must(template.New("ack").Parse(`<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex"><title>Umbrella · {{.ID}}</title>
<style>
:root{color-scheme:light dark;--bg:#f5f7fb;--card:#fff;--text:#111827;--muted:#6b7280;--accent:#2563eb;--border:#e5e7eb}
@media (prefers-color-scheme:dark){:root{--bg:#0b1020;--card:#151b2e;--text:#e5e7eb;--muted:#9ca3af;--accent:#60a5fa;--border:#273049}}
body{margin:0;min-height:100vh;display:grid;place-items:center;background:var(--bg);color:var(--text);font:15px/1.5 system-ui,sans-serif;padding:16px;box-sizing:border-box}
main{background:var(--card);border:1px solid var(--border);border-radius:14px;padding:24px;max-width:460px;width:100%;box-sizing:border-box}
h1{font-size:18px;margin:0 0 4px}p{margin:8px 0}.muted{color:var(--muted)}
button{margin-top:12px;background:var(--accent);color:#fff;border:0;border-radius:8px;padding:10px 18px;font:inherit;font-weight:600;cursor:pointer}
a{color:var(--accent)}
</style></head><body><main>
<p class="muted">Umbrella</p>
{{if .Title}}<h1>{{.ID}} · {{.Title}}</h1>{{end}}
<p>{{.Message}}</p>
{{if .Form}}<form method="post"><button type="submit">{{.Button}}</button></form>{{end}}
{{if .Open}}<p><a href="{{.Open}}">{{.OpenText}}</a></p>{{end}}
</main></body></html>`))

var ackWords = map[string]map[string]string{
	"ru": {"ask": "Подтвердить, что вы взяли инцидент в работу?", "button": "Подтвердить", "done": "Инцидент подтверждён. Спасибо!",
		"already": "Инцидент уже подтверждён.", "resolved": "Инцидент уже решён.", "bad": "Ссылка недействительна или устарела.",
		"unavailable": "Сервис инцидентов сейчас недоступен. Попробуйте позже.", "open": "Открыть в Umbrella"},
	"en": {"ask": "Acknowledge that you are on this incident?", "button": "Acknowledge", "done": "The incident is acknowledged. Thank you!",
		"already": "The incident is already acknowledged.", "resolved": "The incident is already resolved.", "bad": "The link is not valid or has expired.",
		"unavailable": "Incidents are not available now. Try again later.", "open": "Open in Umbrella"},
}

// ackPage acknowledges an incident from the link of a notification. GET only shows the page,
// because messengers open links to make previews; the button posts the form.
func (a *App) ackPage(w http.ResponseWriter, r *http.Request) {
	lang := "ru"
	if a.settings.Get().DefaultLocale == "en" {
		lang = "en"
	}
	words := ackWords[lang]
	page := struct {
		Lang, ID, Title, Message, Button, Open, OpenText string
		Form                                             bool
	}{Lang: lang, Button: words["button"], OpenText: words["open"]}
	render := func(status int) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_ = ackTemplate.Execute(w, page)
	}
	if a.alerts == nil || !a.ingestReady() {
		page.Message = words["unavailable"]
		render(http.StatusServiceUnavailable)
		return
	}
	id, who, err := a.notifier.Links().Verify(r.PathValue("token"), time.Now())
	if err != nil {
		page.Message = words["bad"]
		render(http.StatusForbidden)
		return
	}
	al, _, err := a.alerts.Get(r.Context(), id)
	if err != nil {
		page.Message = words["bad"]
		render(http.StatusNotFound)
		return
	}
	page.ID, page.Title = al.ID, al.Title
	if pub := a.settings.Get().Alerting.PublicURL; pub != "" {
		page.Open = strings.TrimRight(pub, "/") + "/incidents?id=" + al.ID
	}
	switch {
	case al.Status == alert.StatusResolved:
		page.Message = words["resolved"]
		render(http.StatusOK)
		return
	case al.Status == alert.StatusAcknowledged:
		page.Message = words["already"]
		render(http.StatusOK)
		return
	case r.Method == http.MethodGet:
		page.Message, page.Form = words["ask"], true
		render(http.StatusOK)
		return
	}
	actor := a.linkActor(who)
	if _, err := a.alerts.Act(r.Context(), id, "ack", actor, ""); err != nil {
		if errors.Is(err, alert.ErrNotOpen) {
			page.Message = words["already"]
			render(http.StatusOK)
			return
		}
		page.Message = words["unavailable"]
		render(http.StatusInternalServerError)
		return
	}
	page.Message = words["done"]
	render(http.StatusOK)
}

// linkActor names who acknowledged by a link: the user's name, or the address the message
// went to.
func (a *App) linkActor(who string) string {
	kind, v, _ := strings.Cut(who, ":")
	switch kind {
	case "u":
		name := ""
		a.deps.Store.Read(func(d *store.Data) {
			if u := d.Users[v]; u != nil {
				name = u.Username
			}
		})
		if name != "" {
			return name
		}
	case "e":
		return v
	case "t":
		return "telegram " + v
	}
	return "link"
}
