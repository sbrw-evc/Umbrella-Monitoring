package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/flow"
)

const (
	StatusPending = "pending"
	StatusDone    = "done"
	StatusFailed  = "failed"

	claimBatch = 50
)

var ErrNotFound = errors.New("not found")

type Request struct {
	ID          int64             `json:"id"`
	ReceivedAt  time.Time         `json:"received_at"`
	ConnectorID string            `json:"connector_id"`
	Version     int               `json:"version"`
	Status      string            `json:"status"`
	Attempts    int               `json:"attempts"`
	RemoteIP    string            `json:"remote_ip"`
	Method      string            `json:"method"`
	Headers     map[string]string `json:"headers"`
	Query       map[string]string `json:"query"`
	Body        []byte            `json:"-"`
	Size        int               `json:"size"`
	ProcessedAt *time.Time        `json:"processed_at,omitempty"`
	Events      int               `json:"events"`
	Error       string            `json:"error,omitempty"`
}

// Outcome is what processing one request produced.
type Outcome struct {
	Version int
	Result  *flow.Result
}

// Processor runs a request through the connector version it was received for.
type Processor func(ctx context.Context, r Request) (Outcome, error)

// Sink receives the events of a processed request inside the transaction that stores them
// (the alert engine). after runs once the transaction is committed.
type Sink func(ctx context.Context, tx pgx.Tx, connectorID string, events []flow.Event) (after func(), err error)

type Queue struct {
	pool *pgxpool.Pool
	wake chan struct{}
	sink Sink

	mu       sync.Mutex
	rejected map[string]int
}

func New(pool *pgxpool.Pool) *Queue {
	return &Queue{pool: pool, wake: make(chan struct{}, 1), rejected: map[string]int{}}
}

// Enqueue stores a request durably. With an idempotency key, a request seen before is not
// stored again and dup is true.
func (q *Queue) Enqueue(ctx context.Context, r Request, idemKey string) (id int64, dup bool, err error) {
	h, _ := json.Marshal(nonNil(r.Headers))
	qs, _ := json.Marshal(nonNil(r.Query))
	err = pgx.BeginFunc(ctx, q.pool, func(tx pgx.Tx) error {
		if idemKey != "" {
			tag, err := tx.Exec(ctx, "INSERT INTO ingest_idempotency (connector_id, key) VALUES ($1, $2) ON CONFLICT DO NOTHING", r.ConnectorID, idemKey)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				dup = true
				return nil
			}
		}
		return tx.QueryRow(ctx, `INSERT INTO ingest_requests (connector_id, version, remote_ip, method, headers, query, body)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			r.ConnectorID, r.Version, r.RemoteIP, r.Method, h, qs, r.Body).Scan(&id)
	})
	if err == nil && !dup {
		q.Notify()
	}
	return id, dup, err
}

func nonNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// SetSink sets the receiver of processed events. Call it before Run.
func (q *Queue) SetSink(s Sink) { q.sink = s }

// Notify wakes a worker right away instead of at its next poll.
func (q *Queue) Notify() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// Reject counts a request turned away by the intake (authentication, network, size, rate).
func (q *Queue) Reject(connectorID string) {
	q.mu.Lock()
	q.rejected[connectorID]++
	q.mu.Unlock()
}

func (q *Queue) flushRejected(ctx context.Context) {
	q.mu.Lock()
	counts := q.rejected
	q.rejected = map[string]int{}
	q.mu.Unlock()
	for id, n := range counts {
		if _, err := q.pool.Exec(ctx, `INSERT INTO connector_stats (connector_id, bucket, rejected) VALUES ($1, date_trunc('minute', now()), $2)
			ON CONFLICT (connector_id, bucket) DO UPDATE SET rejected = connector_stats.rejected + EXCLUDED.rejected`, id, n); err != nil {
			slog.Warn("connector stats not written", "err", err)
		}
	}
}

// Run processes pending requests until ctx ends. Each batch is one transaction: the events,
// failures, statistics and the request status are written together, so a crash leaves the
// requests pending and they are processed again (at least once; event keys absorb repeats).
func (q *Queue) Run(ctx context.Context, workers int, process Processor) {
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tick := time.NewTicker(time.Second)
			defer tick.Stop()
			for {
				n, err := q.Drain(ctx, process)
				if err != nil && ctx.Err() == nil {
					slog.Error("ingest batch failed", "err", err)
				}
				if n == claimBatch {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case <-q.wake:
				case <-tick.C:
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(10 * time.Second)
		defer tick.Stop()
		upkeep := time.NewTicker(time.Hour)
		defer upkeep.Stop()
		for {
			select {
			case <-ctx.Done():
				q.flushRejected(context.Background())
				return
			case <-tick.C:
				q.flushRejected(ctx)
			case <-upkeep.C:
				if err := Maintain(ctx, q.pool, time.Now().UTC(), DefaultRetention); err != nil && ctx.Err() == nil {
					slog.Error("ingest maintenance failed", "err", err)
				}
			}
		}
	}()
	wg.Wait()
}

type statDelta struct {
	received, filtered, skipped, failed, events, duplicates int
	latency                                                 int64
	latencyMax                                              int
}

// Drain claims one batch of pending requests and processes it. It returns how many it took.
func (q *Queue) Drain(ctx context.Context, process Processor) (int, error) {
	n := 0
	var after []func()
	err := pgx.BeginFunc(ctx, q.pool, func(tx pgx.Tx) error {
		after = after[:0]
		rows, err := tx.Query(ctx, `SELECT id, received_at, connector_id, version, attempts, remote_ip, method, headers, query, body
			FROM ingest_requests WHERE status = 'pending' ORDER BY received_at LIMIT $1 FOR UPDATE SKIP LOCKED`, claimBatch)
		if err != nil {
			return err
		}
		reqs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Request, error) {
			var r Request
			err := row.Scan(&r.ID, &r.ReceivedAt, &r.ConnectorID, &r.Version, &r.Attempts, &r.RemoteIP, &r.Method, &r.Headers, &r.Query, &r.Body)
			return r, err
		})
		if err != nil {
			return err
		}
		n = len(reqs)
		stats := map[string]*statDelta{}
		for _, r := range reqs {
			st := stats[r.ConnectorID]
			if st == nil {
				st = &statDelta{}
				stats[r.ConnectorID] = st
			}
			f, err := q.handle(ctx, tx, r, process, st)
			if err != nil {
				return err
			}
			if f != nil {
				after = append(after, f)
			}
		}
		for id, st := range stats {
			if _, err := tx.Exec(ctx, `INSERT INTO connector_stats AS s (connector_id, bucket, received, filtered, skipped, failed, events, duplicates, latency_ms, latency_max)
				VALUES ($1, date_trunc('minute', now()), $2, $3, $4, $5, $6, $7, $8, $9)
				ON CONFLICT (connector_id, bucket) DO UPDATE SET
					received = s.received + EXCLUDED.received, filtered = s.filtered + EXCLUDED.filtered,
					skipped = s.skipped + EXCLUDED.skipped, failed = s.failed + EXCLUDED.failed,
					events = s.events + EXCLUDED.events, duplicates = s.duplicates + EXCLUDED.duplicates,
					latency_ms = s.latency_ms + EXCLUDED.latency_ms, latency_max = greatest(s.latency_max, EXCLUDED.latency_max)`,
				id, st.received, st.filtered, st.skipped, st.failed, st.events, st.duplicates, st.latency, st.latencyMax); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		for _, f := range after {
			f()
		}
	}
	return n, err
}

func (q *Queue) handle(ctx context.Context, tx pgx.Tx, r Request, process Processor, st *statDelta) (func(), error) {
	st.received++
	out, perr := process(ctx, r)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	version := r.Version
	if out.Version != 0 {
		version = out.Version
	}
	if perr != nil || out.Result == nil {
		msg := "the connector produced no result"
		if perr != nil {
			msg = perr.Error()
		}
		st.failed++
		if _, err := tx.Exec(ctx, `INSERT INTO ingest_failures (connector_id, version, node_id, error, request_id, request_at, item, raw)
			VALUES ($1, $2, '', $3, $4, $5, 0, $6)`, r.ConnectorID, version, msg, r.ID, r.ReceivedAt, clip(string(r.Body), 4000)); err != nil {
			return nil, err
		}
		return nil, finish(ctx, tx, r, StatusFailed, 0, msg)
	}
	res := out.Result
	st.filtered += res.Filtered
	st.skipped += res.Skipped
	inserted := 0
	for _, e := range res.Events {
		labels, _ := json.Marshal(e.Event.Labels)
		// changed: the first delivery of the alert or a change of its status or severity. The
		// rest are duplicates (prev is read before the upsert, in the same snapshot).
		var changed bool
		err := tx.QueryRow(ctx, `WITH prev AS (SELECT status, severity FROM connector_events WHERE connector_id = $1 AND key = $3)
			INSERT INTO connector_events AS ce (connector_id, version, key, title, ci, signal, method, severity, status,
				external_id, value, labels, request_id, request_at, item)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
			ON CONFLICT (connector_id, key) DO UPDATE SET version = EXCLUDED.version, title = EXCLUDED.title, ci = EXCLUDED.ci,
				signal = EXCLUDED.signal, method = EXCLUDED.method, severity = EXCLUDED.severity, status = EXCLUDED.status,
				external_id = EXCLUDED.external_id, value = EXCLUDED.value, labels = EXCLUDED.labels, request_id = EXCLUDED.request_id,
				request_at = EXCLUDED.request_at, item = EXCLUDED.item, last_seen = now(), seen = ce.seen + 1
			RETURNING coalesce((SELECT status <> $9 OR severity <> $8 FROM prev), true)`,
			r.ConnectorID, version, e.Event.Key, e.Event.Title, e.Event.CI, e.Event.Signal, e.Event.Method, e.Event.Severity, e.Event.Status,
			e.Event.ExternalID, e.Event.Value, labels, r.ID, r.ReceivedAt, e.Lineage.Item).Scan(&changed)
		if err != nil {
			return nil, err
		}
		if changed {
			inserted++
		} else {
			st.duplicates++
		}
	}
	st.events += inserted
	for _, f := range res.Failures {
		data, _ := json.Marshal(f.Data)
		if _, err := tx.Exec(ctx, `INSERT INTO ingest_failures (connector_id, version, node_id, error, request_id, request_at, item, data, raw)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, r.ConnectorID, version, f.Node, clip(f.Error, 2000), r.ID, r.ReceivedAt, f.Lineage.Item, data, f.Raw); err != nil {
			return nil, err
		}
	}
	st.failed += len(res.Failures)
	lat := time.Since(r.ReceivedAt).Milliseconds()
	st.latency += lat
	if int(lat) > st.latencyMax {
		st.latencyMax = int(lat)
	}
	status, msg := StatusDone, ""
	if len(res.Failures) > 0 {
		status = StatusFailed
		msg = fmt.Sprintf("%d of the records failed; first: %s", len(res.Failures), clip(res.Failures[0].Error, 500))
	}
	if _, err := tx.Exec(ctx, "UPDATE ingest_requests SET version = $3 WHERE id = $1 AND received_at = $2", r.ID, r.ReceivedAt, version); err != nil {
		return nil, err
	}
	after := q.deliver(ctx, tx, r, res)
	return after, finish(ctx, tx, r, status, len(res.Events), msg)
}

// deliver hands the events to the sink in a savepoint: an alert that cannot be folded does not
// hold back the events, it is logged instead.
func (q *Queue) deliver(ctx context.Context, tx pgx.Tx, r Request, res *flow.Result) func() {
	if q.sink == nil || len(res.Events) == 0 {
		return nil
	}
	events := make([]flow.Event, 0, len(res.Events))
	for _, e := range res.Events {
		events = append(events, e.Event)
	}
	var after func()
	err := pgx.BeginFunc(ctx, tx, func(sp pgx.Tx) error {
		var err error
		after, err = q.sink(ctx, sp, r.ConnectorID, events)
		return err
	})
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("events not folded into alerts", "connector", r.ConnectorID, "request", r.ID, "err", err)
		}
		return nil
	}
	return after
}

func finish(ctx context.Context, tx pgx.Tx, r Request, status string, events int, msg string) error {
	_, err := tx.Exec(ctx, `UPDATE ingest_requests SET status = $3, attempts = attempts + 1, processed_at = now(), events = $4, error = $5
		WHERE id = $1 AND received_at = $2`, r.ID, r.ReceivedAt, status, events, msg)
	return err
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
