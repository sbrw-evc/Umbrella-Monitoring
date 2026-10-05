package pipeline

import (
	"testing"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

func graph(nodes ...model.Node) model.Graph {
	g := model.Graph{Nodes: nodes}
	for i := 0; i+1 < len(nodes); i++ {
		g.Edges = append(g.Edges, model.Edge{ID: "e" + nodes[i].ID, Source: nodes[i].ID, Target: nodes[i+1].ID})
	}
	return g
}

func n(id, kind string, cfg map[string]string) model.Node {
	return model.Node{ID: id, Kind: kind, Config: cfg}
}

func TestJSONBatch(t *testing.T) {
	g := graph(
		n("t", "trigger.webhook", nil),
		n("p", "parse.json", map[string]string{"items": "data.alerts"}),
		n("m", "map.event", map[string]string{"title": "${msg}", "ci": "${host}", "signal": "use.cpu", "severity": "${sev}", "status": "${state}", "external_id": "${id}", "method": "use"}),
		n("o", "out.event", nil),
	)
	res, err := Run(g, `{"data":{"alerts":[{"id":1,"host":"a","sev":"critical","msg":"x","state":"firing"},{"id":2,"host":"b","sev":"warn","msg":"y","state":"OK"}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 2 {
		t.Fatalf("events = %d", len(res.Events))
	}
	e0, e1 := res.Events[0], res.Events[1]
	if e0.CI != "a" || e0.Severity != model.SevCritical || e0.Status != model.EventFiring || e0.ExternalID != "1" || e0.Method != model.MethodUSE {
		t.Errorf("event 0 = %+v", e0)
	}
	if e1.Severity != model.SevWarning || e1.Status != model.EventResolved {
		t.Errorf("event 1 = %+v", e1)
	}
}

func TestKVWithSeverityMapAndFilter(t *testing.T) {
	g := graph(
		n("t", "trigger.webhook", nil),
		n("p", "parse.kv", nil),
		n("f", "filter", map[string]string{"field": "host", "op": "ne", "value": "noise"}),
		n("s", "map.severity", map[string]string{"field": "sev", "mapping": "1=critical\n3=error\n*=info"}),
		n("m", "map.event", map[string]string{"ci": "${host}", "title": "${msg}"}),
		n("o", "out.event", nil),
	)
	res, err := Run(g, "host=sw1 sev=3 msg=\"link down on Te1\"\nhost=noise sev=1 msg=x\nhost=sw2 sev=9 msg=y")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 2 {
		t.Fatalf("events = %d", len(res.Events))
	}
	if res.Events[0].Severity != model.SevError || res.Events[0].Title != "link down on Te1" {
		t.Errorf("event 0 = %+v", res.Events[0])
	}
	if res.Events[1].Severity != model.SevInfo {
		t.Errorf("event 1 severity = %s", res.Events[1].Severity)
	}
}

func TestRegexAndBadJSON(t *testing.T) {
	g := graph(
		n("t", "trigger.webhook", nil),
		n("p", "parse.regex", map[string]string{"pattern": `(?P<host>\S+) (?P<level>\w+): (?P<msg>.*)`}),
		n("m", "map.event", map[string]string{"ci": "${host}", "severity": "${level}", "title": "${msg}"}),
		n("o", "out.event", nil),
	)
	res, err := Run(g, "db1 critical: disk full\nnot matching")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 1 || len(res.Errors) != 1 {
		t.Fatalf("events=%d errors=%d", len(res.Events), len(res.Errors))
	}

	bad := graph(n("t", "trigger.webhook", nil), n("p", "parse.json", nil), n("o", "out.event", nil))
	res, err = Run(bad, `{"a":`)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 1 || len(res.Events) != 0 {
		t.Fatalf("want one parse error, got %+v", res)
	}
}

func TestValidate(t *testing.T) {
	if _, err := Validate(graph(n("p", "parse.json", nil), n("o", "out.event", nil))); err == nil {
		t.Error("graph without trigger must fail")
	}
	cyc := graph(n("t", "trigger.webhook", nil), n("a", "parse.json", nil), n("o", "out.event", nil))
	cyc.Edges = append(cyc.Edges, model.Edge{ID: "x", Source: "o", Target: "a"})
	if _, err := Validate(cyc); err == nil {
		t.Error("cycle must fail")
	}
}

func TestRenderDefault(t *testing.T) {
	r := Record{"a": map[string]any{"b": "v"}}
	if got := Render("${a.b}-${c|dflt}-${c|$a.b}", r); got != "v-dflt-v" {
		t.Errorf("got %q", got)
	}
}

func TestRenderFallbackChain(t *testing.T) {
	r := Record{"labels": map[string]any{"instance": "node:9100", "job": "node"}}
	cases := map[string]string{
		"${labels.ci|$labels.host|$labels.instance}": "node:9100",
		"${labels.job|$labels.instance}":             "node",
		"${labels.ci|$labels.host|none}":             "none",
		"${labels.ci|$labels.host}":                  "",
	}
	for tpl, want := range cases {
		if got := Render(tpl, r); got != want {
			t.Errorf("%s = %q, want %q", tpl, got, want)
		}
	}
}
