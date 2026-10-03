package integration

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type Host struct {
	Key          string
	Name         string
	IPs          []string
	DNS          []string
	Instances    []string
	Type         string
	Team         string
	Description  string
	LogicalGroup string
	Labels       map[string]string
	Owners       []model.Owner
	OwnersSet    bool
	ExternalURL  string
}

type discoveryResult struct {
	Found, Added, Merged, Missing, Deleted, WithOwners int
}

func discovers(t string) bool { return t == TypeZabbix || t == TypePrometheus || t == TypeNetBox }

func syncEnabled(it model.Integration) bool {
	if it.Type == TypeNetBox {
		return true
	}
	return discovers(it.Type) && it.Params["discovery"] != "false"
}

func syncInterval(it model.Integration) time.Duration {
	v := it.Params["discovery_interval"]
	if it.Type == TypeNetBox {
		spec, _ := Spec(TypeNetBox)
		v = spec.param(it.Params, "interval")
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < time.Minute {
		d = 10 * time.Minute
	}
	return d
}

func identityValue(itID, key string) string { return itID + ":" + key }

func activeIdentity(ci *model.CI, kind, value string) bool {
	for _, i := range ci.Identities {
		if i.Kind == kind && i.Until == nil && strings.EqualFold(i.Value, value) {
			return true
		}
	}
	return false
}

func matchHost(d *store.Data, kind, idv string, h Host) *model.CI {
	for _, ci := range d.CIs {
		if activeIdentity(ci, kind, idv) {
			return ci
		}
	}
	names := append([]string{h.Name}, h.DNS...)
	for _, n := range names {
		if n == "" {
			continue
		}
		short := strings.SplitN(n, ".", 2)[0]
		for _, ci := range d.CIs {
			if strings.EqualFold(ci.Name, n) || strings.EqualFold(ci.Name, short) || activeIdentity(ci, "hostname", n) {
				return ci
			}
		}
	}
	for _, ip := range h.IPs {
		for _, ci := range d.CIs {
			if activeIdentity(ci, "ip", ip) {
				return ci
			}
		}
	}
	return nil
}

func (m *Manager) upsertHosts(d *store.Data, it model.Integration, hosts []Host, missing string, now time.Time) discoveryResult {
	kind := it.Type
	authoritative := it.Type == TypeNetBox
	res := discoveryResult{Found: len(hosts)}
	seen := map[string]bool{}
	for _, h := range hosts {
		h.Name = strings.TrimSpace(h.Name)
		if h.Name == "" {
			continue
		}
		idv := identityValue(it.ID, h.Key)
		seen[idv] = true
		ci := matchHost(d, kind, idv, h)
		if ci == nil {
			typ := h.Type
			if typ == "" {
				typ = model.CIHost
			}
			ci = &model.CI{ID: d.NextID("CI"), Name: h.Name, Type: typ, Origin: kind, Source: it.ID, CreatedAt: now,
				Identities: []model.Identity{}, Labels: map[string]string{}}
			d.CIs[ci.ID] = ci
			res.Added++
		} else {
			res.Merged++
		}
		if ci.Labels == nil {
			ci.Labels = map[string]string{}
		}
		if ci.Origin == "auto" {
			ci.Origin, ci.Source = kind, it.ID
			if h.Type != "" {
				ci.Type = h.Type
			}
		}
		if authoritative {
			if ci.Origin != "manual" {
				if ci.Name != h.Name && !nameTaken(d, ci.ID, h.Name) {
					ci.Name = h.Name
				}
				ci.Origin = kind
			}
			if ci.Origin != "manual" && h.Type != "" {
				ci.Type = h.Type
			}
			ci.Source, ci.ExternalURL = it.ID, h.ExternalURL
			if h.Team != "" {
				ci.Team = h.Team
			}
			if h.Description != "" {
				ci.Description = h.Description
			}
			if h.LogicalGroup != "" {
				ci.LogicalGroup = h.LogicalGroup
			}
		} else {
			if ci.Team == "" {
				ci.Team = firstSet(h.Team, it.Team)
			}
			if ci.Description == "" {
				ci.Description = h.Description
			}
			if ci.LogicalGroup == "" {
				ci.LogicalGroup = h.LogicalGroup
			}
			if ci.ExternalURL == "" {
				ci.ExternalURL = h.ExternalURL
			}
		}
		delete(ci.Labels, kind+"_missing")
		for k, v := range h.Labels {
			if v == "" {
				delete(ci.Labels, k)
			} else {
				ci.Labels[k] = v
			}
		}
		setIdentity(ci, kind, idv, now)
		addIdentity(ci, "hostname", h.Name, now)
		for _, n := range h.DNS {
			addIdentity(ci, "hostname", n, now)
		}
		for _, ip := range h.IPs {
			addIdentity(ci, "ip", ip, now)
		}
		for _, in := range h.Instances {
			addIdentity(ci, "instance", in, now)
		}
		if h.OwnersSet {
			kept := []model.Owner{}
			for _, x := range ci.Owners {
				if !strings.HasPrefix(x.From, kind) {
					kept = append(kept, x)
				}
			}
			ci.Owners = append(kept, h.Owners...)
		}
		if len(ci.Owners) > 0 {
			res.WithOwners++
		}
		t := now
		ci.UpdatedAt = &t
	}
	prefix := it.ID + ":"
	for id, ci := range d.CIs {
		for i := range ci.Identities {
			idn := &ci.Identities[i]
			if idn.Kind != kind || idn.Until != nil || !strings.HasPrefix(idn.Value, prefix) || seen[idn.Value] {
				continue
			}
			res.Missing++
			if missing == "delete" && ci.Origin == kind && !otherSources(ci, kind, prefix) {
				d.DeleteCI(id)
				res.Deleted++
				break
			}
			t := now
			idn.Until = &t
			ci.Labels[kind+"_missing"] = "true"
		}
	}
	return res
}

func otherSources(ci *model.CI, kind, prefix string) bool {
	for _, i := range ci.Identities {
		if i.Until != nil {
			continue
		}
		if (i.Kind == TypeZabbix || i.Kind == TypePrometheus || i.Kind == TypeNetBox) && !(i.Kind == kind && strings.HasPrefix(i.Value, prefix)) {
			return true
		}
	}
	return false
}

func nameTaken(d *store.Data, self, name string) bool {
	for _, x := range d.CIs {
		if x.ID != self && strings.EqualFold(x.Name, name) {
			return true
		}
	}
	return false
}

func addIdentity(ci *model.CI, kind, value string, now time.Time) {
	if value == "" || activeIdentity(ci, kind, value) {
		return
	}
	ci.Identities = append(ci.Identities, model.Identity{Kind: kind, Value: value, Since: now})
}

func (m *Manager) Sync(ctx context.Context, id, actor string) (Result, error) {
	v, ok := m.Get(id)
	if !ok {
		return Result{}, ErrNotFound
	}
	it := v.Integration
	if !discovers(it.Type) {
		return Result{}, errors.New("эта интеграция не загружает КЕ")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	res := Result{At: time.Now()}
	msg, err := m.sync(ctx, it)
	res.OK, res.Message = err == nil, msg
	if err != nil {
		res.Message = err.Error()
	}
	m.st.Write(func(d *store.Data) {
		if p := d.Integrations[id]; p != nil {
			t := res.At
			p.SyncedAt, p.SyncOK, p.SyncInfo = &t, res.OK, res.Message
		}
		if actor != "system" {
			d.AddAudit(store.AuditEntry{At: res.At.Format(time.RFC3339), Actor: actor, Action: fmt.Sprintf("integration.sync ok=%v", res.OK), Object: id})
		}
	})
	return res, nil
}

func (m *Manager) sync(ctx context.Context, it model.Integration) (string, error) {
	secret, err := m.secret(it)
	if err != nil {
		return "", err
	}
	var hosts []Host
	var head string
	switch it.Type {
	case TypeNetBox:
		hosts, head, err = netboxHosts(ctx, it, secret)
	case TypeZabbix:
		hosts, head, err = zabbixHosts(ctx, it, secret)
	case TypePrometheus:
		hosts, head, err = prometheusHosts(ctx, it, secret)
	}
	if err != nil {
		return "", err
	}
	missing := "mark"
	if it.Type == TypeNetBox {
		spec, _ := Spec(TypeNetBox)
		missing = spec.param(it.Params, "missing")
	}
	var r discoveryResult
	now := time.Now()
	m.st.Write(func(d *store.Data) { r = m.upsertHosts(d, it, hosts, missing, now) })
	msg := fmt.Sprintf("%s; КЕ новых %d, объединено с существующими %d", head, r.Added, r.Merged)
	if it.Type == TypeNetBox {
		msg += fmt.Sprintf(", с ответственными %d", r.WithOwners)
	}
	if r.Missing > 0 {
		if r.Deleted > 0 {
			msg += fmt.Sprintf("; удалено пропавших %d", r.Deleted)
		} else {
			msg += fmt.Sprintf("; пропало из источника %d (метка %s_missing)", r.Missing, it.Type)
		}
	}
	return msg, nil
}

func zabbixHosts(ctx context.Context, it model.Integration, secret string) ([]Host, string, error) {
	z := newZabbix(it, secret)
	ver, err := z.version(ctx)
	if err != nil {
		return nil, "", err
	}
	if err := z.open(ctx, it.Username, secret); err != nil {
		return nil, "", err
	}
	defer z.close(ctx)
	var list []struct {
		ID          string `json:"hostid"`
		Host        string `json:"host"`
		Name        string `json:"name"`
		Status      string `json:"status"`
		Description string `json:"description"`
		Interfaces  []struct {
			IP  string `json:"ip"`
			DNS string `json:"dns"`
		} `json:"interfaces"`
		Groups []struct {
			Name string `json:"name"`
		} `json:"hostgroups"`
	}
	params := map[string]any{"output": []string{"hostid", "host", "name", "status", "description"},
		"selectInterfaces": []string{"ip", "dns"}, "selectHostGroups": []string{"name"}, "filter": map[string]any{"flags": 0}}
	if err := z.call(ctx, "host.get", params, true, &list); err != nil {
		return nil, "", err
	}
	base := strings.TrimRight(it.URL, "/")
	hosts := make([]Host, 0, len(list))
	for _, h := range list {
		x := Host{Key: h.ID, Name: h.Host, Description: h.Description, Labels: map[string]string{},
			ExternalURL: base + "/zabbix.php?action=host.view&filter_host=" + urlQuery(h.Host)}
		if h.Name != "" && h.Name != h.Host {
			x.DNS = append(x.DNS, h.Name)
		}
		for _, in := range h.Interfaces {
			if in.IP != "" && in.IP != "127.0.0.1" && in.IP != "0.0.0.0" {
				x.IPs = append(x.IPs, in.IP)
			}
			if in.DNS != "" {
				x.DNS = append(x.DNS, in.DNS)
			}
		}
		var groups []string
		for _, g := range h.Groups {
			groups = append(groups, g.Name)
		}
		x.Labels["zabbix_groups"] = strings.Join(groups, ", ")
		if h.Status == "1" {
			x.Labels["zabbix_status"] = "disabled"
		} else {
			x.Labels["zabbix_status"] = "monitored"
		}
		hosts = append(hosts, x)
	}
	return hosts, fmt.Sprintf("Zabbix %s: узлов %d", ver, len(hosts)), nil
}

func prometheusHosts(ctx context.Context, it model.Integration, secret string) ([]Host, string, error) {
	var resp struct {
		Data struct {
			ActiveTargets []struct {
				Labels    map[string]string `json:"labels"`
				ScrapeURL string            `json:"scrapeUrl"`
				Health    string            `json:"health"`
			} `json:"activeTargets"`
		} `json:"data"`
	}
	if err := newRemote(it, secret).do(ctx, http.MethodGet, "/api/v1/targets?state=active", nil, &resp); err != nil {
		return nil, "", err
	}
	label := it.Params["host_label"]
	if label == "" {
		label = "host"
	}
	jobs := map[string]bool{}
	for _, j := range strings.Split(it.Params["jobs"], ",") {
		if j = strings.TrimSpace(j); j != "" {
			jobs[j] = true
		}
	}
	byHost := map[string]*Host{}
	var order []string
	for _, t := range resp.Data.ActiveTargets {
		if len(jobs) > 0 && !jobs[t.Labels["job"]] {
			continue
		}
		inst := t.Labels["instance"]
		hostPart := inst
		if h, _, err := net.SplitHostPort(inst); err == nil {
			hostPart = h
		}
		name := t.Labels[label]
		if name == "" {
			name = hostPart
		}
		if name == "" {
			continue
		}
		h := byHost[strings.ToLower(name)]
		if h == nil {
			h = &Host{Key: strings.ToLower(name), Name: name, Labels: map[string]string{}}
			byHost[strings.ToLower(name)] = h
			order = append(order, strings.ToLower(name))
		}
		h.Instances = append(h.Instances, inst)
		if ip := net.ParseIP(hostPart); ip != nil {
			h.IPs = append(h.IPs, hostPart)
		} else if hostPart != name {
			h.DNS = append(h.DNS, hostPart)
		}
		jobsLabel := h.Labels["prometheus_jobs"]
		if !strings.Contains(","+jobsLabel+",", ","+t.Labels["job"]+",") {
			h.Labels["prometheus_jobs"] = strings.Trim(jobsLabel+","+t.Labels["job"], ",")
		}
		if t.Health != "up" || h.Labels["prometheus_health"] == "" {
			h.Labels["prometheus_health"] = t.Health
		}
	}
	hosts := make([]Host, 0, len(order))
	for _, k := range order {
		hosts = append(hosts, *byHost[k])
	}
	return hosts, fmt.Sprintf("Prometheus: целей %d, хостов %d", len(resp.Data.ActiveTargets), len(hosts)), nil
}

func (m *Manager) runSync(ctx context.Context) {
	var due []string
	m.st.Read(func(d *store.Data) {
		for _, it := range d.Integrations {
			if !syncEnabled(*it) {
				continue
			}
			if it.SyncedAt == nil || time.Since(*it.SyncedAt) >= syncInterval(*it) {
				due = append(due, it.ID)
			}
		}
	})
	for _, id := range due {
		if res, err := m.Sync(ctx, id, "system"); err == nil && !res.OK {
			slog.Warn("inventory sync failed", "integration", id, "err", res.Message)
		}
	}
}

func (m *Manager) Run(ctx context.Context) {
	tk := time.NewTicker(30 * time.Second)
	defer tk.Stop()
	m.runSync(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			m.runSync(ctx)
		}
	}
}
