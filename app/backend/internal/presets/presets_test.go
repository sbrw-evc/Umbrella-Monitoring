package presets

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/flow"
)

// Every preset compiles once a credential is chosen, and its samples produce events without failures.
func TestPresets(t *testing.T) {
	if len(All()) != 4 {
		t.Fatalf("presets = %d", len(All()))
	}
	for _, p := range All() {
		if p.Title.RU == "" || p.Description.RU == "" || len(p.Document.Samples) == 0 {
			t.Errorf("%s: titles and samples are required", p.ID)
		}
		mapping := map[string]string{}
		for _, s := range p.Document.Credentials {
			mapping[s.Slot] = "CRD-1"
		}
		g, err := p.Document.Apply(mapping, func(string) (string, bool) { return flow.CredBearer, true })
		if err != nil {
			t.Fatalf("%s: %v", p.ID, err)
		}
		pl, issues := flow.Compile(g, flow.CompileOptions{Credential: func(string) (string, bool) { return flow.CredBearer, true }})
		if pl == nil {
			t.Fatalf("%s: %v", p.ID, issues)
		}
		for _, s := range p.Document.Samples {
			res, err := pl.Run(context.Background(), flow.Input{Body: []byte(s.Body)}, flow.RunOptions{})
			if err != nil || len(res.Events) == 0 || len(res.Failures) > 0 {
				t.Errorf("%s / %s: events=%+v failures=%+v err=%v", p.ID, s.Name, res.Events, res.Failures, err)
			}
		}
	}
}

func TestZabbixMediaTypeMatchesDeploy(t *testing.T) {
	b, err := os.ReadFile("../../../../deploy/zabbix/umbrella-mediatype.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != zabbixMediaType {
		t.Error("presets/zabbix-mediatype.yaml differs from deploy/zabbix/umbrella-mediatype.yaml: copy it again")
	}
	got := ZabbixMediaType("https://umb.example.com/api/ingest/zabbix-2", "tok'en")
	if !strings.Contains(got, "value: 'https://umb.example.com/api/ingest/zabbix-2'") || !strings.Contains(got, "value: 'tok''en'") {
		t.Errorf("filled media type:\n%s", got)
	}
}
