package api

import (
	"fmt"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

func seedFixture(st *store.Store) {
	now := time.Now()
	st.Write(func(d *store.Data) {
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
		rel := func(from, to, typ string) {
			d.Relations = append(d.Relations, model.Relation{From: from, To: to, Type: typ})
		}

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

		for _, c := range fixtureConnectors(now) {
			cc := c
			d.Connectors[cc.ID] = &cc
			d.UseID(cc.ID)
		}
		mw := d.NextID("MW")
		d.Maintenance[mw] = &model.Maintenance{ID: mw, Title: "Обновление ядра на web-front-02", CIID: w2, CIName: "web-front-02",
			Start: now.Add(20 * time.Hour), End: now.Add(22 * time.Hour), Author: "Инженер мониторинга", CreatedAt: now}
	})
}

func fixtureNode(id, kind string, x, y float64, cfg map[string]string) model.Node {
	return model.Node{ID: id, Kind: kind, X: x, Y: y, Config: cfg}
}

func fixtureChain(nodes ...model.Node) model.Graph {
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

func fixtureConnectors(now time.Time) []model.Connector {
	metrics := fixtureChain(
		fixtureNode("n1", "trigger.webhook", 40, 120, map[string]string{"auth": "token"}),
		fixtureNode("n2", "parse.json", 280, 120, map[string]string{"items": "alerts"}),
		fixtureNode("n3", "map.event", 520, 120, map[string]string{"title": "${annotations.summary}", "ci": "${labels.instance}", "signal": "${labels.signal|use.cpu.utilization}", "method": "use", "severity": "${labels.severity}", "status": "${status}", "external_id": "${fingerprint}", "value": "${annotations.value}"}),
		fixtureNode("n4", "enrich.labels", 760, 120, map[string]string{"labels": "env=prod\nsource_kind=metrics"}),
		fixtureNode("n5", "out.event", 1000, 120, map[string]string{}),
		fixtureNode("n6", "ack.response", 1240, 120, map[string]string{"mode": "http_2xx"}),
	)
	syslog := fixtureChain(
		fixtureNode("n1", "trigger.webhook", 40, 120, map[string]string{"auth": "token"}),
		fixtureNode("n2", "parse.kv", 280, 120, map[string]string{"pair_sep": " ", "kv_sep": "="}),
		fixtureNode("n3", "map.severity", 520, 120, map[string]string{"field": "sev", "mapping": "1=critical\n2=critical\n3=error\n4=warning\n*=info"}),
		fixtureNode("n4", "map.event", 760, 120, map[string]string{"title": "${msg}", "ci": "${host}", "signal": "use.errors", "method": "use", "status": "${state|firing}", "external_id": "${id}"}),
		fixtureNode("n5", "out.event", 1000, 120, map[string]string{}),
		fixtureNode("n6", "ack.response", 1240, 120, map[string]string{"mode": "http_2xx"}),
	)
	cloud := fixtureChain(
		fixtureNode("n1", "trigger.webhook", 40, 120, map[string]string{"auth": "token"}),
		fixtureNode("n2", "parse.json", 280, 120, map[string]string{"items": "Records"}),
		fixtureNode("n3", "filter", 520, 120, map[string]string{"field": "AlarmName", "op": "exists"}),
		fixtureNode("n4", "map.severity", 760, 120, map[string]string{"field": "NewStateValue", "mapping": "ALARM=error\nOK=info\n*=warning"}),
		fixtureNode("n5", "map.event", 1000, 120, map[string]string{"title": "${AlarmDescription|$AlarmName}", "ci": "${Dimensions.InstanceId}", "signal": "${Metric|use.cpu.utilization}", "method": "use", "status": "${NewStateValue}", "external_id": "${AlarmArn}"}),
		fixtureNode("n6", "out.event", 1240, 120, map[string]string{}),
		fixtureNode("n7", "ack.response", 1480, 120, map[string]string{"mode": "http_2xx"}),
	)
	apm := fixtureChain(
		fixtureNode("n1", "trigger.webhook", 40, 120, map[string]string{"auth": "token"}),
		fixtureNode("n2", "parse.json", 280, 120, map[string]string{}),
		fixtureNode("n3", "map.event", 520, 120, map[string]string{"title": "${title}", "ci": "${service}", "signal": "${signal}", "method": "red", "severity": "${severity}", "status": "${state}", "external_id": "${id}", "value": "${value}"}),
		fixtureNode("n4", "out.event", 760, 120, map[string]string{}),
		fixtureNode("n5", "ack.response", 1000, 120, map[string]string{"mode": "http_2xx"}),
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
