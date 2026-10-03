package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

const (
	netboxPage       = 500
	netboxMaxObjects = 50000
	objDevice        = "dcim.device"
	objVM            = "virtualization.virtualmachine"
	objSite          = "dcim.site"
	objTenant        = "tenancy.tenant"
)

type nbRef struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Model string `json:"model"`
}

type nbChoice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type nbIP struct {
	Address string `json:"address"`
}

type nbObject struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Display     string    `json:"display"`
	Description string    `json:"description"`
	Status      *nbChoice `json:"status"`
	Role        *nbRef    `json:"role"`
	DeviceRole  *nbRef    `json:"device_role"`
	DeviceType  *nbRef    `json:"device_type"`
	Platform    *nbRef    `json:"platform"`
	Site        *nbRef    `json:"site"`
	Cluster     *nbRef    `json:"cluster"`
	Tenant      *nbRef    `json:"tenant"`
	PrimaryIP   *nbIP     `json:"primary_ip"`
	kind        string
}

type nbContact struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

type nbAssignment struct {
	ObjectType  string `json:"object_type"`
	ContentType string `json:"content_type"`
	ObjectID    int    `json:"object_id"`
	Contact     nbRef  `json:"contact"`
	Role        *nbRef `json:"role"`
}

func nbList[T any](ctx context.Context, r *remote, path, filter string) ([]T, error) {
	var out []T
	for offset := 0; offset < netboxMaxObjects; offset += netboxPage {
		q := "limit=" + strconv.Itoa(netboxPage) + "&offset=" + strconv.Itoa(offset)
		if filter != "" {
			q += "&" + filter
		}
		var resp struct {
			Count   int    `json:"count"`
			Next    string `json:"next"`
			Results []T    `json:"results"`
		}
		if err := r.do(ctx, http.MethodGet, path+"?"+q, nil, &resp); err != nil {
			return nil, fmt.Errorf("NetBox %s: %w", path, err)
		}
		out = append(out, resp.Results...)
		if resp.Next == "" || len(resp.Results) < netboxPage {
			break
		}
	}
	return out, nil
}

func netboxVersion(ctx context.Context, r *remote) (string, error) {
	var st map[string]any
	if err := r.do(ctx, http.MethodGet, "/api/status/", nil, &st); err != nil {
		return "", err
	}
	return fmt.Sprint(st["netbox-version"]), nil
}

func (o nbObject) role() string {
	if o.Role != nil {
		return o.Role.Name
	}
	if o.DeviceRole != nil {
		return o.DeviceRole.Name
	}
	return ""
}

func (o nbObject) ciType() string {
	r := strings.ToLower(o.role())
	for _, k := range []string{"switch", "router", "firewall", "network", "balancer", "gateway"} {
		if strings.Contains(r, k) {
			return model.CINetwork
		}
	}
	for _, k := range []string{"database", "db", "postgres", "sql", "oracle", "mongo"} {
		if strings.Contains(r, k) {
			return model.CIDatabase
		}
	}
	return model.CIHost
}

func (o nbObject) key() string { return o.kind + ":" + strconv.Itoa(o.ID) }

func (o nbObject) webURL(base string) string {
	if o.kind == objDevice {
		return base + "/dcim/devices/" + strconv.Itoa(o.ID) + "/"
	}
	return base + "/virtualization/virtual-machines/" + strconv.Itoa(o.ID) + "/"
}

func netboxHosts(ctx context.Context, it model.Integration, secret string) ([]Host, string, error) {
	spec, _ := Spec(TypeNetBox)
	p := func(k string) string { return spec.param(it.Params, k) }
	r := newRemote(it, secret)
	ver, err := netboxVersion(ctx, r)
	if err != nil {
		return nil, "", err
	}
	filter := strings.TrimPrefix(strings.TrimSpace(p("filter")), "?")
	if _, err := url.ParseQuery(filter); err != nil {
		return nil, "", fmt.Errorf("фильтр NetBox: %w", err)
	}
	var objects []nbObject
	devices, vms := 0, 0
	if strings.Contains(p("objects"), "devices") {
		list, err := nbList[nbObject](ctx, r, "/api/dcim/devices/", filter)
		if err != nil {
			return nil, "", err
		}
		for i := range list {
			list[i].kind = objDevice
		}
		devices = len(list)
		objects = append(objects, list...)
	}
	if strings.Contains(p("objects"), "vms") {
		list, err := nbList[nbObject](ctx, r, "/api/virtualization/virtual-machines/", filter)
		if err != nil {
			return nil, "", err
		}
		for i := range list {
			list[i].kind = objVM
		}
		vms = len(list)
		objects = append(objects, list...)
	}
	contacts, err := nbList[nbContact](ctx, r, "/api/tenancy/contacts/", "")
	if err != nil {
		return nil, "", err
	}
	assigns, err := nbList[nbAssignment](ctx, r, "/api/tenancy/contact-assignments/", "")
	if err != nil {
		return nil, "", err
	}
	byContact := map[int]nbContact{}
	for _, c := range contacts {
		byContact[c.ID] = c
	}
	roles := map[string]bool{}
	for _, x := range strings.Split(p("contact_roles"), ",") {
		if x = strings.TrimSpace(strings.ToLower(x)); x != "" {
			roles[x] = true
		}
	}
	owners := map[string][]model.Owner{}
	for _, a := range assigns {
		t := firstSet(a.ObjectType, a.ContentType)
		role := ""
		if a.Role != nil {
			role = a.Role.Name
		}
		if len(roles) > 0 && !roles[strings.ToLower(role)] {
			continue
		}
		c := byContact[a.Contact.ID]
		if c.Name == "" {
			c.Name = a.Contact.Name
		}
		k := t + ":" + strconv.Itoa(a.ObjectID)
		owners[k] = append(owners[k], model.Owner{Name: c.Name, Email: c.Email, Phone: c.Phone, Role: role, From: "netbox"})
	}
	ownersOf := func(o nbObject) []model.Owner {
		if list := owners[o.key()]; len(list) > 0 {
			return list
		}
		if o.Site != nil {
			if list := owners[objSite+":"+strconv.Itoa(o.Site.ID)]; len(list) > 0 {
				return withFrom(list, "netbox: площадка "+o.Site.Name)
			}
		}
		if o.Tenant != nil {
			if list := owners[objTenant+":"+strconv.Itoa(o.Tenant.ID)]; len(list) > 0 {
				return withFrom(list, "netbox: арендатор "+o.Tenant.Name)
			}
		}
		return []model.Owner{}
	}
	base := strings.TrimRight(it.URL, "/")
	hosts := make([]Host, 0, len(objects))
	for _, o := range objects {
		h := Host{Key: o.key(), Name: o.Name, Type: o.ciType(), Labels: map[string]string{}, Owners: ownersOf(o), OwnersSet: true,
			ExternalURL: o.webURL(base), Description: o.Description}
		if h.Description == "" && o.DeviceType != nil {
			h.Description = firstSet(o.DeviceType.Model, o.DeviceType.Name)
		}
		switch p("team_from") {
		case "tenant":
			if o.Tenant != nil {
				h.Team = firstSet(o.Tenant.Slug, o.Tenant.Name)
			}
		case "site":
			if o.Site != nil {
				h.Team = firstSet(o.Site.Slug, o.Site.Name)
			}
		}
		if h.Team == "" {
			h.Team = it.Team
		}
		if o.Cluster != nil {
			h.LogicalGroup = o.Cluster.Name
		} else if o.Site != nil {
			h.LogicalGroup = o.Site.Name
		}
		h.Labels["netbox_role"] = o.role()
		if o.Site != nil {
			h.Labels["netbox_site"] = o.Site.Name
		}
		if o.Platform != nil {
			h.Labels["netbox_platform"] = o.Platform.Name
		}
		if o.Tenant != nil {
			h.Labels["netbox_tenant"] = o.Tenant.Name
		}
		if o.Status != nil {
			h.Labels["netbox_status"] = o.Status.Value
		}
		if o.PrimaryIP != nil {
			ip, _, _ := strings.Cut(o.PrimaryIP.Address, "/")
			h.IPs = append(h.IPs, ip)
		}
		hosts = append(hosts, h)
	}
	return hosts, fmt.Sprintf("NetBox %s: устройств %d, ВМ %d", ver, devices, vms), nil
}

func withFrom(list []model.Owner, from string) []model.Owner {
	out := make([]model.Owner, len(list))
	for i, o := range list {
		o.From = from
		out[i] = o
	}
	return out
}

func setIdentity(ci *model.CI, kind, value string, now time.Time) {
	if value == "" || activeIdentity(ci, kind, value) {
		return
	}
	ci.Identities = append(ci.Identities, model.Identity{Kind: kind, Value: value, Since: now})
}

func firstSet(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
