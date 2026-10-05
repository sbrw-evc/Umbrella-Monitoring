package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type MetricSample struct {
	Labels map[string]string `json:"labels"`
	Value  float64           `json:"value"`
}

func (m *Manager) Query(ctx context.Context, sourceID, promql string) ([]MetricSample, error) {
	var it model.Integration
	ok := false
	m.st.Read(func(d *store.Data) {
		if p := d.Integrations[sourceID]; p != nil {
			it, ok = clone(*p), true
		}
	})
	if !ok {
		return nil, fmt.Errorf("источник метрик %s не найден", sourceID)
	}
	if it.Type != TypePrometheus {
		return nil, fmt.Errorf("интеграция %s не источник метрик", sourceID)
	}
	secret, err := m.secret(it)
	if err != nil {
		return nil, err
	}
	return promQuery(ctx, newRemote(it, secret), promql)
}

func promQuery(ctx context.Context, r *remote, promql string) ([]MetricSample, error) {
	var resp struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     any    `json:"result"`
		} `json:"data"`
	}
	err := r.do(ctx, http.MethodGet, "/api/v1/query?query="+url.QueryEscape(promql), nil, &resp)
	var he *httpError
	if errors.As(err, &he) && he.Status == http.StatusBadRequest {
		return nil, fmt.Errorf("ошибка запроса PromQL: %s", truncate(he.Body, 300))
	}
	if err != nil {
		return nil, err
	}
	if resp.Status != "success" {
		return nil, fmt.Errorf("Prometheus: %s", resp.Error)
	}
	parse := func(v any) (float64, bool) {
		pair, ok := v.([]any)
		if !ok || len(pair) != 2 {
			return 0, false
		}
		s, _ := pair[1].(string)
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}
	var out []MetricSample
	switch resp.Data.ResultType {
	case "vector":
		list, _ := resp.Data.Result.([]any)
		for _, x := range list {
			m, _ := x.(map[string]any)
			f, ok := parse(m["value"])
			if !ok {
				continue
			}
			labels := map[string]string{}
			if lm, ok := m["metric"].(map[string]any); ok {
				for k, v := range lm {
					labels[k] = fmt.Sprint(v)
				}
			}
			out = append(out, MetricSample{Labels: labels, Value: f})
		}
	case "scalar":
		if f, ok := parse(resp.Data.Result); ok {
			out = append(out, MetricSample{Labels: map[string]string{}, Value: f})
		}
	default:
		return nil, fmt.Errorf("запрос вернул %s: нужен instant vector или scalar", resp.Data.ResultType)
	}
	return out, nil
}
