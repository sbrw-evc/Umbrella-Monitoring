package api

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	if s.cfg.MetricsToken != "" {
		got, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.MetricsToken)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			http.Error(w, "unauthorized", 401)
			return
		}
	}
	var b strings.Builder
	m := newMetricWriter(&b)
	m.gauge("umbrella_up", "Umbrella is running.", nil, 1)
	m.gauge("umbrella_build_info", "Build information.", map[string]string{"version": "0.2.0-mvp"}, 1)

	s.st.Read(func(d *store.Data) {
		type key struct{ sev, status, team, service string }
		active := map[key]int{}
		pd := map[string]int{}
		var total, resolved, suppressed, fallback, unbound int
		for _, a := range d.Alerts {
			total++
			if !a.Status.Active() {
				resolved++
				continue
			}
			if a.Suppressed {
				suppressed++
				continue
			}
			active[key{string(a.Severity), string(a.Status), a.Team, a.Service}]++
			pd[string(a.PDState)]++
			if a.Fallback {
				fallback++
			}
			if a.CIID == "" {
				unbound++
			}
		}
		sevSeen := map[string]bool{}
		for k := range active {
			sevSeen[k.sev] = true
		}
		m.help("umbrella_incidents_active", "Active incidents (open or acknowledged, not suppressed).", "gauge")
		for _, sev := range []string{"critical", "error", "warning", "info"} {

			if !sevSeen[sev] {
				m.sample("umbrella_incidents_active", map[string]string{"severity": sev, "status": "open", "team": "", "service": ""}, 0)
			}
		}
		keys := make([]key, 0, len(active))
		for k := range active {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j]) })
		for _, k := range keys {
			m.sample("umbrella_incidents_active", map[string]string{"severity": k.sev, "status": k.status, "team": k.team, "service": k.service}, float64(active[k]))
		}
		m.gauge("umbrella_incidents_suppressed", "Active incidents suppressed by maintenance.", nil, float64(suppressed))
		m.gauge("umbrella_incidents_fallback", "Active incidents PagerDuty did not take in time.", nil, float64(fallback))
		m.gauge("umbrella_incidents_unbound", "Active incidents without a CMDB match.", nil, float64(unbound))
		m.counter("umbrella_incidents_total", "Incidents opened since start.", nil, float64(total))
		m.counter("umbrella_incidents_resolved_total", "Incidents resolved since start.", nil, float64(resolved))
		m.help("umbrella_incidents_pagerduty", "Active incidents by PagerDuty delivery state.", "gauge")
		for _, st := range []string{"pending", "accepted", "acked", "failed", "skipped"} {
			m.sample("umbrella_incidents_pagerduty", map[string]string{"state": st}, float64(pd[st]))
		}

		m.help("umbrella_events_total", "Events received per connector.", "counter")
		m.help("umbrella_parse_errors_total", "Parse errors per connector.", "counter")
		m.help("umbrella_connector_running", "1 when the connector is running.", "gauge")
		var cons []*model.Connector
		for _, c := range d.Connectors {
			cons = append(cons, c)
		}
		sort.Slice(cons, func(i, j int) bool { return idNum(cons[i].ID) < idNum(cons[j].ID) })
		for _, c := range cons {
			l := map[string]string{"connector": c.ID, "name": c.Name}
			m.sample("umbrella_events_total", l, float64(c.EventsTotal))
			m.sample("umbrella_parse_errors_total", l, float64(c.ErrorsTotal))
			run := 0.0
			if c.Status == model.ConnectorRunning {
				run = 1
			}
			m.sample("umbrella_connector_running", l, run)
		}

		m.help("umbrella_notifications_total", "Notification deliveries per channel and result.", "counter")
		var chs []*model.Channel
		for _, c := range d.Channels {
			chs = append(chs, c)
		}
		sort.Slice(chs, func(i, j int) bool { return idNum(chs[i].ID) < idNum(chs[j].ID) })
		for _, c := range chs {
			m.sample("umbrella_notifications_total", map[string]string{"channel": c.ID, "name": c.Name, "type": c.Type, "result": "ok"}, float64(c.Sent))
			m.sample("umbrella_notifications_total", map[string]string{"channel": c.ID, "name": c.Name, "type": c.Type, "result": "failed"}, float64(c.Failed))
		}

		types := map[string]int{}
		for _, ci := range d.CIs {
			types[ci.Type]++
		}
		m.help("umbrella_cmdb_cis", "CMDB entries by type.", "gauge")
		for _, t := range sortedKeys(types) {
			m.sample("umbrella_cmdb_cis", map[string]string{"type": t}, float64(types[t]))
		}
		users := 0
		for _, u := range d.Users {
			if !u.Disabled {
				users++
			}
		}
		m.gauge("umbrella_users_enabled", "Enabled user accounts.", nil, float64(users))

		m.help("umbrella_rule_firing", "Series firing per RED/USE rule.", "gauge")
		for _, r := range d.Rules {
			m.sample("umbrella_rule_firing", map[string]string{"rule": r.ID, "name": r.Name, "method": string(r.Method)}, float64(r.Firing))
		}
		m.help("umbrella_rule_errors", "1 when the last evaluation of a rule failed.", "gauge")
		for _, r := range d.Rules {
			m.sample("umbrella_rule_errors", map[string]string{"rule": r.ID, "name": r.Name}, b2f(r.LastError != ""))
		}
		m.help("umbrella_integration_ok", "1 when the last check or inventory sync of an integration succeeded.", "gauge")
		for _, it := range d.Integrations {
			ok := it.LastCheckOK
			if it.SyncedAt != nil {
				ok = it.SyncOK
			}
			m.sample("umbrella_integration_ok", map[string]string{"integration": it.ID, "name": it.Name, "type": it.Type}, b2f(ok))
		}
	})
	st := s.pd.Status()
	m.gauge("umbrella_pagerduty_enabled", "1 when the PagerDuty integration is enabled.", nil, b2f(st.Enabled))
	m.gauge("umbrella_pagerduty_breaker_open", "1 while the PagerDuty circuit breaker is open.", nil, b2f(st.BreakerOpen))
	m.gauge("umbrella_pagerduty_queue", "Commands waiting for the PagerDuty Gateway.", nil, float64(st.QueueLen))
	m.help("umbrella_pagerduty_deliveries_total", "PagerDuty Events API deliveries since start.", "counter")
	m.sample("umbrella_pagerduty_deliveries_total", map[string]string{"result": "ok"}, float64(st.Sent))
	m.sample("umbrella_pagerduty_deliveries_total", map[string]string{"result": "failed"}, float64(st.Failed))
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Write([]byte(b.String()))
}

func b2f(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type metricWriter struct {
	b    *strings.Builder
	seen map[string]bool
}

func newMetricWriter(b *strings.Builder) *metricWriter {
	return &metricWriter{b: b, seen: map[string]bool{}}
}

func (m *metricWriter) help(name, help, typ string) {
	if m.seen[name] {
		return
	}
	m.seen[name] = true
	fmt.Fprintf(m.b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
}

func (m *metricWriter) gauge(name, help string, l map[string]string, v float64) {
	m.help(name, help, "gauge")
	m.sample(name, l, v)
}

func (m *metricWriter) counter(name, help string, l map[string]string, v float64) {
	m.help(name, help, "counter")
	m.sample(name, l, v)
}

var labelEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

func (m *metricWriter) sample(name string, l map[string]string, v float64) {
	m.b.WriteString(name)
	if len(l) > 0 {
		keys := make([]string, 0, len(l))
		for k := range l {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		m.b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				m.b.WriteByte(',')
			}
			fmt.Fprintf(m.b, `%s="%s"`, k, labelEscaper.Replace(l[k]))
		}
		m.b.WriteByte('}')
	}
	fmt.Fprintf(m.b, " %g\n", v)
}
