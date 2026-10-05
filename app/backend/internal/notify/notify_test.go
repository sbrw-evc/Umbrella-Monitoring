package notify_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/notify"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/notify/notifytest"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type secrets map[string]string

func (s secrets) Resolve(ref string) (string, error) {
	if v, ok := s[ref]; ok {
		return v, nil
	}
	return "", errors.New("no secret")
}

type note struct {
	code string
	args map[string]string
}

type results struct {
	mu    sync.Mutex
	notes []note
}

func (r *results) Note(_ context.Context, _, _, code string, args map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notes = append(r.notes, note{code, args})
	return nil
}

func setup(t *testing.T) (*notify.Service, *store.Store, *notifytest.SMTP, *notifytest.Telegram, *results) {
	smtp := notifytest.NewSMTP(t)
	tg := notifytest.NewTelegram(t)
	st := store.New()
	st.Write(func(d *store.Data) {
		d.Settings.DefaultLocale = "ru"
		d.Settings.DefaultTZ = "Europe/Moscow"
		d.Users["u1"] = &model.User{ID: "u1", Username: "ivanov", Timezone: "Asia/Novosibirsk"}
		d.Settings.Alerting = model.Alerting{PublicURL: "https://umbrella.example.com", Notify: model.Notify{
			Email:         model.EmailChannel{Enabled: true, Host: smtp.Host(), Port: smtp.Port(), Security: model.SMTPNone, Username: "relay", PasswordRef: "pw", From: "Umbrella <umbrella@example.com>"},
			Telegram:      model.TelegramChannel{Enabled: true, TokenRef: "tg", APIURL: tg.URL()},
			ExtraEmails:   []string{"duty@example.com"},
			ExtraTelegram: []string{"-100200"},
		}}
	})
	s := notify.New(st, secrets{"pw": "secret", "tg": tg.Token})
	s.Backoff = time.Millisecond
	s.SetLinks(notify.NewLinks([]byte("0123456789abcdef0123456789abcdef")))
	res := &results{}
	s.SetResults(res)
	return s, st, smtp, tg, res
}

func incident() alert.Alert {
	opened := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	return alert.Alert{ID: "INC-7", Title: "HTTP 5xx на app-01", CIName: "app-01.corp.local", Signal: "red.errors", Severity: "critical",
		OpenedAt: opened, PD: alert.PD{State: alert.PDFailed, Error: "breaker open"},
		Route: alert.Route{Services: []alert.Ref{{ID: "s1", Name: "Платежи"}}, Team: &alert.Ref{ID: "t1", Name: "Payments SRE"},
			People: []alert.Person{
				{UserID: "u1", Name: "Иванов", Email: "ivanov@example.com", Telegram: "4242", Role: "lead"},
				{UserID: "u2", Name: "Петров", Email: "", Telegram: "not a chat"},
			}}}
}

func TestBackupNotification(t *testing.T) {
	s, _, smtp, tg, res := setup(t)
	tg.Blocked["-100200"] = true
	s.Deliver(context.Background(), incident())

	mails := smtp.Mails()
	if len(mails) != 2 || mails[0].To != "ivanov@example.com" || mails[1].To != "duty@example.com" {
		t.Fatalf("mails: %+v", mails)
	}
	m := mails[0]
	if m.User != "relay" || m.Password != "secret" || m.From != "umbrella@example.com" {
		t.Fatalf("envelope: %+v", m)
	}
	if !strings.Contains(m.Subject, "INC-7 · HTTP 5xx на app-01") || !strings.Contains(m.Subject, "критично") {
		t.Fatalf("subject: %q", m.Subject)
	}
	for _, want := range []string{"Платежи", "Payments SRE", "05.10.2026 16:00 +07", "ошибка доставки: breaker open",
		"Открыть в Umbrella: https://umbrella.example.com/incidents?id=INC-7", "Подтвердить: https://umbrella.example.com/ack/"} {
		if !strings.Contains(m.Body, want) {
			t.Fatalf("body has no %q:\n%s", want, m.Body)
		}
	}
	// The link acknowledges in the name of the person it was sent to.
	link := m.Body[strings.Index(m.Body, "/ack/")+5:]
	link = link[:strings.IndexByte(link, '\n')]
	id, who, err := s.Links().Verify(link, time.Now())
	if err != nil || id != "INC-7" || who != notify.UserRecipient("u1") {
		t.Fatalf("link: %q %q %v", id, who, err)
	}
	if _, _, err := s.Links().Verify(link, time.Now().Add(notify.LinkTTL+time.Hour)); err == nil {
		t.Fatal("an expired link works")
	}
	if _, _, err := s.Links().Verify(link[:len(link)-2]+"xx", time.Now()); err == nil {
		t.Fatal("a forged link works")
	}

	msgs := tg.Messages()
	if len(msgs) != 1 || msgs[0].ChatID != "4242" || msgs[0].ParseMode != "HTML" || !strings.Contains(msgs[0].Text, "<b>INC-7 · HTTP 5xx на app-01</b>") {
		t.Fatalf("telegram: %+v", msgs)
	}

	codes := map[string][]map[string]string{}
	for _, n := range res.notes {
		codes[n.code] = append(codes[n.code], n.args)
	}
	if len(codes["notify_failed"]) != 1 || codes["notify_failed"][0]["to"] != "-100200" || !strings.Contains(codes["notify_failed"][0]["error"], "blocked") {
		t.Fatalf("failed: %+v", codes)
	}
	if len(codes["notify_sent"]) != 2 || codes["notify_sent"][0]["to"] != "ivanov@example.com, duty@example.com" || codes["notify_sent"][1]["to"] != "4242" {
		t.Fatalf("sent: %+v", codes)
	}
}

func TestBackupNotificationNobody(t *testing.T) {
	s, st, _, _, res := setup(t)
	st.Write(func(d *store.Data) {
		d.Settings.Alerting.Notify.Email.Enabled = false
		d.Settings.Alerting.Notify.ExtraTelegram = nil
	})
	a := incident()
	a.Route.People = []alert.Person{{UserID: "u1", Email: "ivanov@example.com"}}
	s.Deliver(context.Background(), a)
	if len(res.notes) != 1 || res.notes[0].code != "notify_none" {
		t.Fatalf("notes: %+v", res.notes)
	}
}

func TestChannelTests(t *testing.T) {
	s, st, smtp, tg, _ := setup(t)
	if err := s.TestEmail(context.Background(), "me@example.com"); err != nil {
		t.Fatal(err)
	}
	if m := smtp.Mails(); len(m) != 1 || m[0].To != "me@example.com" {
		t.Fatalf("mails: %+v", m)
	}
	bot, err := s.TestTelegram(context.Background(), "4242")
	if err != nil || bot != "umbrella_test_bot" || len(tg.Messages()) != 1 {
		t.Fatalf("telegram: %q %v", bot, err)
	}
	smtp.Reject["nobody@example.com"] = true
	if err := s.TestEmail(context.Background(), "nobody@example.com"); err == nil {
		t.Fatal("a refused recipient passed")
	}
	st.Write(func(d *store.Data) { d.Settings.Alerting.Notify.Telegram.TokenRef = "missing" })
	if _, err := s.TestTelegram(context.Background(), "4242"); !errors.Is(err, notify.ErrNoSecret) {
		t.Fatalf("missing token: %v", err)
	}
}
