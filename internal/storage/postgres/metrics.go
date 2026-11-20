package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/service"
)

// MetricsRepository implements service.MetricsRepository with aggregate
// queries. Every number comes from the source of truth, so the metrics are
// consistent across API replicas and survive restarts.
type MetricsRepository struct {
	pool *pgxpool.Pool
}

// NewMetricsRepository returns a repository using pool.
func NewMetricsRepository(pool *pgxpool.Pool) *MetricsRepository {
	return &MetricsRepository{pool: pool}
}

var _ service.MetricsRepository = (*MetricsRepository)(nil)

// QueueDepth counts unfinished notifications per channel, priority and status.
func (r *MetricsRepository) QueueDepth(ctx context.Context, now time.Time) ([]service.QueueDepth, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT channel, priority, status, count(*), count(*) FILTER (WHERE next_attempt_at <= $1)
		FROM notifications
		WHERE status IN ('scheduled', 'queued', 'retrying', 'processing')
		GROUP BY channel, priority, status`, now)
	if err != nil {
		return nil, fmt.Errorf("query queue depth: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (service.QueueDepth, error) {
		var (
			d                         service.QueueDepth
			channel, priority, status string
		)
		err := row.Scan(&channel, &priority, &status, &d.Count, &d.Due)
		d.Channel, d.Priority, d.Status = domain.Channel(channel), domain.Priority(priority), domain.Status(status)
		return d, err
	})
}

// DeliveryStats aggregates attempts per channel since the given time.
func (r *MetricsRepository) DeliveryStats(ctx context.Context, since time.Time) ([]service.DeliveryStats, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT channel,
		       count(*) FILTER (WHERE outcome = 'success'),
		       count(*) FILTER (WHERE outcome = 'retryable_failure'),
		       count(*) FILTER (WHERE outcome = 'permanent_failure'),
		       COALESCE(percentile_cont(0.50) WITHIN GROUP (ORDER BY duration_ms), 0),
		       COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms), 0),
		       COALESCE(percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ms), 0)
		FROM delivery_attempts
		WHERE finished_at >= $1
		GROUP BY channel`, since)
	if err != nil {
		return nil, fmt.Errorf("query delivery stats: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (service.DeliveryStats, error) {
		var (
			s             service.DeliveryStats
			channel       string
			p50, p95, p99 float64
		)
		err := row.Scan(&channel, &s.Succeeded, &s.RetryableFailures, &s.PermanentFailures, &p50, &p95, &p99)
		s.Channel = domain.Channel(channel)
		s.Latency = service.Percentiles{P50: millis(p50), P95: millis(p95), P99: millis(p99)}
		return s, err
	})
}

// Throughput counts attempts per channel in buckets aligned to the Unix
// epoch, matching service.AlignToBucket.
func (r *MetricsRepository) Throughput(ctx context.Context, since time.Time, bucket time.Duration) ([]service.ThroughputPoint, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT date_bin(make_interval(secs => $2), finished_at, 'epoch'::timestamptz) AS bucket,
		       channel,
		       count(*) FILTER (WHERE outcome = 'success'),
		       count(*) FILTER (WHERE outcome <> 'success')
		FROM delivery_attempts
		WHERE finished_at >= $1
		GROUP BY bucket, channel
		ORDER BY bucket`, since, bucket.Seconds())
	if err != nil {
		return nil, fmt.Errorf("query throughput: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (service.ThroughputPoint, error) {
		var (
			p       service.ThroughputPoint
			channel string
		)
		err := row.Scan(&p.At, &channel, &p.Succeeded, &p.Failed)
		p.Channel = domain.Channel(channel)
		p.At = p.At.UTC()
		return p, err
	})
}

// StatusCounts counts notifications created since the given time by status.
func (r *MetricsRepository) StatusCounts(ctx context.Context, createdSince time.Time) (map[domain.Status]int, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT status, count(*) FROM notifications WHERE created_at >= $1 GROUP BY status`, createdSince)
	if err != nil {
		return nil, fmt.Errorf("query status counts: %w", err)
	}
	counts := make(map[domain.Status]int)
	var (
		status string
		count  int
	)
	if _, err := pgx.ForEachRow(rows, []any{&status, &count}, func() error {
		counts[domain.Status(status)] = count
		return nil
	}); err != nil {
		return nil, fmt.Errorf("query status counts: %w", err)
	}
	return counts, nil
}

// Workers returns the workers that sent a heartbeat since the given time.
func (r *MetricsRepository) Workers(ctx context.Context, seenSince time.Time) ([]service.WorkerStatus, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT worker_id, started_at, seen_at, in_flight, circuits
		FROM worker_heartbeats
		WHERE seen_at >= $1
		ORDER BY worker_id`, seenSince)
	if err != nil {
		return nil, fmt.Errorf("query workers: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (service.WorkerStatus, error) {
		var (
			w   service.WorkerStatus
			raw []byte
		)
		if err := row.Scan(&w.ID, &w.StartedAt, &w.SeenAt, &w.InFlight, &raw); err != nil {
			return w, err
		}
		w.StartedAt, w.SeenAt = w.StartedAt.UTC(), w.SeenAt.UTC()

		var circuits map[string]circuitJSON
		if err := json.Unmarshal(raw, &circuits); err != nil {
			return w, fmt.Errorf("decode circuits of worker %s: %w", w.ID, err)
		}
		w.Circuits = make(map[domain.Channel]service.CircuitInfo, len(circuits))
		for ch, c := range circuits {
			w.Circuits[domain.Channel(ch)] = service.CircuitInfo{State: c.State, RetryAt: c.RetryAt}
		}
		return w, nil
	})
}

func millis(ms float64) time.Duration {
	return time.Duration(ms * float64(time.Millisecond))
}
