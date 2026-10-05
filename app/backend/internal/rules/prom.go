package rules

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

// Auth is the credential a metric source is queried with.
type Auth struct {
	Type    string
	Fields  map[string]string
	Secrets map[string]string
}

// Sample is one series of an instant vector.
type Sample struct {
	Labels map[string]string `json:"labels"`
	Value  float64           `json:"value"`
}

const maxSamples = 10000

var clients = map[bool]*http.Client{
	false: {Timeout: 20 * time.Second},
	true:  {Timeout: 20 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}, //nolint:gosec // the administrator's choice
}

// Query runs an instant query against the Prometheus HTTP API of a source.
func Query(ctx context.Context, src model.MetricSource, auth *Auth, promql string) ([]Sample, error) {
	base, err := url.Parse(strings.TrimRight(src.URL, "/"))
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") {
		return nil, errors.New("the source address is not an http or https URL")
	}
	form := url.Values{"query": {promql}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String()+"/api/v1/query", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if auth != nil {
		switch auth.Type {
		case "bearer":
			req.Header.Set("Authorization", "Bearer "+auth.Secrets["token"])
		case "basic":
			req.SetBasicAuth(auth.Fields["username"], auth.Secrets["password"])
		case "header":
			if h := auth.Fields["header"]; h != "" {
				req.Header.Set(h, auth.Secrets["value"])
			}
		default:
			return nil, fmt.Errorf("a %s credential cannot be used to query metrics", auth.Type)
		}
	}
	resp, err := clients[src.SkipVerify].Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	var out struct {
		Status    string `json:"status"`
		ErrorType string `json:"errorType"`
		Error     string `json:"error"`
		Data      struct {
			ResultType string          `json:"resultType"`
			Result     json.RawMessage `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("the source answered %d", resp.StatusCode)
		}
		return nil, errors.New("the source did not answer with the Prometheus API")
	}
	if out.Status != "success" {
		msg := out.Error
		if msg == "" {
			msg = fmt.Sprintf("the source answered %d", resp.StatusCode)
		}
		return nil, errors.New(msg)
	}
	switch out.Data.ResultType {
	case "vector":
		var vec []struct {
			Metric map[string]string `json:"metric"`
			Value  [2]any            `json:"value"`
		}
		if err := json.Unmarshal(out.Data.Result, &vec); err != nil {
			return nil, err
		}
		if len(vec) > maxSamples {
			return nil, fmt.Errorf("the query returned %d series; narrow it to at most %d", len(vec), maxSamples)
		}
		samples := make([]Sample, 0, len(vec))
		for _, v := range vec {
			f, ok := number(v.Value[1])
			if !ok {
				continue
			}
			if v.Metric == nil {
				v.Metric = map[string]string{}
			}
			samples = append(samples, Sample{Labels: v.Metric, Value: f})
		}
		return samples, nil
	case "scalar":
		var sc [2]any
		if err := json.Unmarshal(out.Data.Result, &sc); err != nil {
			return nil, err
		}
		if f, ok := number(sc[1]); ok {
			return []Sample{{Labels: map[string]string{}, Value: f}}, nil
		}
		return nil, nil
	}
	return nil, fmt.Errorf("a %s result cannot be evaluated: use an instant vector", out.Data.ResultType)
}

func number(v any) (float64, bool) {
	s, ok := v.(string)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}
