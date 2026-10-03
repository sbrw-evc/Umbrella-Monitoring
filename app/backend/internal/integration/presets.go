package integration

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

const zabbixSeverity = "Disaster=critical\nHigh=error\nAverage=warning\nWarning=warning\n*=info"
const alertSeverity = "critical=critical\nerror=error\nwarning=warning\ninfo=info\npage=critical\n*=warning"
const logSeverity = "panic=critical\nfatal=critical\ncritical=critical\nerror=error\nwarning=warning\nwarn=warning\n*=info"

func node(id, kind string, cfg map[string]string) model.Node {
	return model.Node{ID: id, Kind: kind, Config: cfg}
}

func chain(nodes ...model.Node) model.Graph {
	for i := range nodes {
		nodes[i].X = float64(40 + (i%4)*260)
		nodes[i].Y = float64(140 + (i/4)*180)
	}
	g := model.Graph{Nodes: nodes}
	for i := 0; i+1 < len(nodes); i++ {
		g.Edges = append(g.Edges, model.Edge{ID: fmt.Sprintf("e%d", i+1), Source: nodes[i].ID, Target: nodes[i+1].ID})
	}
	return g
}

func webhookTrigger(it model.Integration) model.Node {
	cfg := map[string]string{"auth": "token", "secret_ref": it.WebhookTokenRef}
	if it.WebhookTokenRef == "" {
		cfg["auth"] = "none"
	}
	return node("n1", "trigger.webhook", cfg)
}

func fetchConfig(it model.Integration, url, method, body string) map[string]string {
	cfg := map[string]string{"url": url, "method": method, "body": body, "auth": "none"}
	if it.SecretRef != "" {
		switch it.AuthType {
		case AuthBasic:
			cfg["auth"], cfg["username"] = "basic", it.Username
		case AuthAPIKey:
			cfg["auth"] = "apikey"
		case AuthToken:
			cfg["auth"] = "bearer"
		}
		if cfg["auth"] != "none" {
			cfg["secret_ref"] = it.SecretRef
		}
	}
	if it.TLSSkipVerify {
		cfg["tls_skip_verify"] = "true"
	}
	return cfg
}

func event(title, ci, signal, method, status, ext, value string) map[string]string {
	return map[string]string{"title": title, "ci": ci, "signal": signal, "method": method,
		"severity": "${_severity}", "status": status, "external_id": ext, "value": value}
}

func Graph(it model.Integration) (model.Graph, error) {
	spec, ok := Spec(it.Type)
	if !ok {
		return model.Graph{}, fmt.Errorf("неизвестный тип интеграции %q", it.Type)
	}
	p := func(k string) string { return spec.param(it.Params, k) }
	switch it.Type {
	case TypeZabbix:
		return chain(
			webhookTrigger(it),
			node("n2", "parse.json", map[string]string{}),
			node("n3", "map.severity", map[string]string{"field": "severity", "mapping": zabbixSeverity}),
			node("n4", "enrich.labels", map[string]string{"labels": "source=zabbix\nzabbix_trigger=${trigger_id}\nzabbix_url=${url}\nintegration=" + it.ID}),
			node("n5", "map.event", event("${trigger_name}", "${host}", "zabbix:${trigger_id}", "use", "${status}", "${event_id}", "${value}")),
			node("n6", "out.event", map[string]string{}),
			node("n7", "ack.response", map[string]string{"mode": "http_2xx"}),
		), nil
	case TypeAlertmanager, TypeGrafana:
		src := "prometheus"
		value := "${annotations.value}"
		if it.Type == TypeGrafana {
			src, value = "grafana", "${valueString}"
		}
		return chain(
			webhookTrigger(it),
			node("n2", "parse.json", map[string]string{"items": "alerts"}),
			node("n3", "map.severity", map[string]string{"field": "labels.severity", "mapping": alertSeverity}),
			node("n4", "enrich.labels", map[string]string{"labels": "source=" + src + "\nalertname=${labels.alertname}\njob=${labels.job}\nservice=${labels.service}\nintegration=" + it.ID}),
			node("n5", "map.event", event("${annotations.summary|$labels.alertname}", "${labels.ci|$labels.host|$labels.instance}",
				src+":${labels.alertname}", "${labels.method|other}", "${status}", "${fingerprint}", value)),
			node("n6", "out.event", map[string]string{}),
			node("n7", "ack.response", map[string]string{"mode": "http_2xx"}),
		), nil
	case TypeOpenSearch, TypeElasticsearch:
		query, err := logQuery(p("time_field"), p("level_field"), p("levels"), p("window"))
		if err != nil {
			return model.Graph{}, err
		}
		url := strings.TrimRight(it.URL, "/") + "/" + strings.Trim(p("index"), "/") + "/_search?ignore_unavailable=true"
		f := func(k string) string { return "_source." + p(k) }
		return chain(
			node("n1", "trigger.schedule", map[string]string{"interval": p("interval")}),
			node("n2", "fetch.http", fetchConfig(it, url, "POST", query)),
			node("n3", "parse.json", map[string]string{"items": "hits.hits"}),
			node("n4", "map.severity", map[string]string{"field": f("level_field"), "mapping": logSeverity}),
			node("n5", "enrich.labels", map[string]string{"labels": "source=" + it.Type + "\nservice=${" + f("service_field") + "}\nindex=${_index}\nintegration=" + it.ID}),
			node("n6", "map.event", event("${"+f("message_field")+"}", "${"+f("host_field")+"}",
				it.Type+":${"+f("service_field")+"|app}:${"+f("code_field")+"|"+"error}", "other", "firing", "${_id}", "${"+f("code_field")+"}")),
			node("n7", "out.event", map[string]string{}),
			node("n8", "ack.response", map[string]string{"mode": "cursor"}),
		), nil
	case TypeWebhook, TypeHTTP:
		var first []model.Node
		ack := "http_2xx"
		if it.Type == TypeWebhook {
			first = []model.Node{webhookTrigger(it)}
		} else {
			ack = "cursor"
			first = []model.Node{
				node("n1", "trigger.schedule", map[string]string{"interval": p("interval")}),
				node("n2", "fetch.http", fetchConfig(it, it.URL, p("method"), p("body"))),
			}
		}
		n := len(first)
		id := func(i int) string { return fmt.Sprintf("n%d", n+i) }
		return chain(append(first,
			node(id(1), "parse.json", map[string]string{"items": p("items")}),
			node(id(2), "map.severity", map[string]string{"field": p("severity_field"), "mapping": p("severity_map")}),
			node(id(3), "enrich.labels", map[string]string{"labels": "source=" + it.Type + "\nintegration=" + it.ID}),
			node(id(4), "map.event", event(p("title"), p("ci"), p("signal"), p("method"), p("status"), p("external_id"), "")),
			node(id(5), "out.event", map[string]string{}),
			node(id(6), "ack.response", map[string]string{"mode": ack}),
		)...), nil
	}
	return model.Graph{}, fmt.Errorf("тип %s не поддерживается", it.Type)
}

func logQuery(timeField, levelField, levels, window string) (string, error) {
	var terms []string
	for _, l := range strings.Split(levels, ",") {
		if l = strings.TrimSpace(l); l != "" {
			terms = append(terms, strings.ToLower(l), strings.ToUpper(l))
		}
	}
	if len(terms) == 0 {
		return "", fmt.Errorf("укажите хотя бы один уровень")
	}
	q := map[string]any{
		"size": 200,
		"sort": []any{map[string]any{timeField: "desc"}},
		"query": map[string]any{"bool": map[string]any{"filter": []any{
			map[string]any{"terms": map[string]any{levelField: terms}},
			map[string]any{"range": map[string]any{timeField: map[string]string{"gte": "now-" + window}}},
		}}},
	}
	b, err := json.Marshal(q)
	return string(b), err
}

func Sample(t string) string {
	switch t {
	case TypeZabbix:
		return `{"event_id":"1001","event_value":"1","status":"firing","host":"db-01","trigger_id":"20001","trigger_name":"High CPU utilization","severity":"High","value":"97 %","url":""}`
	case TypeAlertmanager:
		return `{"version":"4","status":"firing","receiver":"umbrella","alerts":[{"status":"firing","fingerprint":"5f2b4c1d9e0a7b36","labels":{"alertname":"HostHighLoad","host":"node-1","severity":"warning"},"annotations":{"summary":"Load is high on node-1","value":"3.1"}}]}`
	case TypeGrafana:
		return `{"receiver":"Umbrella","status":"firing","alerts":[{"status":"firing","fingerprint":"a1b2c3","labels":{"alertname":"HighLatency","instance":"api-1","severity":"error"},"annotations":{"summary":"p99 latency above 1s"},"valueString":"[ var='A' value=1.4 ]"}]}`
	case TypeOpenSearch, TypeElasticsearch:
		return `{"hits":{"hits":[{"_index":"logs-2026.10.02","_id":"Kq3x","_source":{"@timestamp":"2026-10-02T10:00:00Z","level":"error","host":"api-1","service":"shop-api","message":"Payment gateway timeout","error_code":"PAY-504"}}]}}`
	}
	return `{"id":"1","host":"srv-01","title":"Disk almost full","severity":"error","status":"firing","signal":"use.disk.utilization"}`
}
