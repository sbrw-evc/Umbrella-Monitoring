package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

func syncHosts(st *store.Store, it model.Integration, hosts []Host) {
	st.Write(func(d *store.Data) { (&Manager{}).upsertHosts(d, it, hosts, "mark", time.Now()) })
}

func ciByName(st *store.Store, name string) *model.CI {
	var out *model.CI
	st.Read(func(d *store.Data) {
		for _, ci := range d.CIs {
			if ci.Name == name {
				c := *ci
				out = &c
			}
		}
	})
	return out
}

func identityKinds(ci *model.CI) map[string]int {
	out := map[string]int{}
	for _, i := range ci.Identities {
		if i.Until == nil {
			out[i.Kind]++
		}
	}
	return out
}

func TestCollectorAddressesDoNotMergeServicesIntoHost(t *testing.T) {
	st := store.New()
	zbx := model.Integration{ID: "INT-1", Type: TypeZabbix}
	prom := model.Integration{ID: "INT-3", Type: TypePrometheus}
	nb := model.Integration{ID: "INT-4", Type: TypeNetBox}

	syncHosts(st, zbx, []Host{{Key: "10683", Name: "lab-host", Addresses: []string{"zabbix-agent"}}})
	syncHosts(st, prom, []Host{
		{Key: "lab-host", Name: "lab-host", Addresses: []string{"node-exporter", "cadvisor"}, Instances: []string{"node-exporter:9100", "cadvisor:8080"}},
		{Key: "prometheus", Name: "prometheus", Addresses: []string{"localhost"}, Instances: []string{"localhost:9090"}},
	})
	syncHosts(st, nb, []Host{
		{Key: "virtualization.virtualmachine:6", Name: "zabbix-agent"},
		{Key: "virtualization.virtualmachine:9", Name: "node-exporter"},
		{Key: "virtualization.virtualmachine:10", Name: "cadvisor"},
		{Key: "virtualization.virtualmachine:7", Name: "prometheus"},
		{Key: "dcim.device:1", Name: "lab-host", Team: "infra"},
	})

	host := ciByName(st, "lab-host")
	if host == nil {
		t.Fatal("host CI was renamed or lost")
	}
	k := identityKinds(host)
	if k[TypeZabbix] != 1 || k[TypePrometheus] != 1 || k[TypeNetBox] != 1 {
		t.Fatalf("host identities: %+v", host.Identities)
	}
	if host.Team != "infra" {
		t.Fatalf("host team %q", host.Team)
	}
	for _, i := range host.Identities {
		if i.Kind == "hostname" && !strings.EqualFold(i.Value, "lab-host") {
			t.Fatalf("collector address stored as hostname: %+v", i)
		}
	}
	for _, svc := range []string{"zabbix-agent", "node-exporter", "cadvisor"} {
		ci := ciByName(st, svc)
		if ci == nil || ci.ID == host.ID {
			t.Fatalf("service %s has no CI of its own", svc)
		}
		if identityKinds(ci)[TypeNetBox] != 1 {
			t.Fatalf("service %s identities: %+v", svc, ci.Identities)
		}
	}
	p := ciByName(st, "prometheus")
	if p == nil || identityKinds(p)[TypePrometheus] != 1 || identityKinds(p)[TypeNetBox] != 1 {
		t.Fatalf("prometheus CI: %+v", p)
	}
}

func TestOneSourceObjectPerCI(t *testing.T) {
	st := store.New()
	nb := model.Integration{ID: "INT-4", Type: TypeNetBox}
	syncHosts(st, nb, []Host{
		{Key: "dcim.device:1", Name: "web01", DNS: []string{"web.example.org"}},
		{Key: "dcim.device:2", Name: "web02", DNS: []string{"web.example.org"}},
	})
	a, b := ciByName(st, "web01"), ciByName(st, "web02")
	if a == nil || b == nil || a.ID == b.ID {
		t.Fatalf("two NetBox devices must stay two CIs: %+v %+v", a, b)
	}
}

func TestAgentAddressStillMergesPlainHost(t *testing.T) {
	st := store.New()
	syncHosts(st, model.Integration{ID: "INT-1", Type: TypeZabbix}, []Host{{Key: "1", Name: "db-primary", Addresses: []string{"pg01.corp.local"}}})
	syncHosts(st, model.Integration{ID: "INT-4", Type: TypeNetBox}, []Host{{Key: "dcim.device:5", Name: "pg01"}})
	ci := ciByName(st, "pg01")
	if ci == nil {
		t.Fatal("NetBox device pg01 was not merged into the Zabbix host reached at pg01.corp.local")
	}
	if k := identityKinds(ci); k[TypeZabbix] != 1 || k[TypeNetBox] != 1 {
		t.Fatalf("identities: %+v", ci.Identities)
	}
}
