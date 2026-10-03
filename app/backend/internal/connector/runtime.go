package connector

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pipeline"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/secrets"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type Secrets = secrets.Resolver

type Runtime struct {
	st       *store.Store
	eng      *alert.Engine
	secrets  Secrets
	client   *http.Client
	insecure *http.Client
	notify   func(kind string, v any)
	lastRun  map[string]time.Time
}

func New(st *store.Store, eng *alert.Engine, secrets Secrets, notify func(string, any)) *Runtime {
	if notify == nil {
		notify = func(string, any) {}
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	return &Runtime{st: st, eng: eng, secrets: secrets, client: &http.Client{Timeout: 20 * time.Second},
		insecure: &http.Client{Timeout: 20 * time.Second, Transport: tr}, notify: notify, lastRun: map[string]time.Time{}}
}

var (
	ErrNotFound     = errors.New("коннектор не найден")
	ErrStopped      = errors.New("коннектор остановлен")
	ErrNotPublished = errors.New("у коннектора нет опубликованной версии")
	ErrUnauthorized = errors.New("неверный токен источника")
	ErrNotWebhook   = errors.New("коннектор не принимает webhook")
)

func (r *Runtime) snapshot(id string) (model.Connector, bool) {
	var c model.Connector
	ok := false
	r.st.Read(func(d *store.Data) {
		if p := d.Connectors[id]; p != nil {
			c = *p
			if p.Published != nil {
				g := *p.Published
				c.Published = &g
			}
			ok = true
		}
	})
	return c, ok
}

func trigger(g *model.Graph) *model.Node {
	for i := range g.Nodes {
		if strings.HasPrefix(g.Nodes[i].Kind, "trigger.") {
			return &g.Nodes[i]
		}
	}
	return nil
}

func (r *Runtime) Webhook(id, body, token string) (int, error) {
	c, ok := r.snapshot(id)
	if !ok {
		return 0, ErrNotFound
	}
	if c.Status != model.ConnectorRunning {
		return 0, ErrStopped
	}
	if c.Published == nil {
		return 0, ErrNotPublished
	}
	t := trigger(c.Published)
	if t == nil || t.Kind != "trigger.webhook" {
		return 0, ErrNotWebhook
	}
	if t.Config["auth"] != "none" && t.Config["secret_ref"] != "" {
		want, err := r.secrets.Resolve(t.Config["secret_ref"])
		if err != nil {
			return 0, err
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(token)) != 1 {
			return 0, ErrUnauthorized
		}
	}
	return r.execute(c, body)
}

func (r *Runtime) execute(c model.Connector, body string) (int, error) {
	res, err := pipeline.Run(*c.Published, body)
	if err != nil {
		r.parseError(c, "graph", err.Error(), body)
		return 0, err
	}
	for _, be := range res.Errors {
		r.parseError(c, be.Kind, be.Error, be.Raw)
	}
	events := r.eng.Ingest(alert.Source{ID: c.ID, Name: c.Name}, res.Events)
	return len(events), nil
}

func (r *Runtime) parseError(c model.Connector, block, msg, raw string) {
	var pe model.ParseError
	r.st.Write(func(d *store.Data) {
		pe = model.ParseError{ID: d.NextID("ERR"), ConnectorID: c.ID, Connector: c.Name, Block: block, Error: msg, Raw: raw, At: time.Now()}
		d.AddParseError(&pe)
		if p := d.Connectors[c.ID]; p != nil {
			p.ErrorsTotal++
		}
	})
	r.notify("parse_error", pe)
}

type DryRun struct {
	pipeline.Result
	Error string      `json:"error,omitempty"`
	CIs   []CIPreview `json:"cis"`
}

type CIPreview struct {
	Input  string `json:"input"`
	CIID   string `json:"ci_id,omitempty"`
	CIName string `json:"ci_name,omitempty"`
	Found  bool   `json:"found"`
}

func (r *Runtime) DryRun(g model.Graph, sample string) DryRun {
	res, err := pipeline.Run(g, sample)
	out := DryRun{Result: res, CIs: []CIPreview{}}
	if err != nil {
		out.Error = err.Error()
		return out
	}
	r.st.Read(func(d *store.Data) {
		for _, ev := range res.Events {
			p := CIPreview{Input: ev.CI}
			if ci := alert.ResolveCI(d, ev.CI, ev.Labels); ci != nil {
				p.CIID, p.CIName, p.Found = ci.ID, ci.Name, true
			}
			out.CIs = append(out.CIs, p)
		}
	})
	return out
}

func (r *Runtime) Run(ctx context.Context) {
	tk := time.NewTicker(2 * time.Second)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tk.C:
			r.pollDue(ctx, now)
		}
	}
}

func (r *Runtime) pollDue(ctx context.Context, now time.Time) {
	var due []model.Connector
	r.st.Read(func(d *store.Data) {
		for _, c := range d.Connectors {
			if c.Status != model.ConnectorRunning || c.Published == nil {
				continue
			}
			t := trigger(c.Published)
			if t == nil || t.Kind != "trigger.schedule" {
				continue
			}
			iv, err := time.ParseDuration(t.Config["interval"])
			if err != nil || iv < 5*time.Second {
				iv = time.Minute
			}
			if now.Sub(r.lastRun[c.ID]) >= iv {
				cc := *c
				g := *c.Published
				cc.Published = &g
				due = append(due, cc)
			}
		}
	})
	for _, c := range due {
		r.lastRun[c.ID] = now
		body, err := r.fetch(ctx, c.Published)
		if err != nil {
			r.parseError(c, "fetch.http", err.Error(), "")
			continue
		}
		if _, err := r.execute(c, body); err != nil {
			slog.Warn("connector run failed", "connector", c.ID, "err", err)
		}
	}
}

func (r *Runtime) fetch(ctx context.Context, g *model.Graph) (string, error) {
	var n *model.Node
	for i := range g.Nodes {
		if g.Nodes[i].Kind == "fetch.http" {
			n = &g.Nodes[i]
		}
	}
	if n == nil {
		return "", errors.New("нет блока «HTTP-запрос» после расписания")
	}
	body, _, err := r.Fetch(ctx, n.Config)
	return body, err
}

func (r *Runtime) Fetch(ctx context.Context, cfg map[string]string) (string, int, error) {
	method := cfg["method"]
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, cfg["url"], strings.NewReader(cfg["body"]))
	if err != nil {
		return "", 0, err
	}
	if cfg["body"] != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, line := range strings.Split(cfg["headers"], "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) != "" {
			req.Header.Set(strings.TrimSpace(k), strings.TrimSpace(v))
		}
	}
	if err := r.authorize(req, cfg); err != nil {
		return "", 0, err
	}
	client := r.client
	if cfg["tls_skip_verify"] == "true" {
		client = r.insecure
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", resp.StatusCode, err
	}
	if resp.StatusCode/100 != 2 {
		return string(b), resp.StatusCode, fmt.Errorf("источник ответил %d", resp.StatusCode)
	}
	return string(b), resp.StatusCode, nil
}

func (r *Runtime) authorize(req *http.Request, cfg map[string]string) error {
	mode := cfg["auth"]
	ref := cfg["secret_ref"]
	if mode == "" && ref != "" {
		mode = "header"
	}
	if mode == "" || mode == "none" {
		return nil
	}
	if ref == "" {
		return errors.New("в блоке «HTTP-запрос» не задана ссылка на секрет")
	}
	v, err := r.secrets.Resolve(ref)
	if err != nil {
		return err
	}
	switch mode {
	case "basic":
		req.SetBasicAuth(cfg["username"], v)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+v)
	case "apikey":
		req.Header.Set("Authorization", "ApiKey "+v)
	default:
		h := cfg["auth_header"]
		if h == "" {
			h = "Authorization"
		}
		req.Header.Set(h, v)
	}
	return nil
}
