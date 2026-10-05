package rules

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/integration"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/pipeline"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type Querier interface {
	Query(ctx context.Context, sourceID, promql string) ([]integration.MetricSample, error)
}

type Ingester interface {
	Ingest(src alert.Source, drafts []pipeline.Draft) []model.Event
}

var Ops = []string{">", ">=", "<", "<=", "==", "!="}

const minInterval = 10 * time.Second

type Engine struct {
	st  *store.Store
	q   Querier
	in  Ingester
	now func() time.Time
}

func New(st *store.Store, q Querier, in Ingester) *Engine {
	return &Engine{st: st, q: q, in: in, now: time.Now}
}

func (e *Engine) SetClock(now func() time.Time) { e.now = now }

func Normalize(r *model.Rule) error {
	r.Name = strings.TrimSpace(r.Name)
	r.Query = strings.TrimSpace(r.Query)
	r.Signal = strings.TrimSpace(r.Signal)
	if r.Name == "" {
		return errors.New("нужно название правила")
	}
	if r.Method != model.MethodRED && r.Method != model.MethodUSE {
		return errors.New("метод: red или use")
	}
	if r.Signal == "" {
		r.Signal = string(r.Method) + "." + strings.ToLower(strings.ReplaceAll(r.Name, " ", "_"))
	}
	if r.SourceID == "" {
		return errors.New("выберите источник метрик")
	}
	if r.Query == "" {
		return errors.New("нужен запрос PromQL")
	}
	if !contains(Ops, r.Op) {
		return errors.New("условие: >, >=, <, <=, == или !=")
	}
	if math.IsNaN(r.Threshold) || math.IsInf(r.Threshold, 0) {
		return errors.New("порог должен быть числом")
	}
	if r.For == "" {
		r.For = "0s"
	}
	if d, err := time.ParseDuration(r.For); err != nil || d < 0 {
		return errors.New("длительность: например 0s, 1m или 5m")
	}
	if r.Interval == "" {
		r.Interval = "30s"
	}
	if d, err := time.ParseDuration(r.Interval); err != nil || d < minInterval {
		return errors.New("интервал вычисления: не меньше 10s")
	}
	if !r.Severity.Valid() {
		return errors.New("важность: critical, error, warning или info")
	}
	if r.CILabel == "" {
		r.CILabel = "host"
	}
	if strings.TrimSpace(r.Title) == "" {
		r.Title = r.Name + " на ${ci}: ${value}"
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func compare(v float64, op string, t float64) bool {
	switch op {
	case ">":
		return v > t
	case ">=":
		return v >= t
	case "<":
		return v < t
	case "<=":
		return v <= t
	case "==":
		return v == t
	case "!=":
		return v != t
	}
	return false
}

func seriesKey(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		if k != "__name__" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "=" + labels[k] + ",")
	}
	return b.String()
}

func ciOf(r model.Rule, labels map[string]string) string {
	if v := labels[r.CILabel]; v != "" {
		return v
	}
	if inst := labels["instance"]; inst != "" {
		if h, _, err := net.SplitHostPort(inst); err == nil {
			return h
		}
		return inst
	}
	return ""
}

func FormatValue(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e12 {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func title(r model.Rule, ci string, v float64, labels map[string]string) string {
	out := r.Title
	out = strings.ReplaceAll(out, "${ci}", ci)
	out = strings.ReplaceAll(out, "${value}", FormatValue(v))
	out = strings.ReplaceAll(out, "${threshold}", FormatValue(r.Threshold))
	for k, val := range labels {
		out = strings.ReplaceAll(out, "${labels."+k+"}", val)
	}
	return out
}

type PreviewSeries struct {
	CI     string            `json:"ci"`
	CIID   string            `json:"ci_id,omitempty"`
	Labels map[string]string `json:"labels"`
	Value  float64           `json:"value"`
	Match  bool              `json:"match"`
	Title  string            `json:"title"`
}

type Preview struct {
	Series  []PreviewSeries `json:"series"`
	Matched int             `json:"matched"`
	Error   string          `json:"error,omitempty"`
}

func (e *Engine) Preview(ctx context.Context, r model.Rule) Preview {
	out := Preview{Series: []PreviewSeries{}}
	if err := Normalize(&r); err != nil {
		out.Error = err.Error()
		return out
	}
	samples, err := e.q.Query(ctx, r.SourceID, r.Query)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	e.st.Read(func(d *store.Data) {
		for _, s := range samples {
			ci := ciOf(r, s.Labels)
			ps := PreviewSeries{CI: ci, Labels: s.Labels, Value: s.Value, Match: compare(s.Value, r.Op, r.Threshold), Title: title(r, ci, s.Value, s.Labels)}
			if c := alert.ResolveCI(d, ci, s.Labels); c != nil {
				ps.CIID = c.ID
			}
			if ps.Match {
				out.Matched++
			}
			out.Series = append(out.Series, ps)
		}
	})
	sort.SliceStable(out.Series, func(i, j int) bool { return out.Series[i].Match && !out.Series[j].Match })
	if len(out.Series) > 200 {
		out.Series = out.Series[:200]
	}
	return out
}

func (e *Engine) Run(ctx context.Context) {
	tk := time.NewTicker(5 * time.Second)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			e.tick(ctx)
		}
	}
}

func (e *Engine) tick(ctx context.Context) {
	now := e.now()
	var due []model.Rule
	e.st.Read(func(d *store.Data) {
		for _, r := range d.Rules {
			if !r.Enabled {
				continue
			}
			iv, err := time.ParseDuration(r.Interval)
			if err != nil || iv < minInterval {
				iv = 30 * time.Second
			}
			if r.LastEvalAt == nil || now.Sub(*r.LastEvalAt) >= iv {
				due = append(due, *r)
			}
		}
	})
	for _, r := range due {
		if err := e.Evaluate(ctx, r.ID); err != nil {
			slog.Debug("rule evaluation failed", "rule", r.ID, "err", err)
		}
	}
}

func (e *Engine) Evaluate(ctx context.Context, id string) error {
	var r model.Rule
	ok := false
	e.st.Read(func(d *store.Data) {
		if p := d.Rules[id]; p != nil {
			r, ok = *p, true
		}
	})
	if !ok {
		return errors.New("правило не найдено")
	}
	qctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	samples, qerr := e.q.Query(qctx, r.SourceID, r.Query)
	cancel()
	now := e.now()
	var drafts []pipeline.Draft
	e.st.Write(func(d *store.Data) {
		p := d.Rules[id]
		if p == nil {
			return
		}
		t := now
		p.LastEvalAt = &t
		if qerr != nil {
			p.LastError = qerr.Error()
			return
		}
		p.LastError = ""
		if p.State == nil {
			p.State = map[string]*model.RuleSeries{}
		}
		hold, _ := time.ParseDuration(p.For)
		seen := map[string]bool{}
		for _, s := range samples {
			key := seriesKey(s.Labels)
			seen[key] = true
			st := p.State[key]
			if !compare(s.Value, p.Op, p.Threshold) {
				if st != nil {
					if st.Firing {
						drafts = append(drafts, draft(*p, key, st, s.Value, model.EventResolved))
					}
					delete(p.State, key)
				}
				continue
			}
			if st == nil {
				st = &model.RuleSeries{CI: ciOf(*p, s.Labels), Labels: s.Labels, Since: now}
				p.State[key] = st
			}
			st.Value, st.LastSeen = s.Value, now
			if !st.Firing && now.Sub(st.Since) >= hold {
				st.Firing = true
				fired := now
				st.FiredAt = &fired
				drafts = append(drafts, draft(*p, key, st, s.Value, model.EventFiring))
			}
		}
		for key, st := range p.State {
			if seen[key] {
				continue
			}
			if st.Firing {
				drafts = append(drafts, draft(*p, key, st, st.Value, model.EventResolved))
			}
			delete(p.State, key)
		}
		p.SeriesCount, p.Pending, p.Firing = len(samples), 0, 0
		for _, st := range p.State {
			if st.Firing {
				p.Firing++
			} else {
				p.Pending++
			}
		}
	})
	if len(drafts) > 0 {
		e.in.Ingest(alert.Source{ID: r.ID, Name: "Правило " + r.Name}, drafts)
	}
	return qerr
}

func draft(r model.Rule, key string, st *model.RuleSeries, v float64, status model.EventStatus) pipeline.Draft {
	labels := map[string]string{"rule": r.ID, "source": "rule"}
	for k, x := range st.Labels {
		if k != "__name__" {
			labels[k] = x
		}
	}
	if r.ServiceLabel != "" && st.Labels[r.ServiceLabel] != "" {
		labels["service"] = st.Labels[r.ServiceLabel]
	}
	if r.Team != "" {
		labels["team"] = r.Team
	}
	fired := ""
	if st.FiredAt != nil {
		fired = strconv.FormatInt(st.FiredAt.Unix(), 10)
	}
	return pipeline.Draft{
		Title:      title(r, st.CI, v, st.Labels),
		CI:         st.CI,
		Signal:     r.Signal,
		Method:     r.Method,
		Severity:   r.Severity,
		Status:     status,
		ExternalID: r.ID + "|" + key + "|" + fired,
		Value:      FormatValue(v),
		Labels:     labels,
		Raw:        fmt.Sprintf(`{"rule":%q,"query":%q,"value":%s,"op":%q,"threshold":%s,"labels":%q}`, r.ID, r.Query, FormatValue(v), r.Op, FormatValue(r.Threshold), key),
	}
}

func (e *Engine) Release(r model.Rule) {
	var drafts []pipeline.Draft
	for key, st := range r.State {
		if st.Firing {
			drafts = append(drafts, draft(r, key, st, st.Value, model.EventResolved))
		}
	}
	if len(drafts) > 0 {
		e.in.Ingest(alert.Source{ID: r.ID, Name: "Правило " + r.Name}, drafts)
	}
}

func Templates() []model.Rule {
	t := func(method model.Method, signal, name, query, label, op string, thr float64, hold string, sev model.Severity, title string) model.Rule {
		return model.Rule{Method: method, Signal: signal, Name: name, Query: query, CILabel: label, Op: op, Threshold: thr, For: hold,
			Interval: "30s", Severity: sev, Title: title, Enabled: true}
	}
	return []model.Rule{
		t(model.MethodUSE, "use.cpu.utilization", "Загрузка CPU хоста", `100 * (1 - avg by (host, instance) (rate(node_cpu_seconds_total{mode="idle"}[2m])))`, "host", ">", 90, "5m", model.SevWarning, "CPU ${ci}: ${value}% (порог ${threshold}%)"),
		t(model.MethodUSE, "use.mem.utilization", "Память хоста", `100 * (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)`, "host", ">", 90, "5m", model.SevWarning, "Память ${ci}: ${value}% занято"),
		t(model.MethodUSE, "use.disk.utilization", "Заполнение диска", `100 * (1 - node_filesystem_avail_bytes{fstype!~"tmpfs|overlay|squashfs"} / node_filesystem_size_bytes{fstype!~"tmpfs|overlay|squashfs"})`, "host", ">", 85, "5m", model.SevError, "Диск ${labels.mountpoint} на ${ci}: ${value}%"),
		t(model.MethodUSE, "use.saturation", "Насыщение CPU (load на ядро)", `node_load5 / on (host, instance) count by (host, instance) (node_cpu_seconds_total{mode="idle"})`, "host", ">", 2, "10m", model.SevError, "Load на ядро ${ci}: ${value}"),
		t(model.MethodUSE, "use.errors", "Сетевые ошибки", `sum by (host, instance) (increase(node_network_receive_errs_total[5m]) + increase(node_network_transmit_errs_total[5m]))`, "host", ">", 0, "0s", model.SevWarning, "Сетевые ошибки на ${ci}: ${value} за 5 мин"),
		t(model.MethodUSE, "use.container.cpu", "CPU контейнера", `100 * sum by (name) (rate(container_cpu_usage_seconds_total{name!=""}[2m]))`, "name", ">", 80, "5m", model.SevWarning, "CPU контейнера ${ci}: ${value}%"),
		t(model.MethodUSE, "use.container.memory", "Память контейнера, МБ", `container_memory_working_set_bytes{name!=""} / 1048576`, "name", ">", 1024, "5m", model.SevWarning, "Память контейнера ${ci}: ${value} МБ"),
		t(model.MethodRED, "red.rate", "Нет запросов", `sum by (job) (rate(http_requests_total[5m]))`, "job", "<", 0.1, "10m", model.SevError, "Поток запросов ${ci}: ${value}/с"),
		t(model.MethodRED, "red.errors", "Доля ошибок 5xx", `100 * sum by (job) (rate(http_requests_total{code=~"5.."}[5m])) / sum by (job) (rate(http_requests_total[5m]))`, "job", ">", 5, "5m", model.SevCritical, "Ошибки 5xx ${ci}: ${value}%"),
		t(model.MethodRED, "red.duration", "Задержка p99", `histogram_quantile(0.99, sum by (job, le) (rate(http_request_duration_seconds_bucket[5m])))`, "job", ">", 1, "5m", model.SevError, "p99 ${ci}: ${value} с"),
	}
}
