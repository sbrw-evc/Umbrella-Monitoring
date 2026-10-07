package app

import "github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/collab"

// collab is the configured collaboration providers. Stub until the integrations are wired;
// integrations.go replaces this file.
func (a *App) collab() collab.Registry { return collab.Registry{} }
