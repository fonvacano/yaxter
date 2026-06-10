package relay

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	"github.com/fonvacano/yaxter/pkg/outbox"
)

// Publisher delivers a batch to the broker preserving slice order.
// Implementations must be at-least-once: returning nil means every message
// is durably accepted by the broker.
type Publisher interface {
	Publish(ctx context.Context, msgs []outbox.Message) error
}

type Config struct {
	PollInterval time.Duration // default 200ms (§2.4)
	BatchSize    int           // default 500
	DeleteGrace  time.Duration // published rows older than this are deleted
	DeleteBatch  int           // max rows deleted per cycle
	LockID       int64         // advisory lock; unique per physical shard
}

func DefaultConfig() Config {
	return Config{
		PollInterval: 200 * time.Millisecond,
		BatchSize:    500,
		DeleteGrace:  time.Minute,
		DeleteBatch:  1000,
		LockID:       0x79617874, // "yaxt"; shard index is added per relay
	}
}

// Metrics are the §5 outbox SLIs. Registering on a nil registry keeps the
// collectors usable in tests without a server.
type Metrics struct {
	PendingRows prometheus.Gauge
	LagSeconds  prometheus.Gauge
	Published   prometheus.Counter
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		PendingRows: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "outbox_pending_rows",
			Help: "Unpublished outbox rows.",
		}),
		LagSeconds: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "outbox_lag_seconds",
			Help: "Age of the oldest unpublished outbox row.",
		}),
		Published: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "outbox_published_total",
			Help: "Outbox rows published to the broker.",
		}),
	}
	if reg != nil {
		reg.MustRegister(m.PendingRows, m.LagSeconds, m.Published)
	}
	return m
}

type Relay struct {
	pool *pgxpool.Pool
	pub  Publisher
	cfg  Config
	m    *Metrics
	log  zerolog.Logger
}

func New(pool *pgxpool.Pool, pub Publisher, cfg Config, m *Metrics, log zerolog.Logger) *Relay {
	return &Relay{pool: pool, pub: pub, cfg: cfg, m: m, log: log}
}

// Run acquires leadership, then polls forever. Errors inside a cycle are
// logged and retried next tick — a broker outage must never crash the relay
// (the outbox absorbs, §2.4 degradation story).
func (r *Relay) Run(ctx context.Context) error {
	release, err := AcquireLeadership(ctx, r.pool, r.cfg.LockID, time.Second)
	if err != nil {
		return err
	}
	defer release()
	r.log.Info().Int64("lock_id", r.cfg.LockID).Msg("relay leadership acquired")

	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := r.cycle(ctx); err != nil {
				r.log.Error().Err(err).Msg("relay cycle failed; will retry")
			}
		}
	}
}

// cycle drains the backlog (publish+mark per batch), deletes old published
// rows, and refreshes the gauges.
func (r *Relay) cycle(ctx context.Context) error {
	defer r.observe(ctx)
	for {
		n, err := r.publishBatch(ctx)
		if err != nil {
			return err
		}
		if n < r.cfg.BatchSize {
			break
		}
	}
	return r.deleteOld(ctx)
}

func (r *Relay) publishBatch(ctx context.Context) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	rows, err := tx.Query(ctx, `
		SELECT id, topic, key, payload, COALESCE(traceparent, '')
		FROM outbox
		WHERE published_at IS NULL
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, r.cfg.BatchSize)
	if err != nil {
		return 0, err
	}
	var msgs []outbox.Message
	var ids []int64
	for rows.Next() {
		var m outbox.Message
		if err := rows.Scan(&m.ID, &m.Topic, &m.Key, &m.Payload, &m.Traceparent); err != nil {
			rows.Close()
			return 0, err
		}
		msgs = append(msgs, m)
		ids = append(ids, m.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(msgs) == 0 {
		return 0, nil
	}

	// At-least-once: a crash after Publish but before Commit re-publishes
	// the batch next cycle; consumers dedupe by envelope.event_id.
	if err := r.pub.Publish(ctx, msgs); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE outbox SET published_at = now() WHERE id = ANY($1)`, ids); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	r.m.Published.Add(float64(len(msgs)))
	return len(msgs), nil
}

func (r *Relay) deleteOld(ctx context.Context) error {
	// make_interval, not Duration.String()::interval — Postgres parses the
	// "m" in Go's "1m0s" as months.
	_, err := r.pool.Exec(ctx, `
		DELETE FROM outbox WHERE id IN (
			SELECT id FROM outbox
			WHERE published_at IS NOT NULL
			  AND published_at < now() - make_interval(secs => $1)
			LIMIT $2
		)`, r.cfg.DeleteGrace.Seconds(), r.cfg.DeleteBatch)
	return err
}

func (r *Relay) observe(ctx context.Context) {
	var pending int64
	var lag *float64
	err := r.pool.QueryRow(ctx, `
		SELECT count(*), EXTRACT(EPOCH FROM now() - min(created_at))
		FROM outbox WHERE published_at IS NULL`).Scan(&pending, &lag)
	if err != nil {
		r.log.Warn().Err(err).Msg("outbox metrics query failed")
		return
	}
	r.m.PendingRows.Set(float64(pending))
	if lag != nil {
		r.m.LagSeconds.Set(*lag)
	} else {
		r.m.LagSeconds.Set(0)
	}
}
