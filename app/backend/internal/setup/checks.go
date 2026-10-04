package setup

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/config"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/directory"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/httpx"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/secrets"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

type openbaoReply struct {
	OK         bool           `json:"ok"`
	Status     secrets.Status `json:"status"`
	WriteOK    bool           `json:"write_ok"`
	WriteError string         `json:"write_error,omitempty"`
	Error      string         `json:"error,omitempty"`
}

func (m *Module) testOpenBao(w http.ResponseWriter, r *http.Request) {
	var in config.OpenBao
	if !httpx.Decode(w, r, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	_, rep := checkOpenBao(ctx, in)
	httpx.JSON(w, http.StatusOK, rep)
}

func checkOpenBao(ctx context.Context, in config.OpenBao) (*secrets.Client, openbaoReply) {
	var rep openbaoReply
	in, err := in.Normalize()
	if err != nil {
		rep.Error = err.Error()
		return nil, rep
	}
	c, err := in.Client()
	if err != nil {
		rep.Error = err.Error()
		return nil, rep
	}
	rep.Status = c.Status(ctx)
	if !rep.Status.TokenOK || !rep.Status.MountOK {
		rep.Error = rep.Status.Error
		return nil, rep
	}
	probe := "setup-probe-" + auth.RandomToken("", 6)
	if err := c.Put(ctx, probe, map[string]string{"probe": time.Now().UTC().Format(time.RFC3339)}); err != nil {
		rep.WriteError = err.Error()
		rep.Error = "cannot write secrets: " + err.Error()
		return nil, rep
	}
	if err := c.Delete(ctx, probe); err != nil {
		rep.WriteError = err.Error()
		rep.Error = "cannot delete secrets: " + err.Error()
		return nil, rep
	}
	rep.WriteOK, rep.OK = true, true
	return c, rep
}

type pgInput struct {
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Database      string `json:"database"`
	User          string `json:"user"`
	Password      string `json:"password"`
	SSLMode       string `json:"sslmode"`
	ReuseExisting bool   `json:"reuse_existing"`
}

func (p pgInput) config() store.PGConfig {
	return store.PGConfig{Host: strings.TrimSpace(p.Host), Port: p.Port, Database: strings.TrimSpace(p.Database),
		User: strings.TrimSpace(p.User), SSLMode: p.SSLMode, Password: p.Password}
}

type pgReply struct {
	OK    bool          `json:"ok"`
	Probe store.PGProbe `json:"probe"`
	Error string        `json:"error,omitempty"`
}

func (m *Module) testPostgres(w http.ResponseWriter, r *http.Request) {
	var in pgInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	probe, err := store.ProbePostgres(r.Context(), in.config())
	rep := pgReply{OK: err == nil, Probe: probe}
	if err != nil {
		rep.Error = err.Error()
	}
	httpx.JSON(w, http.StatusOK, rep)
}

type ldapInput struct {
	Config       directory.Config `json:"config"`
	BindPassword string           `json:"bind_password"`
	TestUsername string           `json:"test_username,omitempty"`
	TestPassword string           `json:"test_password,omitempty"`
}

type ldapReply struct {
	OK    bool            `json:"ok"`
	Probe directory.Probe `json:"probe"`
	Error string          `json:"error,omitempty"`
}

func (m *Module) testLDAP(w http.ResponseWriter, r *http.Request) {
	var in ldapInput
	if !httpx.Decode(w, r, &in) {
		return
	}
	probe, err := directory.Test(in.Config, in.BindPassword, in.TestUsername, in.TestPassword)
	rep := ldapReply{OK: err == nil, Probe: probe}
	if err != nil {
		rep.Error = err.Error()
	}
	httpx.JSON(w, http.StatusOK, rep)
}
