package pipeline

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

type BlockSpec struct {
	Kind        string      `json:"kind"`
	Category    string      `json:"category"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Fields      []FieldSpec `json:"fields"`
}

type FieldSpec struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Options     []string `json:"options,omitempty"`
	Default     string   `json:"default,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Help        string   `json:"help,omitempty"`
}

var Blocks = []BlockSpec{
	{Kind: "trigger.webhook", Category: "trigger", Title: "Входящий webhook",
		Description: "Источник отправляет события на /api/ingest/{id}. Ответ 2xx уходит только после записи события.",
		Fields: []FieldSpec{
			{Key: "auth", Label: "Проверка", Type: "select", Options: []string{"token", "none"}, Default: "token"},
			{Key: "secret_ref", Label: "Токен источника", Type: "secret", Placeholder: "openbao://umbrella/connectors/<id>#token", Help: "Источник передаёт его в X-Umbrella-Token или Authorization: Bearer; значение хранится только в OpenBao"},
		}},
	{Kind: "trigger.schedule", Category: "trigger", Title: "Расписание",
		Description: "Запускает получение данных с заданным интервалом (pull).",
		Fields: []FieldSpec{
			{Key: "interval", Label: "Интервал", Type: "text", Default: "60s", Placeholder: "30s, 1m, 5m"},
		}},
	{Kind: "fetch.http", Category: "fetch", Title: "HTTP-запрос",
		Description: "GET или POST к REST API источника. Логины, пароли и токены берутся из OpenBao по ссылке.",
		Fields: []FieldSpec{
			{Key: "url", Label: "URL", Type: "text", Placeholder: "https://monitoring.example/api/problems"},
			{Key: "method", Label: "Метод", Type: "select", Options: []string{"GET", "POST"}, Default: "GET"},
			{Key: "body", Label: "Тело запроса", Type: "textarea"},
			{Key: "auth", Label: "Авторизация", Type: "select", Options: []string{"none", "bearer", "basic", "apikey", "header"}, Default: "none"},
			{Key: "username", Label: "Логин (Basic)", Type: "text"},
			{Key: "secret_ref", Label: "Секрет", Type: "secret", Placeholder: "openbao://umbrella/connectors/<id>#token", Help: "Пароль или токен хранится только в OpenBao"},
			{Key: "auth_header", Label: "Заголовок (режим header)", Type: "text", Placeholder: "Authorization"},
			{Key: "headers", Label: "Заголовки", Type: "textarea", Placeholder: "Accept: application/json"},
			{Key: "tls_skip_verify", Label: "Не проверять TLS", Type: "select", Options: []string{"false", "true"}, Default: "false"},
		}},
	{Kind: "parse.json", Category: "parse", Title: "Парсинг JSON",
		Description: "Разбирает JSON. Путь к массиву разбивает пачку на отдельные события.",
		Fields: []FieldSpec{
			{Key: "items", Label: "Путь к массиву событий", Type: "text", Placeholder: "data.alerts", Help: "Пусто: тело целиком — одно событие или массив"},
		}},
	{Kind: "parse.kv", Category: "parse", Title: "Парсинг key=value",
		Description: "Строки вида host=db1 sev=5 msg=\"disk full\". Каждая строка — событие.",
		Fields: []FieldSpec{
			{Key: "pair_sep", Label: "Разделитель пар", Type: "text", Default: " "},
			{Key: "kv_sep", Label: "Разделитель ключа", Type: "text", Default: "="},
		}},
	{Kind: "parse.regex", Category: "parse", Title: "Парсинг regex",
		Description: "Именованные группы (?P<name>...) становятся полями. Каждая строка — событие.",
		Fields: []FieldSpec{
			{Key: "pattern", Label: "Выражение", Type: "textarea", Placeholder: `(?P<host>\S+) (?P<level>\w+): (?P<msg>.*)`},
		}},
	{Kind: "parse.csv", Category: "parse", Title: "Парсинг CSV",
		Description: "Первая строка — заголовок, остальные — события.",
		Fields: []FieldSpec{
			{Key: "delimiter", Label: "Разделитель", Type: "text", Default: ","},
		}},
	{Kind: "filter", Category: "transform", Title: "Фильтр",
		Description: "Пропускает дальше только события, подходящие под условие.",
		Fields: []FieldSpec{
			{Key: "field", Label: "Поле", Type: "text", Placeholder: "status"},
			{Key: "op", Label: "Условие", Type: "select", Options: []string{"eq", "ne", "contains", "exists", "not_exists"}, Default: "eq"},
			{Key: "value", Label: "Значение", Type: "text"},
		}},
	{Kind: "map.severity", Category: "transform", Title: "Справочник severity",
		Description: "Переводит значение поля источника в critical, error, warning или info.",
		Fields: []FieldSpec{
			{Key: "field", Label: "Поле источника", Type: "text", Placeholder: "priority"},
			{Key: "mapping", Label: "Соответствие", Type: "textarea", Default: "5=critical\n4=error\n3=warning\n*=info", Help: "по строке на значение; * — всё остальное"},
		}},
	{Kind: "enrich.labels", Category: "transform", Title: "Метки",
		Description: "Добавляет постоянные или вычисленные метки.",
		Fields: []FieldSpec{
			{Key: "labels", Label: "Метки", Type: "textarea", Placeholder: "env=prod\nregion=${region}"},
		}},
	{Kind: "map.event", Category: "transform", Title: "Шаблон источника",
		Description: "Сопоставляет поля источника с единой моделью события. ${путь} подставляет значение поля.",
		Fields: []FieldSpec{
			{Key: "title", Label: "Заголовок", Type: "text", Placeholder: "${msg}"},
			{Key: "ci", Label: "КЕ", Type: "text", Placeholder: "${host}", Help: "имя, хост, тег или облачный ID — КЕ найдёт CI Resolver"},
			{Key: "signal", Label: "Сигнал", Type: "text", Placeholder: "use.cpu.utilization"},
			{Key: "method", Label: "Метод", Type: "select", Options: []string{"red", "use", "other"}, Default: "other"},
			{Key: "severity", Label: "Severity", Type: "text", Placeholder: "${severity}"},
			{Key: "status", Label: "Статус", Type: "text", Placeholder: "${state}", Help: "resolved, ok, closed, recovery → resolved; иначе firing"},
			{Key: "external_id", Label: "ID в источнике", Type: "text", Placeholder: "${id}"},
			{Key: "value", Label: "Значение", Type: "text", Placeholder: "${value}"},
		}},
	{Kind: "ack.response", Category: "ack", Title: "Подтверждение получения",
		Description: "Push: ответ 2xx источнику только после записи событий. Pull: повторно полученные события отбрасываются по ID в источнике.",
		Fields: []FieldSpec{
			{Key: "mode", Label: "Способ", Type: "select", Options: []string{"http_2xx", "cursor"}, Default: "http_2xx"},
		}},
	{Kind: "out.event", Category: "output", Title: "Событие",
		Description: "Передаёт нормализованные события в обработку тревог.",
		Fields:      []FieldSpec{}},
}

func Spec(kind string) (BlockSpec, bool) {
	for _, b := range Blocks {
		if b.Kind == kind {
			return b, true
		}
	}
	return BlockSpec{}, false
}

type Record map[string]any

type Draft struct {
	Title      string            `json:"title"`
	CI         string            `json:"ci"`
	Signal     string            `json:"signal"`
	Method     model.Method      `json:"method"`
	Severity   model.Severity    `json:"severity"`
	Status     model.EventStatus `json:"status"`
	ExternalID string            `json:"external_id"`
	Value      string            `json:"value,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Raw        string            `json:"raw"`
}

type StepTrace struct {
	NodeID string `json:"node_id"`
	Kind   string `json:"kind"`
	In     int    `json:"in"`
	Out    int    `json:"out"`
	Sample any    `json:"sample,omitempty"`
	Error  string `json:"error,omitempty"`
	Millis int64  `json:"ms"`
}

type Result struct {
	Events []Draft      `json:"events"`
	Trace  []StepTrace  `json:"trace"`
	Errors []BlockError `json:"errors"`
}

type BlockError struct {
	NodeID string `json:"node_id"`
	Kind   string `json:"kind"`
	Error  string `json:"error"`
	Raw    string `json:"raw"`
}

func Validate(g model.Graph) ([]model.Node, error) {
	byID := map[string]model.Node{}
	for _, n := range g.Nodes {
		if _, ok := Spec(n.Kind); !ok {
			return nil, fmt.Errorf("неизвестный блок %q", n.Kind)
		}
		byID[n.ID] = n
	}
	var triggers []model.Node
	hasOut := false
	for _, n := range g.Nodes {
		if strings.HasPrefix(n.Kind, "trigger.") {
			triggers = append(triggers, n)
		}
		if n.Kind == "out.event" {
			hasOut = true
		}
	}
	if len(triggers) != 1 {
		return nil, errors.New("в коннекторе должен быть ровно один триггер")
	}
	if !hasOut {
		return nil, errors.New("нет блока «Событие»: коннектор ничего не передаст дальше")
	}
	next := map[string][]string{}
	for _, e := range g.Edges {
		if _, ok := byID[e.Source]; !ok {
			return nil, fmt.Errorf("связь %s ссылается на несуществующий блок", e.ID)
		}
		if _, ok := byID[e.Target]; !ok {
			return nil, fmt.Errorf("связь %s ссылается на несуществующий блок", e.ID)
		}
		next[e.Source] = append(next[e.Source], e.Target)
	}

	order := []model.Node{}
	queue := []string{triggers[0].ID}
	seen := map[string]bool{}
	reach := reachable(triggers[0].ID, next)
	deg := map[string]int{}
	for _, e := range g.Edges {
		if reach[e.Source] {
			deg[e.Target]++
		}
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		order = append(order, byID[id])
		targets := append([]string(nil), next[id]...)
		sort.Strings(targets)
		for _, t := range targets {
			deg[t]--
			if deg[t] == 0 {
				queue = append(queue, t)
			}
		}
	}
	for id := range reach {
		if !seen[id] {
			return nil, errors.New("в графе есть цикл")
		}
	}
	outReached := false
	for _, n := range order {
		if n.Kind == "out.event" {
			outReached = true
		}
	}
	if !outReached {
		return nil, errors.New("блок «Событие» не связан с триггером")
	}
	return order, nil
}

func reachable(from string, next map[string][]string) map[string]bool {
	r := map[string]bool{from: true}
	stack := []string{from}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, t := range next[id] {
			if !r[t] {
				r[t] = true
				stack = append(stack, t)
			}
		}
	}
	return r
}

func Run(g model.Graph, raw string) (Result, error) {
	order, err := Validate(g)
	if err != nil {
		return Result{}, err
	}
	parents := map[string][]string{}
	for _, e := range g.Edges {
		parents[e.Target] = append(parents[e.Target], e.Source)
	}
	outputs := map[string][]Record{}
	res := Result{Events: []Draft{}, Trace: []StepTrace{}, Errors: []BlockError{}}

	for _, n := range order {
		start := time.Now()
		var in []Record
		if strings.HasPrefix(n.Kind, "trigger.") {
			in = []Record{{"_raw": raw}}
		} else {
			for _, p := range parents[n.ID] {
				in = append(in, outputs[p]...)
			}
		}
		var out []Record
		var stepErr error
		if n.Disabled {
			out = in
		} else {
			out, stepErr = apply(n, in, &res)
		}
		tr := StepTrace{NodeID: n.ID, Kind: n.Kind, In: len(in), Out: len(out), Millis: time.Since(start).Milliseconds()}
		if stepErr != nil {
			tr.Error = stepErr.Error()
			res.Errors = append(res.Errors, BlockError{NodeID: n.ID, Kind: n.Kind, Error: stepErr.Error(), Raw: truncate(raw, 4000)})
		}
		if len(out) > 0 {
			tr.Sample = public(out[0])
		}
		res.Trace = append(res.Trace, tr)
		outputs[n.ID] = out
	}
	return res, nil
}

func apply(n model.Node, in []Record, res *Result) ([]Record, error) {
	c := n.Config
	if c == nil {
		c = map[string]string{}
	}
	switch n.Kind {
	case "trigger.webhook", "trigger.schedule", "fetch.http", "ack.response":

		return in, nil
	case "parse.json":
		return parseJSON(in, c["items"])
	case "parse.kv":
		return parseKV(in, def(c["pair_sep"], " "), def(c["kv_sep"], "="))
	case "parse.regex":
		return parseRegex(in, c["pattern"])
	case "parse.csv":
		return parseCSV(in, def(c["delimiter"], ","))
	case "filter":
		return filter(in, c["field"], def(c["op"], "eq"), c["value"]), nil
	case "map.severity":
		return mapSeverity(in, c["field"], c["mapping"]), nil
	case "enrich.labels":
		return enrichLabels(in, c["labels"]), nil
	case "map.event":
		return mapEvent(in, c), nil
	case "out.event":
		for _, r := range in {
			res.Events = append(res.Events, toDraft(r))
		}
		return in, nil
	}
	return nil, fmt.Errorf("блок %s не поддерживается", n.Kind)
}

func def(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func rawOf(r Record) string {
	s, _ := r["_raw"].(string)
	return s
}

func parseJSON(in []Record, itemsPath string) ([]Record, error) {
	var out []Record
	for _, r := range in {
		raw := rawOf(r)
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return out, fmt.Errorf("невалидный JSON: %v", err)
		}
		if itemsPath != "" {
			got, ok := Lookup(v, itemsPath)
			if !ok {
				return out, fmt.Errorf("в теле нет пути %q", itemsPath)
			}
			v = got
		}
		items, isArr := v.([]any)
		if !isArr {
			items = []any{v}
		}
		for _, it := range items {
			rec := Record{}
			if m, ok := it.(map[string]any); ok {
				for k, val := range m {
					rec[k] = val
				}
			} else {
				rec["value"] = it
			}
			b, _ := json.Marshal(it)
			rec["_raw"] = string(b)
			out = append(out, rec)
		}
	}
	return out, nil
}

func lines(s string) []string {
	var res []string
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			res = append(res, l)
		}
	}
	return res
}

func parseKV(in []Record, pairSep, kvSep string) ([]Record, error) {
	var out []Record
	for _, r := range in {
		for _, line := range lines(rawOf(r)) {
			rec := Record{"_raw": line}
			for _, pair := range splitQuoted(line, pairSep) {
				k, v, ok := strings.Cut(pair, kvSep)
				if !ok || k == "" {
					continue
				}
				rec[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"`)
			}
			if len(rec) == 1 {
				return out, fmt.Errorf("в строке нет пар ключ%sзначение: %s", kvSep, truncate(line, 120))
			}
			out = append(out, rec)
		}
	}
	return out, nil
}

func splitQuoted(s, sep string) []string {
	var parts []string
	var cur strings.Builder
	inQ := false
	for i := 0; i < len(s); {
		if s[i] == '"' {
			inQ = !inQ
			cur.WriteByte(s[i])
			i++
			continue
		}
		if !inQ && strings.HasPrefix(s[i:], sep) {
			parts = append(parts, cur.String())
			cur.Reset()
			i += len(sep)
			continue
		}
		cur.WriteByte(s[i])
		i++
	}
	parts = append(parts, cur.String())
	return parts
}

func parseRegex(in []Record, pattern string) ([]Record, error) {
	if pattern == "" {
		return nil, errors.New("не задано выражение")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("ошибка в выражении: %v", err)
	}
	var out []Record
	var miss []string
	for _, r := range in {
		for _, line := range lines(rawOf(r)) {
			m := re.FindStringSubmatch(line)
			if m == nil {
				miss = append(miss, line)
				continue
			}
			rec := Record{"_raw": line}
			for i, name := range re.SubexpNames() {
				if name != "" {
					rec[name] = m[i]
				}
			}
			out = append(out, rec)
		}
	}
	if len(miss) > 0 {
		return out, fmt.Errorf("%d строк не подошли под выражение, первая: %s", len(miss), truncate(miss[0], 120))
	}
	return out, nil
}

func parseCSV(in []Record, delim string) ([]Record, error) {
	var out []Record
	for _, r := range in {
		rd := csv.NewReader(strings.NewReader(rawOf(r)))
		rd.Comma = []rune(delim)[0]
		rd.FieldsPerRecord = -1
		rows, err := rd.ReadAll()
		if err != nil {
			return out, fmt.Errorf("ошибка CSV: %v", err)
		}
		if len(rows) < 2 {
			continue
		}
		head := rows[0]
		for _, row := range rows[1:] {
			rec := Record{"_raw": strings.Join(row, delim)}
			for i, h := range head {
				if i < len(row) {
					rec[strings.TrimSpace(h)] = row[i]
				}
			}
			out = append(out, rec)
		}
	}
	return out, nil
}

func filter(in []Record, field, op, value string) []Record {
	var out []Record
	for _, r := range in {
		v, ok := Lookup(map[string]any(r), field)
		s := Stringify(v)
		keep := false
		switch op {
		case "eq":
			keep = ok && s == value
		case "ne":
			keep = !ok || s != value
		case "contains":
			keep = ok && strings.Contains(strings.ToLower(s), strings.ToLower(value))
		case "exists":
			keep = ok && s != ""
		case "not_exists":
			keep = !ok || s == ""
		}
		if keep {
			out = append(out, r)
		}
	}
	return out
}

func ParseMapping(s string) map[string]string {
	m := map[string]string{}
	for _, l := range lines(s) {
		k, v, ok := strings.Cut(l, "=")
		if ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}

func mapSeverity(in []Record, field, mapping string) []Record {
	m := ParseMapping(mapping)
	for _, r := range in {
		v, _ := Lookup(map[string]any(r), field)
		s := strings.ToLower(Stringify(v))
		sev, ok := m[s]
		if !ok {
			for k, val := range m {
				if strings.EqualFold(k, s) {
					sev, ok = val, true
				}
			}
		}
		if !ok {
			sev = m["*"]
		}
		if sev != "" {
			r["_severity"] = sev
		}
	}
	return in
}

func enrichLabels(in []Record, spec string) []Record {
	m := ParseMapping(spec)
	for _, r := range in {
		labels, _ := r["_labels"].(map[string]string)
		if labels == nil {
			labels = map[string]string{}
		}
		for k, v := range m {
			labels[k] = Render(v, r)
		}
		r["_labels"] = labels
	}
	return in
}

func mapEvent(in []Record, c map[string]string) []Record {
	for _, r := range in {
		out := map[string]string{}
		for _, key := range []string{"title", "ci", "signal", "method", "severity", "status", "external_id", "value"} {
			if tpl, ok := c[key]; ok && tpl != "" {
				out[key] = Render(tpl, r)
			}
		}
		r["_event"] = out
	}
	return in
}

var placeholder = regexp.MustCompile(`\$\{([^}]+)\}`)

func Render(tpl string, r Record) string {
	return placeholder.ReplaceAllStringFunc(tpl, func(m string) string {
		parts := strings.Split(m[2:len(m)-1], "|")
		if v, ok := Lookup(map[string]any(r), strings.TrimSpace(parts[0])); ok && Stringify(v) != "" {
			return Stringify(v)
		}
		for _, alt := range parts[1:] {
			ref, isRef := strings.CutPrefix(alt, "$")
			if !isRef {
				return alt
			}
			if v, ok := Lookup(map[string]any(r), ref); ok && Stringify(v) != "" {
				return Stringify(v)
			}
		}
		return ""
	})
}

func Lookup(v any, path string) (any, bool) {
	if path == "" {
		return v, true
	}
	cur := v
	for _, part := range strings.Split(path, ".") {
		switch t := cur.(type) {
		case map[string]any:
			next, ok := t[part]
			if !ok {
				return nil, false
			}
			cur = next
		case Record:
			next, ok := t[part]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(t) {
				return nil, false
			}
			cur = t[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

func Stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

var resolvedWords = map[string]bool{"resolved": true, "ok": true, "closed": true, "recovery": true, "recovered": true, "normal": true, "inactive": true}

func NormalizeSeverity(s string) model.Severity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical", "crit", "fatal", "disaster", "emergency", "p1", "high":
		return model.SevCritical
	case "error", "err", "major", "average", "p2":
		return model.SevError
	case "warning", "warn", "minor", "p3", "medium":
		return model.SevWarning
	case "info", "information", "informational", "notice", "ok", "low", "p4", "p5":
		return model.SevInfo
	}
	return model.SevWarning
}

func toDraft(r Record) Draft {
	ev, _ := r["_event"].(map[string]string)
	if ev == nil {
		ev = map[string]string{}
	}
	d := Draft{
		Title:      ev["title"],
		CI:         ev["ci"],
		Signal:     ev["signal"],
		Method:     model.Method(strings.ToLower(ev["method"])),
		ExternalID: ev["external_id"],
		Value:      ev["value"],
		Raw:        rawOf(r),
	}
	sev := ev["severity"]
	if s, ok := r["_severity"].(string); ok && s != "" {
		sev = s
	}
	d.Severity = NormalizeSeverity(sev)
	if resolvedWords[strings.ToLower(strings.TrimSpace(ev["status"]))] {
		d.Status = model.EventResolved
	} else {
		d.Status = model.EventFiring
	}
	if d.Method != model.MethodRED && d.Method != model.MethodUSE {
		d.Method = model.MethodOther
	}
	if d.Signal == "" {
		d.Signal = "generic"
	}
	if d.Title == "" {
		d.Title = d.Signal + " на " + d.CI
	}
	if labels, ok := r["_labels"].(map[string]string); ok {
		d.Labels = labels
	}
	return d
}

func public(r Record) map[string]any {
	out := map[string]any{}
	for k, v := range r {
		if k == "_raw" {
			continue
		}
		out[k] = v
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
