package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pipeline"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/secrets"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

var (
	ErrNotFound = errors.New("интеграция не найдена")
	slugRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,40}$`)
)

type Vault interface {
	secrets.Resolver
	Enabled() bool
	PutRef(ctx context.Context, path, key, value string) (string, error)
	Delete(ctx context.Context, path string) error
	Owns(ref, path string) bool
}

type Manager struct {
	st        *store.Store
	vault     Vault
	publicURL string
	feed      Feed
}

type Feed func(connectorID, body, token string) (int, error)

func (m *Manager) SetFeed(f Feed) { m.feed = f }

func NewManager(st *store.Store, vault Vault, publicURL string) *Manager {
	return &Manager{st: st, vault: vault, publicURL: strings.TrimRight(publicURL, "/")}
}

type Input struct {
	Name          *string           `json:"name"`
	Type          string            `json:"type"`
	Team          *string           `json:"team"`
	Slug          *string           `json:"slug"`
	URL           *string           `json:"url"`
	AuthType      *string           `json:"auth_type"`
	Username      *string           `json:"username"`
	Secret        *string           `json:"secret"`
	WebhookToken  *string           `json:"webhook_token"`
	TLSSkipVerify *bool             `json:"tls_skip_verify"`
	Params        map[string]string `json:"params"`
	Regenerate    bool              `json:"regenerate"`
}

type View struct {
	model.Integration
	Mode        string     `json:"mode"`
	Title       string     `json:"type_title"`
	Slug        string     `json:"slug,omitempty"`
	Status      string     `json:"connector_status,omitempty"`
	Version     int        `json:"connector_version,omitempty"`
	Edited      bool       `json:"connector_edited"`
	EventsTotal int        `json:"events_total"`
	ErrorsTotal int        `json:"errors_total"`
	LastEventAt *time.Time `json:"last_event_at,omitempty"`
	IngestURL   string     `json:"ingest_url,omitempty"`
	TokenHeader string     `json:"token_header,omitempty"`
	Snippet     string     `json:"snippet,omitempty"`
	SetupHelp   string     `json:"setup_help,omitempty"`
	CanSetup    bool       `json:"can_setup"`
	CanSync     bool       `json:"can_sync"`
	SecretSet   bool       `json:"secret_set"`
}

func secretPath(id string) string { return "integrations/" + id }

func graphHash(g model.Graph) string {
	b, _ := json.Marshal(g)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

func (m *Manager) ingestURL(it model.Integration, slug string) string {
	base := m.publicURL
	if v := strings.TrimRight(it.Params["umbrella_url"], "/"); v != "" {
		base = v
	}
	return base + "/api/ingest/" + slug
}

func (m *Manager) view(d *store.Data, it *model.Integration) View {
	spec, _ := Spec(it.Type)
	v := View{Integration: *it, Mode: spec.Mode, Title: spec.Title, CanSetup: spec.Setup, SetupHelp: spec.SetupHelp, SecretSet: it.SecretRef != "",
		CanSync: syncEnabled(*it)}
	v.Params = map[string]string{}
	for k, x := range it.Params {
		v.Params[k] = x
	}
	if c := d.Connectors[it.ConnectorID]; c != nil {
		v.Slug, v.Status, v.Version = c.Slug, string(c.Status), c.Version
		v.EventsTotal, v.ErrorsTotal, v.LastEventAt = c.EventsTotal, c.ErrorsTotal, c.LastEventAt
		if c.Published != nil && it.Remote["graph"] != "" && graphHash(*c.Published) != it.Remote["graph"] {
			v.Edited = true
		}
		if spec.Mode == ModePush {
			ref := c.Slug
			if ref == "" {
				ref = c.ID
			}
			v.IngestURL = m.ingestURL(*it, ref)
			if it.WebhookTokenRef != "" {
				v.TokenHeader = "X-Umbrella-Token или Authorization: Bearer"
			}
			if it.Type == TypeAlertmanager {
				v.Snippet = alertmanagerSnippet(v.IngestURL, it.WebhookTokenRef != "")
			}
		}
	}
	return v
}

func alertmanagerSnippet(ingest string, token bool) string {
	s := "route:\n  receiver: umbrella\n  group_wait: 10s\n  group_interval: 30s\n  repeat_interval: 1h\nreceivers:\n  - name: umbrella\n    webhook_configs:\n      - url: " + ingest + "\n        send_resolved: true\n"
	if token {
		s += "        http_config:\n          authorization:\n            type: Bearer\n            credentials: <токен приёма>\n"
	}
	return s
}

func (m *Manager) List() []View {
	var out []View
	m.st.Read(func(d *store.Data) {
		for _, it := range d.Integrations {
			out = append(out, m.view(d, it))
		}
	})
	return out
}

func (m *Manager) Get(id string) (View, bool) {
	var v View
	ok := false
	m.st.Read(func(d *store.Data) {
		if it := d.Integrations[id]; it != nil {
			v, ok = m.view(d, it), true
		}
	})
	return v, ok
}

func validate(spec TypeSpec, it *model.Integration) error {
	if strings.TrimSpace(it.Name) == "" {
		return errors.New("нужно название интеграции")
	}
	if spec.URLRequired && strings.TrimSpace(it.URL) == "" {
		return fmt.Errorf("нужно поле «%s»", spec.URLLabel)
	}
	if it.URL != "" {
		u, err := url.Parse(it.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("адрес должен начинаться с http:// или https://")
		}
	}
	if it.AuthType == "" {
		it.AuthType = spec.Auth[0]
	}
	allowed := false
	for _, a := range spec.Auth {
		if a == it.AuthType {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("способ входа %q не поддерживается для %s", it.AuthType, spec.Title)
	}
	if it.AuthType == AuthBasic && strings.TrimSpace(it.Username) == "" {
		return errors.New("для входа по логину нужен логин")
	}
	for _, f := range spec.Params {
		if f.Required && strings.TrimSpace(spec.param(it.Params, f.Key)) == "" {
			return fmt.Errorf("нужно поле «%s»", f.Label)
		}
	}
	for _, k := range []string{"interval"} {
		if v := spec.param(it.Params, k); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil || d < 5*time.Second {
				return errors.New("интервал опроса: не меньше 5s, например 30s или 1m")
			}
		}
	}
	if v := it.Params["umbrella_url"]; v != "" {
		if u, err := url.Parse(v); err != nil || u.Host == "" {
			return errors.New("адрес Umbrella для источника должен быть полным URL")
		}
	}
	return nil
}

func (m *Manager) Create(ctx context.Context, in Input, actor string) (View, error) {
	spec, ok := Spec(in.Type)
	if !ok {
		return View{}, fmt.Errorf("неизвестный тип интеграции %q", in.Type)
	}
	if !m.vault.Enabled() {
		return View{}, secrets.ErrNotConfigured
	}
	it := model.Integration{Type: in.Type, Params: map[string]string{}, Remote: map[string]string{}}
	apply(&it, in)
	if err := validate(spec, &it); err != nil {
		return View{}, err
	}
	slug := in.Type
	if in.Slug != nil && strings.TrimSpace(*in.Slug) != "" {
		slug = strings.TrimSpace(*in.Slug)
		if !slugRe.MatchString(slug) {
			return View{}, errors.New("короткое имя: латиница в нижнем регистре, цифры и дефис")
		}
	}
	m.st.Write(func(d *store.Data) {
		it.ID = d.NextID("INT")
		if in.Slug == nil || strings.TrimSpace(*in.Slug) == "" {
			slug = freeSlug(d, slug)
		}
	})
	if err := m.storeSecrets(ctx, &it, in, spec, true); err != nil {
		return View{}, err
	}
	if !hasConnector(spec) {
		now := time.Now()
		var out View
		m.st.Write(func(d *store.Data) {
			it.CreatedAt, it.UpdatedAt, it.UpdatedBy = now, now, actor
			d.Integrations[it.ID] = &it
			d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: actor, Action: "integration.create " + it.Type, Object: it.ID})
			out = m.view(d, &it)
		})
		return out, nil
	}
	g, err := Graph(it)
	if err != nil {
		m.dropSecrets(ctx, it)
		return View{}, err
	}
	if _, err := pipeline.Validate(g); err != nil {
		m.dropSecrets(ctx, it)
		return View{}, err
	}
	now := time.Now()
	var out View
	m.st.Write(func(d *store.Data) {
		for _, c := range d.Connectors {
			if c.Slug == slug {
				err = fmt.Errorf("короткое имя %s уже занято коннектором %s", slug, c.ID)
				return
			}
		}
		pub := g
		c := &model.Connector{ID: d.NextID("CON"), Slug: slug, Name: it.Name, Team: it.Team,
			Description: spec.Title + ": интеграция " + it.ID, Status: model.ConnectorRunning, Version: 1,
			Draft: g, Published: &pub, SampleInput: Sample(it.Type), UpdatedAt: now, UpdatedBy: actor}
		d.Connectors[c.ID] = c
		it.ConnectorID = c.ID
		it.Remote["graph"] = graphHash(g)
		it.CreatedAt, it.UpdatedAt, it.UpdatedBy = now, now, actor
		d.Integrations[it.ID] = &it
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: actor, Action: "integration.create " + it.Type, Object: it.ID})
		out = m.view(d, &it)
	})
	if err != nil {
		m.dropSecrets(ctx, it)
		return View{}, err
	}
	return out, nil
}

func hasConnector(spec TypeSpec) bool { return spec.Mode == ModePush || spec.Mode == ModePull }

func freeSlug(d *store.Data, base string) string {
	taken := map[string]bool{}
	for _, c := range d.Connectors {
		taken[c.Slug] = true
	}
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		s := fmt.Sprintf("%s-%d", base, i)
		if !taken[s] {
			return s
		}
	}
}

func apply(it *model.Integration, in Input) {
	if in.Name != nil {
		it.Name = strings.TrimSpace(*in.Name)
	}
	if in.Team != nil {
		it.Team = strings.TrimSpace(*in.Team)
	}
	if in.URL != nil {
		it.URL = strings.TrimSpace(*in.URL)
	}
	if in.AuthType != nil {
		it.AuthType = *in.AuthType
	}
	if in.Username != nil {
		it.Username = strings.TrimSpace(*in.Username)
	}
	if in.TLSSkipVerify != nil {
		it.TLSSkipVerify = *in.TLSSkipVerify
	}
	for k, v := range in.Params {
		if v == "" {
			delete(it.Params, k)
		} else {
			it.Params[k] = v
		}
	}
}

func (m *Manager) storeSecrets(ctx context.Context, it *model.Integration, in Input, spec TypeSpec, create bool) error {
	path := secretPath(it.ID)
	if in.Secret != nil && *in.Secret != "" {
		ref, err := m.vault.PutRef(ctx, path, "secret", *in.Secret)
		if err != nil {
			return err
		}
		it.SecretRef = ref
	}
	if it.AuthType == AuthNone {
		it.SecretRef = ""
	} else if it.SecretRef == "" {
		return fmt.Errorf("нужен %s", strings.ToLower(spec.SecretLabel))
	}
	if spec.Mode == ModePush {
		tok := ""
		if in.WebhookToken != nil {
			tok = strings.TrimSpace(*in.WebhookToken)
		}
		if tok == "" && create {
			tok = auth.RandomToken("", 24)
		}
		if tok != "" {
			ref, err := m.vault.PutRef(ctx, path, "webhook_token", tok)
			if err != nil {
				return err
			}
			it.WebhookTokenRef = ref
		}
	}
	return nil
}

func (m *Manager) dropSecrets(ctx context.Context, it model.Integration) {
	_ = m.vault.Delete(ctx, secretPath(it.ID))
}

func (m *Manager) Update(ctx context.Context, id string, in Input, actor string) (View, error) {
	var it model.Integration
	found := false
	m.st.Read(func(d *store.Data) {
		if p := d.Integrations[id]; p != nil {
			it, found = clone(*p), true
		}
	})
	if !found {
		return View{}, ErrNotFound
	}
	spec, _ := Spec(it.Type)
	apply(&it, in)
	if err := validate(spec, &it); err != nil {
		return View{}, err
	}
	if err := m.storeSecrets(ctx, &it, in, spec, false); err != nil {
		return View{}, err
	}
	var g model.Graph
	if hasConnector(spec) {
		var err error
		if g, err = Graph(it); err != nil {
			return View{}, err
		}
		if _, err := pipeline.Validate(g); err != nil {
			return View{}, err
		}
	}
	var err error
	now := time.Now()
	var out View
	m.st.Write(func(d *store.Data) {
		p := d.Integrations[id]
		if p == nil {
			err = ErrNotFound
			return
		}
		c := d.Connectors[it.ConnectorID]
		if c != nil && hasConnector(spec) {
			if in.Slug != nil && strings.TrimSpace(*in.Slug) != "" && *in.Slug != c.Slug {
				s := strings.TrimSpace(*in.Slug)
				if !slugRe.MatchString(s) {
					err = errors.New("короткое имя: латиница в нижнем регистре, цифры и дефис")
					return
				}
				for _, o := range d.Connectors {
					if o.ID != c.ID && o.Slug == s {
						err = fmt.Errorf("короткое имя %s уже занято коннектором %s", s, o.ID)
						return
					}
				}
				c.Slug = s
			}
			edited := c.Published != nil && it.Remote["graph"] != "" && graphHash(*c.Published) != it.Remote["graph"]
			next := g
			if edited && !in.Regenerate {
				next = patch(*c.Published, g)
			}
			if c.Published == nil || graphHash(*c.Published) != graphHash(next) {
				pub := next
				c.Draft, c.Published, c.DraftDirty = next, &pub, false
				c.Version++
			}
			if !edited || in.Regenerate {
				it.Remote["graph"] = graphHash(next)
			}
			c.Name, c.Team, c.UpdatedAt, c.UpdatedBy = it.Name, it.Team, now, actor
		}
		it.UpdatedAt, it.UpdatedBy = now, actor
		*p = it
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: actor, Action: "integration.update", Object: id})
		out = m.view(d, p)
	})
	return out, err
}

func patch(cur, gen model.Graph) model.Graph {
	byKind := map[string]map[string]string{}
	for _, n := range gen.Nodes {
		if strings.HasPrefix(n.Kind, "trigger.") || n.Kind == "fetch.http" {
			byKind[n.Kind] = n.Config
		}
	}
	out := model.Graph{Edges: append([]model.Edge(nil), cur.Edges...)}
	for _, n := range cur.Nodes {
		if cfg, ok := byKind[n.Kind]; ok {
			n.Config = cfg
		}
		out.Nodes = append(out.Nodes, n)
	}
	return out
}

func clone(it model.Integration) model.Integration {
	c := it
	c.Params = map[string]string{}
	for k, v := range it.Params {
		c.Params[k] = v
	}
	c.Remote = map[string]string{}
	for k, v := range it.Remote {
		c.Remote[k] = v
	}
	return c
}

func (m *Manager) Delete(ctx context.Context, id string, teardown bool, actor string) (string, error) {
	var it model.Integration
	found := false
	m.st.Read(func(d *store.Data) {
		if p := d.Integrations[id]; p != nil {
			it, found = clone(*p), true
		}
	})
	if !found {
		return "", ErrNotFound
	}
	note := ""
	if teardown && it.SetupAt != nil {
		if err := m.teardown(ctx, it); err != nil {
			note = "объекты в источнике не удалены: " + err.Error()
		} else {
			note = "объекты в источнике удалены"
		}
	}
	m.dropSecrets(ctx, it)
	now := time.Now()
	m.st.Write(func(d *store.Data) {
		delete(d.Connectors, it.ConnectorID)
		delete(d.Integrations, id)
		d.AddAudit(store.AuditEntry{At: now.Format(time.RFC3339), Actor: actor, Action: "integration.delete", Object: id})
	})
	return note, nil
}

func (m *Manager) secret(it model.Integration) (string, error) {
	if it.SecretRef == "" || it.AuthType == AuthNone {
		return "", nil
	}
	return m.vault.Resolve(it.SecretRef)
}

type Result struct {
	OK      bool      `json:"ok"`
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

func (m *Manager) Check(ctx context.Context, id string) (Result, error) {
	v, ok := m.Get(id)
	if !ok {
		return Result{}, ErrNotFound
	}
	msg, err := m.check(ctx, v)
	res := Result{OK: err == nil, Message: msg, At: time.Now()}
	if err != nil {
		res.Message = err.Error()
	}
	m.st.Write(func(d *store.Data) {
		if p := d.Integrations[id]; p != nil {
			t := res.At
			p.LastCheckAt, p.LastCheckOK, p.LastCheck = &t, res.OK, res.Message
		}
	})
	return res, nil
}

func (m *Manager) check(ctx context.Context, v View) (string, error) {
	it := v.Integration
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	secret, err := m.secret(it)
	if err != nil {
		return "", err
	}
	spec, _ := Spec(it.Type)
	switch it.Type {
	case TypeZabbix:
		z := newZabbix(it, secret)
		ver, err := z.version(ctx)
		if err != nil {
			return "", err
		}
		if err := z.open(ctx, it.Username, secret); err != nil {
			return "", err
		}
		defer z.close(ctx)
		var hosts string
		if err := z.call(ctx, "host.get", map[string]any{"countOutput": true}, true, &hosts); err != nil {
			return "", err
		}
		return fmt.Sprintf("Zabbix %s, узлов сети: %s", ver, hosts), nil
	case TypeAlertmanager:
		if it.URL == "" {
			return "адрес Alertmanager не задан: проверяется только приём событий" + pushState(v), nil
		}
		var st struct {
			VersionInfo struct {
				Version string `json:"version"`
			} `json:"versionInfo"`
			Cluster struct {
				Status string `json:"status"`
			} `json:"cluster"`
		}
		if err := newRemote(it, secret).do(ctx, http.MethodGet, "/api/v2/status", nil, &st); err != nil {
			return "", err
		}
		return fmt.Sprintf("Alertmanager %s, кластер %s", st.VersionInfo.Version, st.Cluster.Status) + pushState(v), nil
	case TypeGrafana:
		r := newRemote(it, secret)
		var h struct {
			Version string `json:"version"`
		}
		if err := r.do(ctx, http.MethodGet, "/api/health", nil, &h); err != nil {
			return "", err
		}
		var cps []contactPoint
		if err := r.do(ctx, http.MethodGet, "/api/v1/provisioning/contact-points", nil, &cps); err != nil {
			return "", fmt.Errorf("Grafana %s: нет доступа к Alerting: %w", h.Version, err)
		}
		return fmt.Sprintf("Grafana %s, точек контакта: %d", h.Version, len(cps)) + pushState(v), nil
	case TypeOpenSearch, TypeElasticsearch:
		r := newRemote(it, secret)
		var info struct {
			Version struct {
				Number       string `json:"number"`
				Distribution string `json:"distribution"`
			} `json:"version"`
		}
		if err := r.do(ctx, http.MethodGet, "/", nil, &info); err != nil {
			return "", err
		}
		var cnt struct {
			Count int `json:"count"`
		}
		index := spec.param(it.Params, "index")
		if err := r.do(ctx, http.MethodGet, "/"+strings.Trim(index, "/")+"/_count?ignore_unavailable=true", nil, &cnt); err != nil {
			return "", fmt.Errorf("индекс %s: %w", index, err)
		}
		dist := info.Version.Distribution
		if dist == "" {
			dist = it.Type
		}
		return fmt.Sprintf("%s %s, документов в %s: %d", dist, info.Version.Number, index, cnt.Count), nil
	case TypeHTTP:
		r := newRemote(it, secret)
		method := spec.param(it.Params, "method")
		var body any
		if b := spec.param(it.Params, "body"); b != "" {
			var x any
			if json.Unmarshal([]byte(b), &x) == nil {
				body = x
			}
		}
		r.base = ""
		var out any
		if err := r.do(ctx, method, it.URL, body, &out); err != nil {
			return "", err
		}
		return "источник ответил 2xx, JSON разобран", nil
	case TypeWebhook:
		return "приём по адресу " + v.IngestURL + pushState(v), nil
	case TypePrometheus:
		r := newRemote(it, secret)
		var bi struct {
			Data struct {
				Version string `json:"version"`
			} `json:"data"`
		}
		ver := "API"
		if err := r.do(ctx, http.MethodGet, "/api/v1/status/buildinfo", nil, &bi); err == nil && bi.Data.Version != "" {
			ver = bi.Data.Version
		}
		samples, err := promQuery(ctx, r, "up")
		if err != nil {
			return "", err
		}
		up := 0
		for _, x := range samples {
			if x.Value == 1 {
				up++
			}
		}
		return fmt.Sprintf("Prometheus %s, целей up: %d из %d", ver, up, len(samples)), nil
	case TypeNetBox:
		r := newRemote(it, secret)
		ver, err := netboxVersion(ctx, r)
		if err != nil {
			return "", err
		}
		var cnt struct {
			Count int `json:"count"`
		}
		if err := r.do(ctx, http.MethodGet, "/api/dcim/devices/?limit=1&brief=1", nil, &cnt); err != nil {
			return "", err
		}
		return fmt.Sprintf("NetBox %s, устройств: %d", ver, cnt.Count), nil
	}
	return "", errors.New("проверка не поддерживается")
}

func pushState(v View) string {
	if v.LastEventAt == nil {
		return "; событий от источника ещё не было"
	}
	return fmt.Sprintf("; последнее событие %s, всего %d", v.LastEventAt.Format("02.01 15:04:05"), v.EventsTotal)
}

func (m *Manager) Setup(ctx context.Context, id, actor string) (Result, error) {
	v, ok := m.Get(id)
	if !ok {
		return Result{}, ErrNotFound
	}
	it := v.Integration
	spec, _ := Spec(it.Type)
	if !spec.Setup {
		return Result{}, errors.New("для этого типа источник настраивается вручную")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	res := Result{At: time.Now()}
	remote, info, err := m.setup(ctx, v)
	if err != nil {
		res.Message = err.Error()
	} else {
		res.OK, res.Message = true, info
	}
	m.st.Write(func(d *store.Data) {
		p := d.Integrations[id]
		if p == nil {
			return
		}
		if err == nil {
			t := res.At
			p.SetupAt = &t
			if p.Remote == nil {
				p.Remote = map[string]string{}
			}
			for k, x := range remote {
				p.Remote[k] = x
			}
		}
		p.SetupInfo = res.Message
		d.AddAudit(store.AuditEntry{At: res.At.Format(time.RFC3339), Actor: actor, Action: fmt.Sprintf("integration.setup ok=%v", res.OK), Object: id})
	})
	return res, nil
}

func (m *Manager) setup(ctx context.Context, v View) (map[string]string, string, error) {
	it := v.Integration
	secret, err := m.secret(it)
	if err != nil {
		return nil, "", err
	}
	token := ""
	if it.WebhookTokenRef != "" {
		if token, err = m.vault.Resolve(it.WebhookTokenRef); err != nil {
			return nil, "", err
		}
	}
	switch it.Type {
	case TypeZabbix:
		z := newZabbix(it, secret)
		if err := z.open(ctx, it.Username, secret); err != nil {
			return nil, "", err
		}
		defer z.close(ctx)
		remote, info, err := z.setup(ctx, it, v.IngestURL, token)
		if err != nil {
			return nil, "", err
		}
		return remote, info + m.backfillZabbix(ctx, z, it, token), nil
	case TypeGrafana:
		return grafanaSetup(ctx, newRemote(it, secret), it, v.IngestURL, token)
	}
	return nil, "", errors.New("автонастройка не поддерживается")
}

func (m *Manager) teardown(ctx context.Context, it model.Integration) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	secret, err := m.secret(it)
	if err != nil {
		return err
	}
	switch it.Type {
	case TypeZabbix:
		z := newZabbix(it, secret)
		if err := z.open(ctx, it.Username, secret); err != nil {
			return err
		}
		defer z.close(ctx)
		return z.teardown(ctx, it.Remote)
	case TypeGrafana:
		return grafanaTeardown(ctx, newRemote(it, secret), it.Remote)
	}
	return nil
}

func (m *Manager) RevealToken(id string) (string, error) {
	v, ok := m.Get(id)
	if !ok {
		return "", ErrNotFound
	}
	if v.WebhookTokenRef == "" {
		return "", errors.New("у интеграции нет токена приёма")
	}
	return m.vault.Resolve(v.WebhookTokenRef)
}

func urlQuery(s string) string { return url.QueryEscape(s) }

func (m *Manager) backfillZabbix(ctx context.Context, z *zabbix, it model.Integration, token string) string {
	if m.feed == nil || it.ConnectorID == "" {
		return ""
	}
	bodies, err := z.openProblems(ctx, it)
	if err != nil {
		return "; открытые проблемы не загружены: " + err.Error()
	}
	sent := 0
	for _, b := range bodies {
		if _, err := m.feed(it.ConnectorID, b, token); err != nil {
			return fmt.Sprintf("; открытых проблем передано %d из %d: %v", sent, len(bodies), err)
		}
		sent++
	}
	return fmt.Sprintf("; открытых проблем передано %d", sent)
}
