package app

import (
	"context"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/config"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/directory"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/secrets"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type Secrets interface {
	Resolve(ref string) (string, error)
	PutRef(ctx context.Context, path, key, value string) (string, error)
	Status(ctx context.Context) secrets.Status
}

type Directory interface {
	Authenticate(cfg directory.Config, bindPassword, username, password string) (directory.Identity, error)
	Test(cfg directory.Config, bindPassword, username, password string) (directory.Probe, error)
	Memberships(cfg directory.Config, bindPassword string, usernames []string) (map[string]directory.Membership, error)
}

type Database interface {
	Where() string
	Info(ctx context.Context) (store.PGInfo, error)
	Health(ctx context.Context) (store.PGHealth, error)
}

type Runtime interface {
	Switch(ctx context.Context, transfer func(ctx context.Context) (config.File, error)) error
}

type ldapDirectory struct{}

func (ldapDirectory) Authenticate(cfg directory.Config, bindPassword, username, password string) (directory.Identity, error) {
	return directory.Authenticate(cfg, bindPassword, username, password)
}

func (ldapDirectory) Memberships(cfg directory.Config, bindPassword string, usernames []string) (map[string]directory.Membership, error) {
	return directory.Memberships(cfg, bindPassword, usernames)
}

func (ldapDirectory) Test(cfg directory.Config, bindPassword, username, password string) (directory.Probe, error) {
	return directory.Test(cfg, bindPassword, username, password)
}
