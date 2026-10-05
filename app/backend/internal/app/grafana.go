package app

import "github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/alert"

// grafanaLink is the address of the incident context in Grafana; empty until Grafana is set up.
func (a *App) grafanaLink(al alert.Alert) string { return "" }
