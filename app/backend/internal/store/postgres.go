package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PGConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	User     string `json:"user"`
	SSLMode  string `json:"sslmode"`
	Password string `json:"-"`
}

func (c PGConfig) Normalize() (PGConfig, error) {
	if c.Port == 0 {
		c.Port = 5432
	}
	if c.SSLMode == "" {
		c.SSLMode = "prefer"
	}
	switch c.SSLMode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return c, errors.New("sslmode must be disable, allow, prefer, require, verify-ca or verify-full")
	}
	if c.Host == "" || c.Database == "" || c.User == "" {
		return c, errors.New("host, database and user are required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return c, errors.New("invalid port")
	}
	return c, nil
}

func (c PGConfig) DSN() string {
	u := url.URL{Scheme: "postgres", Host: c.Host + ":" + strconv.Itoa(c.Port), Path: "/" + c.Database,
		User: url.UserPassword(c.User, c.Password)}
	q := url.Values{"sslmode": {c.SSLMode}, "application_name": {"umbrella"}, "connect_timeout": {"10"}}
	u.RawQuery = q.Encode()
	return u.String()
}

func (c PGConfig) Where() string {
	return fmt.Sprintf("postgres://%s@%s:%d/%s", c.User, c.Host, c.Port, c.Database)
}

type PGBackend struct {
	cfg  PGConfig
	pool *pgxpool.Pool
}

const pgSchema = `CREATE TABLE IF NOT EXISTS umbrella_state (
	name     text PRIMARY KEY,
	format   integer NOT NULL,
	saved_at timestamptz NOT NULL,
	data     bytea NOT NULL
)`

func OpenPostgres(ctx context.Context, cfg PGConfig) (*PGBackend, error) {
	cfg, err := cfg.Normalize()
	if err != nil {
		return nil, err
	}
	pc, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, err
	}
	pc.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := pool.Ping(cctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("PostgreSQL %s: %w", cfg.Where(), err)
	}
	if _, err := pool.Exec(cctx, pgSchema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("PostgreSQL %s: create table umbrella_state: %w", cfg.Where(), err)
	}
	return &PGBackend{cfg: cfg, pool: pool}, nil
}

type PGInfo struct {
	Version  string     `json:"version"`
	HasState bool       `json:"has_state"`
	SavedAt  *time.Time `json:"saved_at,omitempty"`
}

func (p *PGBackend) Info(ctx context.Context) (PGInfo, error) {
	var info PGInfo
	if err := p.pool.QueryRow(ctx, "SHOW server_version").Scan(&info.Version); err != nil {
		return info, err
	}
	var at time.Time
	err := p.pool.QueryRow(ctx, "SELECT saved_at FROM umbrella_state WHERE name = 'current'").Scan(&at)
	if err == nil {
		info.HasState, info.SavedAt = true, &at
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return info, err
	}
	return info, nil
}

func (p *PGBackend) Kind() string  { return "postgres" }
func (p *PGBackend) Where() string { return p.cfg.Where() }
func (p *PGBackend) Close()        { p.pool.Close() }

func (p *PGBackend) Load(ctx context.Context) ([][]byte, error) {
	rows, err := p.pool.Query(ctx, "SELECT data FROM umbrella_state WHERE name IN ('current', 'previous') ORDER BY name = 'current' DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, ErrNoState
	}
	return out, nil
}

func (p *PGBackend) Save(ctx context.Context, data []byte) error {
	return pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO umbrella_state (name, format, saved_at, data)
			SELECT 'previous', format, saved_at, data FROM umbrella_state WHERE name = 'current'
			ON CONFLICT (name) DO UPDATE SET format = EXCLUDED.format, saved_at = EXCLUDED.saved_at, data = EXCLUDED.data`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO umbrella_state (name, format, saved_at, data) VALUES ('current', $1, now(), $2)
			ON CONFLICT (name) DO UPDATE SET format = EXCLUDED.format, saved_at = EXCLUDED.saved_at, data = EXCLUDED.data`,
			snapshotFormat, data)
		return err
	})
}

type PGProbe struct {
	Version   string     `json:"version"`
	Database  string     `json:"database"`
	User      string     `json:"user"`
	CanCreate bool       `json:"can_create"`
	HasState  bool       `json:"has_state"`
	SavedAt   *time.Time `json:"saved_at,omitempty"`
}

func ProbePostgres(ctx context.Context, cfg PGConfig) (PGProbe, error) {
	var p PGProbe
	cfg, err := cfg.Normalize()
	if err != nil {
		return p, err
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, err := pgx.Connect(cctx, cfg.DSN())
	if err != nil {
		return p, fmt.Errorf("PostgreSQL %s: %w", cfg.Where(), err)
	}
	defer conn.Close(context.Background())
	var exists bool
	err = conn.QueryRow(cctx, `SELECT current_setting('server_version'), current_database(), current_user,
		has_schema_privilege(current_user, current_schema(), 'CREATE'), to_regclass('umbrella_state') IS NOT NULL`).
		Scan(&p.Version, &p.Database, &p.User, &p.CanCreate, &exists)
	if err != nil {
		return p, fmt.Errorf("PostgreSQL %s: %w", cfg.Where(), err)
	}
	if !exists {
		return p, nil
	}
	var at time.Time
	err = conn.QueryRow(cctx, "SELECT saved_at FROM umbrella_state WHERE name = 'current'").Scan(&at)
	switch {
	case err == nil:
		p.HasState, p.SavedAt = true, &at
	case !errors.Is(err, pgx.ErrNoRows):
		return p, fmt.Errorf("PostgreSQL %s: %w", cfg.Where(), err)
	}
	return p, nil
}
