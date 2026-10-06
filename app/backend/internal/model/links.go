package model

import (
	"net/url"
	"strings"
)

// IncidentURL is the page of an incident in Umbrella under the public address base (a trailing
// slash is dropped); an empty base gives the path alone.
func IncidentURL(base, id string) string {
	return strings.TrimRight(base, "/") + "/incidents?id=" + url.QueryEscape(id)
}

// GrafanaHopURL is the link that opens the Grafana context of an incident through Umbrella, so
// the link stays right when the Grafana settings change.
func GrafanaHopURL(base, id string) string {
	return strings.TrimRight(base, "/") + "/go/incidents/" + url.PathEscape(id) + "/grafana"
}
