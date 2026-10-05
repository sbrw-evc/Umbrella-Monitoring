package pagerduty_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pagerduty"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pagerduty/pdtest"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type secrets map[string]string

func (s secrets) Resolve(ref string) (string, error) {
	if v, ok := s[ref]; ok {
		return v, nil
	}
	return "", errors.New("no secret")
}

type result struct {
	action alert.Action
	route  string
	err    error
}

type results struct {
	mu  sync.Mutex
	got []result
}

func (r *results) PDResult(_ context.Context, _ string, action alert.Action, route string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, result{action, route, err})
}
func (r *results) PDInbound(context.Context, alert.PDUpdate) error { return nil }
func (r *results) PDKeys(context.Context, string, string) ([]string, error) {
	return nil, nil
}

func (r *results) wait(t *testing.T, n int) []result {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		r.mu.Lock()
		got := append([]result(nil), r.got...)
		r.mu.Unlock()
		if len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("results = %+v, want %d", got, n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDelivery(t *testing.T) {
	fake := pdtest.New(t)
	st := store.New()
	st.Write(func(d *store.Data) {
		d.Settings.Alerting.PagerDuty = model.PagerDuty{Enabled: true, EventsURL: fake.EventsURL(), RoutingKeyRef: "ref", MinSeverity: "error"}
	})
	g := pagerduty.New(st, secrets{"ref": "key-1"})
	g.Backoff = time.Millisecond
	res := &results{}
	g.SetResults(res)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx)

	a := alert.Alert{ID: "INC-1", PD: alert.PD{Key: "umb-INC-1"}, Title: "Down", Severity: "critical", Sources: map[string]*alert.Source{}}
	g.Send(alert.Command{Action: alert.PDTrigger, Alert: a})
	if got := res.wait(t, 1); got[0].err != nil || got[0].route != pagerduty.DefaultRoute || len(fake.Events()) != 1 || fake.Events()[0].RoutingKey != "key-1" {
		t.Fatalf("delivered: %+v %+v", got, fake.Events())
	}
	warn := a
	warn.Severity = "warning"
	g.Send(alert.Command{Action: alert.PDTrigger, Alert: warn})
	if got := res.wait(t, 2); !errors.Is(got[1].err, alert.ErrPDSkipped) {
		t.Errorf("below the threshold: %+v", got[1])
	}
	fake.SetEventsStatus(http.StatusServiceUnavailable)
	g.Send(alert.Command{Action: alert.PDResolve, Alert: a})
	if got := res.wait(t, 3); got[2].err == nil {
		t.Errorf("5xx is an error after retries: %+v", got[2])
	}
	if s := g.Status(); s.Sent != 1 || s.Failed != 1 || s.LastError == "" {
		t.Errorf("status = %+v", s)
	}
	// Two more failing deliveries of 3 attempts open the breaker.
	g.Send(alert.Command{Action: alert.PDResolve, Alert: a})
	g.Send(alert.Command{Action: alert.PDResolve, Alert: a})
	res.wait(t, 5)
	if !g.Status().BreakerOpen {
		t.Error("the breaker opens after 5 failures in a row")
	}
	st.Write(func(d *store.Data) { d.Settings.Alerting.PagerDuty.Enabled = false })
	g.Send(alert.Command{Action: alert.PDTrigger, Alert: a})
	if got := res.wait(t, 6); !errors.Is(got[5].err, pagerduty.ErrDisabled) {
		t.Errorf("disabled: %+v", got[5])
	}
}

func TestSignature(t *testing.T) {
	body := []byte(`{"event":{}}`)
	sig := pdtest.Sign("s", body)
	if !pagerduty.VerifySignature("s", body, "v1=00,"+sig) {
		t.Error("one of several signatures matches")
	}
	if pagerduty.VerifySignature("", body, sig) || pagerduty.VerifySignature("other", body, sig) {
		t.Error("no secret or a wrong one fails")
	}
}
