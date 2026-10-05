package integration

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

type remote struct {
	base    string
	auth    string
	user    string
	secret  string
	client  *http.Client
	headers map[string]string
	scheme  string
}

func newRemote(it model.Integration, secret string) *remote {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if it.TLSSkipVerify {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	scheme := "Bearer"
	if it.Type == TypeNetBox && it.AuthType == AuthToken {
		scheme = "Token"
	}
	return &remote{base: strings.TrimRight(it.URL, "/"), auth: it.AuthType, user: it.Username, secret: secret,
		client: &http.Client{Timeout: 30 * time.Second, Transport: tr}, headers: map[string]string{}, scheme: scheme}
}

type httpError struct {
	Status int
	Body   string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("ответ %d: %s", e.Status, truncate(e.Body, 300))
}

func (r *remote) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}
	if r.secret != "" {
		switch r.auth {
		case AuthBasic:
			req.SetBasicAuth(r.user, r.secret)
		case AuthToken, AuthBearer:
			req.Header.Set("Authorization", r.scheme+" "+r.secret)
		case AuthAPIKey:
			req.Header.Set("Authorization", "ApiKey "+r.secret)
		}
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("нет соединения с %s: %w", r.base, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode/100 != 2 {
		return &httpError{Status: resp.StatusCode, Body: strings.TrimSpace(string(raw))}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("неверный ответ: %w", err)
		}
	}
	return nil
}

type zabbix struct {
	r     *remote
	token string
	login bool
}

func newZabbix(it model.Integration, secret string) *zabbix {
	r := newRemote(it, "")
	return &zabbix{r: r, token: secret, login: it.AuthType == AuthBasic}
}

func (z *zabbix) call(ctx context.Context, method string, params any, auth bool, out any) error {
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
			Data    string `json:"data"`
		} `json:"error"`
	}
	z.r.headers = map[string]string{}
	if auth && z.token != "" {
		z.r.headers["Authorization"] = "Bearer " + z.token
	}
	body := map[string]any{"jsonrpc": "2.0", "method": method, "params": params, "id": 1}
	if err := z.r.do(ctx, http.MethodPost, "/api_jsonrpc.php", body, &resp); err != nil {
		return fmt.Errorf("Zabbix %s: %w", method, err)
	}
	if resp.Error != nil {
		return fmt.Errorf("Zabbix %s: %s %s", method, resp.Error.Message, resp.Error.Data)
	}
	if out != nil {
		return json.Unmarshal(resp.Result, out)
	}
	return nil
}

func (z *zabbix) open(ctx context.Context, user, password string) error {
	if !z.login {
		return nil
	}
	var tok string
	if err := z.call(ctx, "user.login", map[string]string{"username": user, "password": password}, false, &tok); err != nil {
		return err
	}
	z.token = tok
	return nil
}

func (z *zabbix) close(ctx context.Context) {
	if z.login && z.token != "" {
		_ = z.call(ctx, "user.logout", []any{}, true, nil)
	}
}

func (z *zabbix) version(ctx context.Context) (string, error) {
	var v string
	err := z.call(ctx, "apiinfo.version", []any{}, false, &v)
	return v, err
}

const zabbixScript = `var p = JSON.parse(value);
var req = new HttpRequest();
req.addHeader('Content-Type: application/json');
req.addHeader('X-Umbrella-Token: ' + p.token);
var body = {
  event_id: p.event_id,
  event_value: p.event_value,
  status: p.event_value === '0' ? 'resolved' : 'firing',
  host: p.host,
  host_name: p.host_name,
  trigger_id: p.trigger_id,
  trigger_name: p.trigger_name,
  severity: p.severity,
  value: p.value,
  tags: p.tags,
  url: p.zabbix_url
};
var resp = req.post(p.url, JSON.stringify(body));
var code = req.getStatus();
Zabbix.log(4, '[Umbrella] ' + code + ' ' + resp);
if (code < 200 || code >= 300) {
  throw 'Umbrella answered ' + code + ': ' + resp;
}
return 'OK';`

func (z *zabbix) setup(ctx context.Context, it model.Integration, ingestURL, token string) (map[string]string, string, error) {
	spec, _ := Spec(TypeZabbix)
	p := func(k string) string { return spec.param(it.Params, k) }
	params := []map[string]string{
		{"name": "url", "value": ingestURL}, {"name": "token", "value": token},
		{"name": "event_id", "value": "{EVENT.ID}"}, {"name": "event_value", "value": "{EVENT.VALUE}"},
		{"name": "host", "value": "{HOST.HOST}"}, {"name": "host_name", "value": "{HOST.NAME}"},
		{"name": "trigger_id", "value": "{TRIGGER.ID}"}, {"name": "trigger_name", "value": "{EVENT.NAME}"},
		{"name": "severity", "value": "{EVENT.SEVERITY}"}, {"name": "value", "value": "{EVENT.OPDATA}"},
		{"name": "tags", "value": "{EVENT.TAGS}"},
		{"name": "zabbix_url", "value": strings.TrimRight(it.URL, "/") + "/zabbix.php?action=problem.view&triggerids%5B%5D={TRIGGER.ID}"},
	}
	mt := map[string]any{
		"name": p("media_type"), "type": 4, "status": 0, "script": zabbixScript, "parameters": params, "timeout": "10s",
		"maxattempts": 3, "attempt_interval": "10s", "process_tags": 0,
		"description": "Umbrella: проблемы и восстановления. Ответ не 2xx — Zabbix повторяет отправку.",
		"message_templates": []map[string]any{
			{"eventsource": 0, "recovery": 0, "subject": "Problem: {EVENT.NAME}", "message": "{EVENT.NAME} on {HOST.NAME}"},
			{"eventsource": 0, "recovery": 1, "subject": "Resolved: {EVENT.NAME}", "message": "{EVENT.NAME} on {HOST.NAME} resolved"},
			{"eventsource": 0, "recovery": 2, "subject": "Updated: {EVENT.NAME}", "message": "{EVENT.UPDATE.MESSAGE}"},
		},
	}
	var found []struct {
		ID string `json:"mediatypeid"`
	}
	if err := z.call(ctx, "mediatype.get", map[string]any{"filter": map[string]string{"name": p("media_type")}, "output": []string{"mediatypeid"}}, true, &found); err != nil {
		return nil, "", err
	}
	var mtID string
	if len(found) > 0 {
		mtID = found[0].ID
		mt["mediatypeid"] = mtID
		if err := z.call(ctx, "mediatype.update", mt, true, nil); err != nil {
			return nil, "", err
		}
	} else {
		var created struct {
			IDs []string `json:"mediatypeids"`
		}
		if err := z.call(ctx, "mediatype.create", mt, true, &created); err != nil {
			return nil, "", err
		}
		if len(created.IDs) == 0 {
			return nil, "", fmt.Errorf("Zabbix не вернул id типа оповещения")
		}
		mtID = created.IDs[0]
	}

	var users []struct {
		ID     string           `json:"userid"`
		Medias []map[string]any `json:"medias"`
	}
	if err := z.call(ctx, "user.get", map[string]any{"filter": map[string]string{"username": p("zabbix_user")},
		"output": []string{"userid"}, "selectMedias": []string{"mediatypeid", "sendto", "active", "severity", "period"}}, true, &users); err != nil {
		return nil, "", err
	}
	if len(users) == 0 {
		return nil, "", fmt.Errorf("пользователь Zabbix %q не найден", p("zabbix_user"))
	}
	uid := users[0].ID
	medias := []map[string]any{}
	has := false
	for _, m := range users[0].Medias {
		if fmt.Sprint(m["mediatypeid"]) == mtID {
			has = true
		}
		medias = append(medias, map[string]any{"mediatypeid": m["mediatypeid"], "sendto": m["sendto"], "active": m["active"],
			"severity": m["severity"], "period": m["period"]})
	}
	if !has {
		medias = append(medias, map[string]any{"mediatypeid": mtID, "sendto": "umbrella", "active": 0, "severity": 63, "period": "1-7,00:00-24:00"})
		if err := z.call(ctx, "user.update", map[string]any{"userid": uid, "medias": medias}, true, nil); err != nil {
			return nil, "", err
		}
	}

	action := map[string]any{
		"operations":          []map[string]any{{"operationtype": 0, "opmessage": map[string]any{"default_msg": 1, "mediatypeid": mtID}, "opmessage_usr": []map[string]string{{"userid": uid}}}},
		"recovery_operations": []map[string]any{{"operationtype": 11, "opmessage": map[string]any{"default_msg": 1}}},
	}
	var acts []struct {
		ID string `json:"actionid"`
	}
	if err := z.call(ctx, "action.get", map[string]any{"filter": map[string]string{"name": p("action")}, "output": []string{"actionid"}}, true, &acts); err != nil {
		return nil, "", err
	}
	var actID string
	if len(acts) > 0 {
		actID = acts[0].ID
		action["actionid"] = actID
		action["status"] = 0
		if err := z.call(ctx, "action.update", action, true, nil); err != nil {
			return nil, "", err
		}
	} else {
		action["name"], action["eventsource"], action["status"], action["esc_period"] = p("action"), 0, 0, "1h"
		var created struct {
			IDs []string `json:"actionids"`
		}
		if err := z.call(ctx, "action.create", action, true, &created); err != nil {
			return nil, "", err
		}
		if len(created.IDs) > 0 {
			actID = created.IDs[0]
		}
	}
	info := fmt.Sprintf("Zabbix: тип оповещения «%s» (%s), медиа пользователя %s, действие «%s» (%s); адрес приёма %s",
		p("media_type"), mtID, p("zabbix_user"), p("action"), actID, ingestURL)
	return map[string]string{"mediatype_id": mtID, "action_id": actID, "user_id": uid}, info, nil
}

var zabbixSeverityName = []string{"Not classified", "Information", "Warning", "Average", "High", "Disaster"}

const maxBackfill = 1000

func (z *zabbix) openProblems(ctx context.Context, it model.Integration) ([]string, error) {
	var problems []struct {
		EventID  string `json:"eventid"`
		ObjectID string `json:"objectid"`
		Name     string `json:"name"`
		Severity string `json:"severity"`
		OpData   string `json:"opdata"`
		Tags     []struct {
			Tag   string `json:"tag"`
			Value string `json:"value"`
		} `json:"tags"`
	}
	if err := z.call(ctx, "problem.get", map[string]any{"source": 0, "object": 0, "recent": false, "suppressed": false,
		"output": []string{"eventid", "objectid", "name", "severity", "opdata"}, "selectTags": "extend",
		"sortfield": []string{"eventid"}, "sortorder": "DESC", "limit": maxBackfill}, true, &problems); err != nil {
		return nil, err
	}
	if len(problems) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(problems))
	for _, p := range problems {
		ids = append(ids, p.ObjectID)
	}
	var triggers []struct {
		ID    string `json:"triggerid"`
		Hosts []struct {
			Host string `json:"host"`
			Name string `json:"name"`
		} `json:"hosts"`
	}
	if err := z.call(ctx, "trigger.get", map[string]any{"triggerids": ids, "output": []string{"triggerid"},
		"selectHosts": []string{"host", "name"}}, true, &triggers); err != nil {
		return nil, err
	}
	hosts := map[string][2]string{}
	for _, t := range triggers {
		if len(t.Hosts) > 0 {
			hosts[t.ID] = [2]string{t.Hosts[0].Host, t.Hosts[0].Name}
		}
	}
	base := strings.TrimRight(it.URL, "/")
	out := make([]string, 0, len(problems))
	for i := len(problems) - 1; i >= 0; i-- {
		p := problems[i]
		h, ok := hosts[p.ObjectID]
		if !ok {
			continue
		}
		sev := zabbixSeverityName[0]
		if n, err := strconv.Atoi(p.Severity); err == nil && n >= 0 && n < len(zabbixSeverityName) {
			sev = zabbixSeverityName[n]
		}
		tags := make([]string, 0, len(p.Tags))
		for _, t := range p.Tags {
			tags = append(tags, t.Tag+":"+t.Value)
		}
		b, err := json.Marshal(map[string]string{"event_id": p.EventID, "event_value": "1", "status": "firing",
			"host": h[0], "host_name": h[1], "trigger_id": p.ObjectID, "trigger_name": p.Name, "severity": sev,
			"value": p.OpData, "tags": strings.Join(tags, ", "),
			"url": base + "/zabbix.php?action=problem.view&triggerids%5B%5D=" + p.ObjectID})
		if err != nil {
			return nil, err
		}
		out = append(out, string(b))
	}
	return out, nil
}

func (z *zabbix) teardown(ctx context.Context, remote map[string]string) error {
	if id := remote["action_id"]; id != "" {
		if err := z.call(ctx, "action.delete", []string{id}, true, nil); err != nil {
			return err
		}
	}
	if id := remote["mediatype_id"]; id != "" {
		if err := z.call(ctx, "mediatype.delete", []string{id}, true, nil); err != nil {
			return err
		}
	}
	return nil
}

type contactPoint struct {
	UID                   string         `json:"uid,omitempty"`
	Name                  string         `json:"name"`
	Type                  string         `json:"type"`
	Settings              map[string]any `json:"settings"`
	DisableResolveMessage bool           `json:"disableResolveMessage"`
}

func grafanaSetup(ctx context.Context, r *remote, it model.Integration, ingestURL, token string) (map[string]string, string, error) {
	spec, _ := Spec(TypeGrafana)
	name := spec.param(it.Params, "contact_point")
	r.headers = map[string]string{"X-Disable-Provenance": "true"}
	cp := contactPoint{Name: name, Type: "webhook", Settings: map[string]any{
		"url": ingestURL, "httpMethod": "POST", "authorization_scheme": "Bearer", "authorization_credentials": token,
	}}
	var existing []contactPoint
	if err := r.do(ctx, http.MethodGet, "/api/v1/provisioning/contact-points?name="+urlQuery(name), nil, &existing); err != nil {
		return nil, "", fmt.Errorf("Grafana: точки контакта: %w", err)
	}
	if len(existing) > 0 {
		cp.UID = existing[0].UID
		if err := r.do(ctx, http.MethodPut, "/api/v1/provisioning/contact-points/"+cp.UID, cp, nil); err != nil {
			return nil, "", fmt.Errorf("Grafana: обновление точки контакта: %w", err)
		}
	} else {
		var created contactPoint
		if err := r.do(ctx, http.MethodPost, "/api/v1/provisioning/contact-points", cp, &created); err != nil {
			return nil, "", fmt.Errorf("Grafana: создание точки контакта: %w", err)
		}
		cp.UID = created.UID
	}
	info := fmt.Sprintf("Grafana: точка контакта «%s» → %s", name, ingestURL)
	if spec.param(it.Params, "route_all") == "true" {
		if err := grafanaRoute(ctx, r, name, true); err != nil {
			return nil, "", err
		}
		info += "; маршрут для всех алертов (continue)"
	}
	return map[string]string{"contact_point_uid": cp.UID, "contact_point": name}, info, nil
}

func grafanaRoute(ctx context.Context, r *remote, receiver string, add bool) error {
	var tree map[string]any
	if err := r.do(ctx, http.MethodGet, "/api/v1/provisioning/policies", nil, &tree); err != nil {
		return fmt.Errorf("Grafana: политика уведомлений: %w", err)
	}
	routes, _ := tree["routes"].([]any)
	kept := []any{}
	for _, x := range routes {
		if m, ok := x.(map[string]any); ok && m["receiver"] == receiver && m["continue"] == true && m["object_matchers"] == nil {
			continue
		}
		kept = append(kept, x)
	}
	if add {
		kept = append([]any{map[string]any{"receiver": receiver, "continue": true}}, kept...)
	}
	tree["routes"] = kept
	if err := r.do(ctx, http.MethodPut, "/api/v1/provisioning/policies", tree, nil); err != nil {
		return fmt.Errorf("Grafana: обновление политики уведомлений: %w", err)
	}
	return nil
}

func grafanaTeardown(ctx context.Context, r *remote, remote map[string]string) error {
	r.headers = map[string]string{"X-Disable-Provenance": "true"}
	if name := remote["contact_point"]; name != "" {
		if err := grafanaRoute(ctx, r, name, false); err != nil {
			return err
		}
	}
	if uid := remote["contact_point_uid"]; uid != "" {
		err := r.do(ctx, http.MethodDelete, "/api/v1/provisioning/contact-points/"+uid, nil, nil)
		if he, ok := err.(*httpError); ok && he.Status == http.StatusNotFound {
			return nil
		}
		return err
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
