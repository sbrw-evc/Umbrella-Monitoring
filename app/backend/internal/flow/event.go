package flow

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/textx"
)

const (
	SeverityCritical = "critical"
	SeverityError    = "error"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"

	StatusFiring   = "firing"
	StatusResolved = "resolved"

	MethodRED   = "red"
	MethodUSE   = "use"
	MethodOther = "other"

	maxEventField = 1000
	maxLabels     = 64
)

var Severities = []string{SeverityCritical, SeverityError, SeverityWarning, SeverityInfo}

// Event is the normalized output of a connector: the fixed target schema of map.event.
type Event struct {
	Title      string            `json:"title"`
	CI         string            `json:"ci"`
	Signal     string            `json:"signal"`
	Method     string            `json:"method"`
	Severity   string            `json:"severity"`
	Status     string            `json:"status"`
	ExternalID string            `json:"external_id"`
	Value      string            `json:"value"`
	Labels     map[string]string `json:"labels"`
	// Key identifies a delivery of the same alert: repeated deliveries update one event.
	Key string `json:"key"`
}

var severityWords = map[string]string{
	"critical": SeverityCritical, "crit": SeverityCritical, "disaster": SeverityCritical, "fatal": SeverityCritical,
	"emergency": SeverityCritical, "p1": SeverityCritical, "sev1": SeverityCritical,
	"error": SeverityError, "err": SeverityError, "high": SeverityError, "major": SeverityError, "p2": SeverityError, "sev2": SeverityError,
	"warning": SeverityWarning, "warn": SeverityWarning, "average": SeverityWarning, "medium": SeverityWarning, "minor": SeverityWarning,
	"p3": SeverityWarning, "sev3": SeverityWarning,
	"info": SeverityInfo, "information": SeverityInfo, "informational": SeverityInfo, "notice": SeverityInfo, "low": SeverityInfo,
	"not classified": SeverityInfo, "none": SeverityInfo, "debug": SeverityInfo, "p4": SeverityInfo, "p5": SeverityInfo, "sev4": SeverityInfo,
}

func NormalizeSeverity(v string) (string, bool) {
	s, ok := severityWords[strings.ToLower(strings.TrimSpace(v))]
	return s, ok
}

var statusWords = map[string]string{
	"": StatusFiring, "firing": StatusFiring, "fire": StatusFiring, "problem": StatusFiring, "alerting": StatusFiring, "alert": StatusFiring,
	"active": StatusFiring, "triggered": StatusFiring, "trigger": StatusFiring, "open": StatusFiring, "critical": StatusFiring, "1": StatusFiring,
	"resolved": StatusResolved, "resolve": StatusResolved, "ok": StatusResolved, "closed": StatusResolved, "recovery": StatusResolved,
	"recovered": StatusResolved, "inactive": StatusResolved, "normal": StatusResolved, "cleared": StatusResolved, "0": StatusResolved,
}

func NormalizeStatus(v string) (string, bool) {
	s, ok := statusWords[strings.ToLower(strings.TrimSpace(v))]
	return s, ok
}

// EventFromData reads an event from a record produced by map.event (and possibly changed by
// later nodes) and checks it against the target schema.
func EventFromData(d map[string]any) (Event, error) {
	str := func(k string) string { return strings.TrimSpace(Stringify(d[k])) }
	e := Event{
		Title: str("title"), CI: str("ci"), Signal: str("signal"), Method: str("method"),
		Severity: str("severity"), Status: str("status"), ExternalID: str("external_id"), Value: str("value"), Key: str("key"),
	}
	if e.Key == "" {
		return e, errors.New("the record did not pass an “Event mapping” node")
	}
	if e.Title == "" {
		return e, errors.New("title is empty")
	}
	if e.CI == "" {
		return e, errors.New("ci is empty")
	}
	sev, ok := NormalizeSeverity(e.Severity)
	if !ok {
		return e, fmt.Errorf("severity %q is not critical, error, warning or info", e.Severity)
	}
	e.Severity = sev
	st, ok := NormalizeStatus(e.Status)
	if !ok {
		return e, fmt.Errorf("status %q is neither firing nor resolved", e.Status)
	}
	e.Status = st
	switch e.Method {
	case "":
		e.Method = MethodOther
	case MethodRED, MethodUSE, MethodOther:
	default:
		return e, fmt.Errorf("method %q is not red, use or other", e.Method)
	}
	for _, f := range []*string{&e.Title, &e.CI, &e.Signal, &e.ExternalID, &e.Value} {
		*f = textx.Runes(*f, maxEventField)
	}
	e.Labels = labelsOf(d["labels"])
	return e, nil
}

func labelsOf(v any) map[string]string {
	out := map[string]string{}
	m, ok := v.(map[string]any)
	if !ok {
		return out
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if len(out) == maxLabels {
			break
		}
		if s := Stringify(m[k]); s != "" {
			out[k] = truncate(s, maxEventField)
		}
	}
	return out
}

func eventKey(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}
