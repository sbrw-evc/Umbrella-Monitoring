// Package demo seeds a sample CMDB map, connectors and rules, backfills a
// day of incidents and keeps generating source traffic, so the UI can be
// tried without real sources. Source names are generic on purpose.
package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/connector"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// Seed fills an empty store.
func Seed(st *store.Store) {
	now := time.Now()
	st.Write(func(d *store.Data) {
		d.Teams = []model.Team{
			{ID: "payments", Name: "Платёжные сервисы"},
			{ID: "web", Name: "Интернет-банк"},
			{ID: "infra", Name: "Инфраструктура"},
		}
		add := func(name, typ, team, desc string, ids ...model.Identity) string {
			id := d.NextID("CI")
			if ids == nil {
				ids = []model.Identity{}
			}
			d.CIs[id] = &model.CI{ID: id, Name: name, Type: typ, Team: team, Description: desc, Identities: ids, Origin: "discovery", CreatedAt: now.Add(-30 * 24 * time.Hour), Labels: map[string]string{}}
			return id
		}
		host := func(name, ip string) []model.Identity {
			return []model.Identity{{Kind: "hostname", Value: name + ".corp.local", Since: now.Add(-90 * 24 * time.Hour)}, {Kind: "ip", Value: ip, Since: now.Add(-90 * 24 * time.Hour)}}
		}
		rel := func(from, to, typ string) { d.Relations = append(d.Relations, model.Relation{From: from, To: to, Type: typ}) }

		bsPay := add("Онлайн-платежи", model.CIBusinessService, "payments", "Бизнес-услуга: приём платежей клиентов")
		bsBank := add("Интернет-банк", model.CIBusinessService, "web", "Бизнес-услуга: личный кабинет клиента")
		svcGw := add("Платёжный шлюз", model.CIITService, "payments", "ИТ-сервис обработки платежей")
		svcLk := add("Личный кабинет", model.CIITService, "web", "Веб-приложение клиента")
		svcAuth := add("Авторизация", model.CIITService, "web", "Вход и токены клиентов")
		h1 := add("pay-app-01", model.CIHost, "payments", "Сервер приложений", host("pay-app-01", "10.10.1.11")...)
		h2 := add("pay-app-02", model.CIHost, "payments", "Сервер приложений", host("pay-app-02", "10.10.1.12")...)
		db := add("pay-db-01", model.CIDatabase, "payments", "PostgreSQL платёжного шлюза", host("pay-db-01", "10.10.2.21")...)
		old := now.Add(-2 * 24 * time.Hour)
		asg := add("asg-pay-workers", model.CICloudGroup, "payments", "Autoscaling-группа обработчиков; ID инстансов меняются после перезапуска",
			model.Identity{Kind: "cloud_instance", Value: "i-0a7f3c19d2e4b5a61", Since: now.Add(-20 * 24 * time.Hour), Until: &old},
			model.Identity{Kind: "cloud_instance", Value: "i-0c2e9b7d41f8a3e02", Since: old},
			model.Identity{Kind: "cloud_tag", Value: "ci=asg-pay-workers", Since: now.Add(-20 * 24 * time.Hour)})
		d.CIs[asg].LogicalGroup = "eu-central-1 / asg-pay-workers"
		w1 := add("web-front-01", model.CIHost, "web", "Фронтенд", host("web-front-01", "10.20.1.11")...)
		w2 := add("web-front-02", model.CIHost, "web", "Фронтенд", host("web-front-02", "10.20.1.12")...)
		authDep := add("auth-api", model.CIDeployment, "web", "Deployment в кластере k8s-prod, namespace auth",
			model.Identity{Kind: "k8s", Value: "k8s-prod/auth/auth-api", Since: now.Add(-60 * 24 * time.Hour)})
		sw := add("core-sw-01", model.CINetwork, "infra", "Коммутатор ядра ЦОД", model.Identity{Kind: "ip", Value: "10.0.0.1", Since: now.Add(-365 * 24 * time.Hour)})

		rel(bsPay, svcGw, "depends_on")
		rel(bsPay, svcAuth, "depends_on")
		rel(bsBank, svcLk, "depends_on")
		rel(bsBank, svcAuth, "depends_on")
		rel(svcGw, h1, "runs_on")
		rel(svcGw, h2, "runs_on")
		rel(svcGw, db, "depends_on")
		rel(svcGw, asg, "depends_on")
		rel(svcLk, w1, "runs_on")
		rel(svcLk, w2, "runs_on")
		rel(svcLk, svcAuth, "depends_on")
		rel(svcAuth, authDep, "runs_on")
		rel(h1, sw, "depends_on")
		rel(h2, sw, "depends_on")
		rel(db, sw, "depends_on")

		for _, c := range connectors(now) {
			cc := c
			d.Connectors[cc.ID] = &cc
			d.UseID(cc.ID)
		}
		d.Rules = []model.Rule{
			{ID: "R-1", Method: model.MethodRED, Signal: "red.rate", Name: "Падение трафика", Condition: "rate < 0.5 × baseline(10m)", AppliesTo: "ИТ-сервисы", Severity: model.SevError, Enabled: true},
			{ID: "R-2", Method: model.MethodRED, Signal: "red.errors", Name: "Расход бюджета ошибок", Condition: "burn_rate(1h) > 14.4", AppliesTo: "ИТ-сервисы с SLO", Severity: model.SevCritical, Enabled: true},
			{ID: "R-3", Method: model.MethodRED, Signal: "red.duration", Name: "Задержка выше цели", Condition: "p99 > slo.latency for 5m", AppliesTo: "ИТ-сервисы с SLO", Severity: model.SevError, Enabled: true},
			{ID: "R-4", Method: model.MethodUSE, Signal: "use.cpu.utilization", Name: "Высокая загрузка", Condition: "utilization > 90% for 15m or disk_full_eta < 24h", AppliesTo: "хосты, БД, облачные группы", Severity: model.SevWarning, Enabled: true},
			{ID: "R-5", Method: model.MethodUSE, Signal: "use.saturation", Name: "Насыщение", Condition: "load > 2 × cores or queue grows 10m", AppliesTo: "хосты, БД", Severity: model.SevError, Enabled: true},
			{ID: "R-6", Method: model.MethodUSE, Signal: "use.errors", Name: "Ошибки ресурса", Condition: "increase(errors[5m]) > 0", AppliesTo: "все КЕ", Severity: model.SevWarning, Enabled: true},
		}
		mw := d.NextID("MW")
		d.Maintenance[mw] = &model.Maintenance{ID: mw, Title: "Обновление ядра на web-front-02", CIID: w2, CIName: "web-front-02",
			Start: now.Add(20 * time.Hour), End: now.Add(22 * time.Hour), Author: "Инженер мониторинга", CreatedAt: now}
	})
}

func node(id, kind string, x, y float64, cfg map[string]string) model.Node {
	return model.Node{ID: id, Kind: kind, X: x, Y: y, Config: cfg}
}

// chain links nodes one after another and lays them out in rows of four,
// so the builder canvas stays readable.
func chain(nodes ...model.Node) model.Graph {
	for i := range nodes {
		nodes[i].X = float64(40 + (i%4)*260)
		nodes[i].Y = float64(60 + (i/4)*180)
	}
	g := model.Graph{Nodes: nodes}
	for i := 0; i+1 < len(nodes); i++ {
		g.Edges = append(g.Edges, model.Edge{ID: fmt.Sprintf("e%d", i+1), Source: nodes[i].ID, Target: nodes[i+1].ID})
	}
	return g
}

func connectors(now time.Time) []model.Connector {
	metrics := chain(
		node("n1", "trigger.webhook", 40, 120, map[string]string{"auth": "token"}),
		node("n2", "parse.json", 280, 120, map[string]string{"items": "alerts"}),
		node("n3", "map.event", 520, 120, map[string]string{"title": "${annotations.summary}", "ci": "${labels.instance}", "signal": "${labels.signal|use.cpu.utilization}", "method": "use", "severity": "${labels.severity}", "status": "${status}", "external_id": "${fingerprint}", "value": "${annotations.value}"}),
		node("n4", "enrich.labels", 760, 120, map[string]string{"labels": "env=prod\nsource_kind=metrics"}),
		node("n5", "out.event", 1000, 120, map[string]string{}),
		node("n6", "ack.response", 1240, 120, map[string]string{"mode": "http_2xx"}),
	)
	syslog := chain(
		node("n1", "trigger.webhook", 40, 120, map[string]string{"auth": "token"}),
		node("n2", "parse.kv", 280, 120, map[string]string{"pair_sep": " ", "kv_sep": "="}),
		node("n3", "map.severity", 520, 120, map[string]string{"field": "sev", "mapping": "1=critical\n2=critical\n3=error\n4=warning\n*=info"}),
		node("n4", "map.event", 760, 120, map[string]string{"title": "${msg}", "ci": "${host}", "signal": "use.errors", "method": "use", "status": "${state|firing}", "external_id": "${id}"}),
		node("n5", "out.event", 1000, 120, map[string]string{}),
		node("n6", "ack.response", 1240, 120, map[string]string{"mode": "http_2xx"}),
	)
	cloud := chain(
		node("n1", "trigger.webhook", 40, 120, map[string]string{"auth": "token"}),
		node("n2", "parse.json", 280, 120, map[string]string{"items": "Records"}),
		node("n3", "filter", 520, 120, map[string]string{"field": "AlarmName", "op": "exists"}),
		node("n4", "map.severity", 760, 120, map[string]string{"field": "NewStateValue", "mapping": "ALARM=error\nOK=info\n*=warning"}),
		node("n5", "map.event", 1000, 120, map[string]string{"title": "${AlarmDescription|$AlarmName}", "ci": "${Dimensions.InstanceId}", "signal": "${Metric|use.cpu.utilization}", "method": "use", "status": "${NewStateValue}", "external_id": "${AlarmArn}"}),
		node("n6", "out.event", 1240, 120, map[string]string{}),
		node("n7", "ack.response", 1480, 120, map[string]string{"mode": "http_2xx"}),
	)
	apm := chain(
		node("n1", "trigger.webhook", 40, 120, map[string]string{"auth": "token"}),
		node("n2", "parse.json", 280, 120, map[string]string{}),
		node("n3", "map.event", 520, 120, map[string]string{"title": "${title}", "ci": "${service}", "signal": "${signal}", "method": "red", "severity": "${severity}", "status": "${state}", "external_id": "${id}", "value": "${value}"}),
		node("n4", "out.event", 760, 120, map[string]string{}),
		node("n5", "ack.response", 1000, 120, map[string]string{"mode": "http_2xx"}),
	)
	mk := func(id, name, team, desc string, g model.Graph, sample string) model.Connector {
		pub := g
		return model.Connector{ID: id, Name: name, Team: team, Description: desc, Status: model.ConnectorRunning, Version: 1,
			Draft: g, Published: &pub, SampleInput: sample, UpdatedAt: now.Add(-3 * time.Hour), UpdatedBy: "Инженер мониторинга"}
	}
	return []model.Connector{
		mk("CON-1", "Webhook: алерты метрик", "infra", "Пачка алертов системы сбора метрик, JSON", metrics,
			`{"alerts":[{"status":"firing","fingerprint":"a1b2","labels":{"instance":"pay-app-01","severity":"warning","signal":"use.cpu.utilization"},"annotations":{"summary":"CPU выше 90% 15 минут","value":"94%"}}]}`),
		mk("CON-2", "Syslog сетевого оборудования", "infra", "Строки key=value от syslog-шлюза", syslog,
			`host=core-sw-01 sev=3 id=sw-778 msg="Interface Te1/0/24 down"`),
		mk("CON-3", "Облако: алармы", "payments", "Алармы облачного провайдера; ресурс указан ID инстанса", cloud,
			`{"Records":[{"AlarmName":"cpu-high","AlarmArn":"arn:alarm:cpu-high:1","NewStateValue":"ALARM","Metric":"use.cpu.utilization","AlarmDescription":"CPU обработчиков выше 85%","Dimensions":{"InstanceId":"i-0c2e9b7d41f8a3e02"}}]}`),
		mk("CON-4", "APM: RED-сигналы сервисов", "payments", "Сигналы Rate, Errors, Duration по сервисам", apm,
			`{"id":"apm-55","service":"Платёжный шлюз","signal":"red.errors","severity":"critical","state":"firing","title":"Доля ошибок 7% (SLO 1%)","value":"7%"}`),
	}
}

// scenario is one problem a fake source reports.
type scenario struct {
	conn   string
	fire   func(id string) string
	clear  func(id, body string) string
	weight int
}

func js(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

var hosts = []string{"pay-app-01", "pay-app-02", "pay-db-01", "web-front-01", "web-front-02", "10.10.1.12", "web-front-01.corp.local"}

var scenarios = []scenario{
	{conn: "CON-1", weight: 5,
		fire: func(id string) string {
			h := hosts[rand.Intn(len(hosts))]
			sig := []string{"use.cpu.utilization", "use.mem.utilization", "use.disk.utilization", "use.saturation"}[rand.Intn(4)]
			sev := []string{"warning", "warning", "error", "critical"}[rand.Intn(4)]
			return js(map[string]any{"alerts": []any{map[string]any{"status": "firing", "fingerprint": id,
				"labels":      map[string]string{"instance": h, "severity": sev, "signal": sig},
				"annotations": map[string]string{"summary": titles[sig] + " на " + h, "value": fmt.Sprintf("%d%%", 85+rand.Intn(15))}}}})
		},
		clear: func(_, body string) string { return strings.Replace(body, `"status":"firing"`, `"status":"resolved"`, 1) }},
	{conn: "CON-2", weight: 2,
		fire: func(id string) string {
			return fmt.Sprintf(`host=core-sw-01 sev=%d id=%s msg="%s"`, 2+rand.Intn(3), id, []string{"Interface Te1/0/24 down", "BGP neighbor 10.0.0.9 down", "Fan tray 2 failure"}[rand.Intn(3)])
		},
		clear: func(id, _ string) string { return fmt.Sprintf(`host=core-sw-01 sev=5 id=%s state=ok msg="recovered"`, id) }},
	{conn: "CON-3", weight: 2,
		fire: func(id string) string {
			return js(map[string]any{"Records": []any{map[string]any{"AlarmName": "cpu-high", "AlarmArn": id, "NewStateValue": "ALARM", "Metric": "use.cpu.utilization",
				"AlarmDescription": "CPU обработчиков выше 85%", "Dimensions": map[string]string{"InstanceId": "i-0c2e9b7d41f8a3e02"}}}})
		},
		clear: func(id, _ string) string {
			return js(map[string]any{"Records": []any{map[string]any{"AlarmName": "cpu-high", "AlarmArn": id, "NewStateValue": "OK", "Metric": "use.cpu.utilization",
				"Dimensions": map[string]string{"InstanceId": "i-0c2e9b7d41f8a3e02"}}}})
		}},
	{conn: "CON-4", weight: 2,
		fire: func(id string) string {
			svc := []string{"Платёжный шлюз", "Личный кабинет", "Авторизация"}[rand.Intn(3)]
			sig := []string{"red.errors", "red.duration", "red.rate"}[rand.Intn(3)]
			return js(map[string]any{"id": id, "service": svc, "signal": sig, "severity": []string{"error", "critical"}[rand.Intn(2)], "state": "firing", "title": titles[sig] + ": " + svc})
		},
		clear: func(_, body string) string { return strings.Replace(body, `"state":"firing"`, `"state":"resolved"`, 1) }},
	// A malformed message keeps the parse error list realistic.
	{conn: "CON-1", weight: 1,
		fire:  func(id string) string { return `{"alerts":[{"status":"firing",` },
		clear: nil},
}

var titles = map[string]string{
	"use.cpu.utilization":  "Загрузка CPU выше 90%",
	"use.mem.utilization":  "Память занята на 95%",
	"use.disk.utilization": "Диск заполнится менее чем за 24 ч",
	"use.saturation":       "Очередь запросов растёт",
	"red.errors":           "Доля ошибок выше SLO",
	"red.duration":         "p99 задержки выше цели",
	"red.rate":             "Трафик упал на 60%",
}

type open struct {
	sc    scenario
	id    string
	until time.Time
	body  string
}

// Generator produces traffic through the real connector runtime.
type Generator struct {
	rt     *connector.Runtime
	eng    *alert.Engine
	active []open
	seq    int
}

// NewGenerator creates a generator.
func NewGenerator(rt *connector.Runtime, eng *alert.Engine) *Generator {
	return &Generator{rt: rt, eng: eng}
}

func pick() scenario {
	total := 0
	for _, s := range scenarios {
		total += s.weight
	}
	n := rand.Intn(total)
	for _, s := range scenarios {
		if n < s.weight {
			return s
		}
		n -= s.weight
	}
	return scenarios[0]
}

// Backfill replays a day of incidents with past timestamps. Call it before
// the background loops start.
func (g *Generator) Backfill(st *store.Store) {
	now := time.Now()
	for i := 0; i < 70; i++ {
		at := now.Add(-time.Duration(24*60-i*20) * time.Minute).Add(time.Duration(rand.Intn(600)) * time.Second)
		g.eng.SetClock(func() time.Time { return at })
		sc := pick()
		g.seq++
		id := fmt.Sprintf("bf-%d", g.seq)
		body := sc.fire(id)
		_, _ = g.rt.Webhook(sc.conn, body, "")
		// Most past problems were resolved; the last ones stay open.
		if sc.clear != nil && i < 62 {
			end := at.Add(time.Duration(5+rand.Intn(50)) * time.Minute)
			g.eng.SetClock(func() time.Time { return end })
			_, _ = g.rt.Webhook(sc.conn, sc.clear(id, body), "")
		}
	}
	g.eng.SetClock(time.Now)
	// Pretend PagerDuty accepted the history.
	st.Write(func(d *store.Data) {
		for _, a := range d.Alerts {
			if a.PDState == model.PDPending {
				a.PDState = model.PDAccepted
			}
		}
	})
}

// Run emits a new problem every few seconds and clears old ones.
func (g *Generator) Run(ctx context.Context, every time.Duration) {
	tk := time.NewTicker(every)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tk.C:
			keep := g.active[:0]
			for _, o := range g.active {
				if now.After(o.until) {
					_, _ = g.rt.Webhook(o.sc.conn, o.sc.clear(o.id, o.body), "")
				} else {
					keep = append(keep, o)
				}
			}
			g.active = keep
			if len(g.active) < 25 {
				sc := pick()
				g.seq++
				id := fmt.Sprintf("live-%d", g.seq)
				body := sc.fire(id)
				_, _ = g.rt.Webhook(sc.conn, body, "")
				if sc.clear != nil {
					g.active = append(g.active, open{sc: sc, id: id, body: body, until: now.Add(time.Duration(40+rand.Intn(240)) * time.Second)})
				}
			}
		}
	}
}
