package service

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/RamizUnal/notification-system/internal/domain"
)

// Metrics windows.
const (
	DefaultMetricsWindow = 5 * time.Minute
	MinMetricsWindow     = time.Minute
	MaxMetricsWindow     = 24 * time.Hour
	// maxBuckets bounds the points per channel in the throughput series;
	// minBucket keeps short windows at one-second resolution, so the series
	// agrees with what a live per-second meter shows.
	maxBuckets = 300
	minBucket  = time.Second
	// workerTTL is how long a worker counts as alive after its last heartbeat.
	workerTTL = 15 * time.Second
)

// MetricsRepository reads the aggregates behind the metrics endpoint.
type MetricsRepository interface {
	QueueDepth(ctx context.Context, now time.Time) ([]QueueDepth, error)
	DeliveryStats(ctx context.Context, since time.Time) ([]DeliveryStats, error)
	Throughput(ctx context.Context, since time.Time, bucket time.Duration) ([]ThroughputPoint, error)
	StatusCounts(ctx context.Context, createdSince time.Time) (map[domain.Status]int, error)
	Workers(ctx context.Context, seenSince time.Time) ([]WorkerStatus, error)
}

// QueueDepth counts unfinished notifications of one channel, priority and status.
type QueueDepth struct {
	Channel  domain.Channel
	Priority domain.Priority
	Status   domain.Status
	Count    int
	// Due is how many of them are ready for delivery now; the rest are
	// scheduled for later or waiting for a retry.
	Due int
}

// DeliveryStats aggregates the delivery attempts of one channel in a window.
type DeliveryStats struct {
	Channel           domain.Channel
	Succeeded         int
	RetryableFailures int
	PermanentFailures int
	Latency           Percentiles
}

// Attempts is the total number of delivery attempts.
func (s DeliveryStats) Attempts() int {
	return s.Succeeded + s.RetryableFailures + s.PermanentFailures
}

// Percentiles of provider response time.
type Percentiles struct {
	P50, P95, P99 time.Duration
}

// ThroughputPoint counts the attempts of one channel finished in one bucket.
type ThroughputPoint struct {
	At        time.Time
	Channel   domain.Channel
	Succeeded int
	Failed    int
}

// CircuitInfo is a worker's view of one channel's circuit breaker.
type CircuitInfo struct {
	State   string // closed, open or half_open
	RetryAt *time.Time
}

// WorkerStatus is the last heartbeat of a live worker.
type WorkerStatus struct {
	ID        string
	StartedAt time.Time
	SeenAt    time.Time
	InFlight  int
	Circuits  map[domain.Channel]CircuitInfo
}

// ChannelMetrics summarizes one channel.
type ChannelMetrics struct {
	Channel domain.Channel
	// Waiting counts due and deferred notifications per priority.
	Waiting map[domain.Priority]int
	// Due is how many waiting notifications are ready to send right now: the
	// backlog that rate limiting drains.
	Due        int
	Scheduled  int
	Retrying   int
	Processing int
	Stats      DeliveryStats
	// SuccessRate is the share of attempts in the window that succeeded, or
	// nil when there were none.
	SuccessRate *float64
	// Throughput is successful deliveries per second over the window.
	Throughput float64
	RateLimit  int
	// Circuit is the most severe breaker state reported by live workers:
	// open, then half_open, then closed; "unknown" without live workers.
	Circuit string
}

// MetricsSnapshot is the system's health at one moment.
type MetricsSnapshot struct {
	GeneratedAt  time.Time
	Window       time.Duration
	Bucket       time.Duration
	Channels     []ChannelMetrics
	Throughput   []ThroughputPoint
	StatusCounts map[domain.Status]int // notifications created within the window
	Workers      []WorkerStatus
}

// MetricsService assembles metrics snapshots.
type MetricsService struct {
	settings
	repo      MetricsRepository
	rateLimit int
}

// NewMetricsService returns the service. rateLimit is the configured
// per-channel ceiling, reported alongside the measured throughput.
func NewMetricsService(repo MetricsRepository, rateLimit int, opts ...Option) *MetricsService {
	return &MetricsService{settings: newSettings(opts), repo: repo, rateLimit: rateLimit}
}

// Snapshot reports queue depth, delivery outcomes, latency and throughput
// over the trailing window.
func (s *MetricsService) Snapshot(ctx context.Context, window time.Duration) (MetricsSnapshot, error) {
	if window == 0 {
		window = DefaultMetricsWindow
	}
	if window < MinMetricsWindow || window > MaxMetricsWindow {
		verr := &domain.ValidationError{}
		verr.Add("window", domain.CodeOutOfRange, "window must be between 1m and 24h")
		return MetricsSnapshot{}, verr
	}

	now := s.now().UTC()
	since := now.Add(-window)
	bucket := max(minBucket, window/maxBuckets)

	depth, err := s.repo.QueueDepth(ctx, now)
	if err != nil {
		return MetricsSnapshot{}, fmt.Errorf("read queue depth: %w", err)
	}
	stats, err := s.repo.DeliveryStats(ctx, since)
	if err != nil {
		return MetricsSnapshot{}, fmt.Errorf("read delivery stats: %w", err)
	}
	points, err := s.repo.Throughput(ctx, since, bucket)
	if err != nil {
		return MetricsSnapshot{}, fmt.Errorf("read throughput: %w", err)
	}
	counts, err := s.repo.StatusCounts(ctx, since)
	if err != nil {
		return MetricsSnapshot{}, fmt.Errorf("read status counts: %w", err)
	}
	workers, err := s.repo.Workers(ctx, now.Add(-workerTTL))
	if err != nil {
		return MetricsSnapshot{}, fmt.Errorf("read workers: %w", err)
	}

	return MetricsSnapshot{
		GeneratedAt:  now,
		Window:       window,
		Bucket:       bucket,
		Channels:     s.channelMetrics(depth, stats, workers, window),
		Throughput:   fillSeries(points, since, now, bucket),
		StatusCounts: counts,
		Workers:      workers,
	}, nil
}

func (s *MetricsService) channelMetrics(depth []QueueDepth, stats []DeliveryStats, workers []WorkerStatus, window time.Duration) []ChannelMetrics {
	byChannel := make(map[domain.Channel]*ChannelMetrics)
	for _, ch := range domain.Channels() {
		byChannel[ch] = &ChannelMetrics{
			Channel:   ch,
			Waiting:   map[domain.Priority]int{domain.PriorityHigh: 0, domain.PriorityNormal: 0, domain.PriorityLow: 0},
			Stats:     DeliveryStats{Channel: ch},
			RateLimit: s.rateLimit,
			Circuit:   circuitSummary(ch, workers),
		}
	}

	for _, d := range depth {
		m, ok := byChannel[d.Channel]
		if !ok {
			continue
		}
		switch d.Status {
		case domain.StatusProcessing:
			m.Processing += d.Count
		case domain.StatusScheduled:
			m.Scheduled += d.Count
			m.Waiting[d.Priority] += d.Count
		case domain.StatusRetrying:
			m.Retrying += d.Count
			m.Waiting[d.Priority] += d.Count
		default:
			m.Waiting[d.Priority] += d.Count
		}
		if d.Status.IsWaiting() {
			m.Due += d.Due
		}
	}

	for _, st := range stats {
		m, ok := byChannel[st.Channel]
		if !ok {
			continue
		}
		m.Stats = st
		if total := st.Attempts(); total > 0 {
			rate := float64(st.Succeeded) / float64(total)
			m.SuccessRate = &rate
		}
		m.Throughput = float64(st.Succeeded) / window.Seconds()
	}

	out := make([]ChannelMetrics, 0, len(byChannel))
	for _, ch := range domain.Channels() {
		out = append(out, *byChannel[ch])
	}
	return out
}

// circuitSummary reports the most severe state any live worker sees.
func circuitSummary(ch domain.Channel, workers []WorkerStatus) string {
	if len(workers) == 0 {
		return "unknown"
	}
	severity := map[string]int{"closed": 0, "half_open": 1, "open": 2}
	worst := "closed"
	for _, w := range workers {
		if c, ok := w.Circuits[ch]; ok && severity[c.State] > severity[worst] {
			worst = c.State
		}
	}
	return worst
}

// AlignToBucket rounds t down to a multiple of bucket since the Unix epoch,
// the same alignment PostgreSQL's date_bin(bucket, t, 'epoch') uses.
func AlignToBucket(t time.Time, bucket time.Duration) time.Time {
	ns := t.UnixNano()
	return time.Unix(0, ns-ns%int64(bucket)).UTC()
}

// fillSeries returns one point per channel and bucket in [since, now],
// inserting zeros where nothing happened, so charts get continuous lines.
func fillSeries(points []ThroughputPoint, since, now time.Time, bucket time.Duration) []ThroughputPoint {
	type key struct {
		at time.Time
		ch domain.Channel
	}
	index := make(map[key]ThroughputPoint, len(points))
	for _, p := range points {
		index[key{AlignToBucket(p.At, bucket), p.Channel}] = p
	}

	var out []ThroughputPoint
	for at := AlignToBucket(since, bucket); !at.After(now); at = at.Add(bucket) {
		for _, ch := range domain.Channels() {
			p, ok := index[key{at, ch}]
			if !ok {
				p = ThroughputPoint{At: at, Channel: ch}
			}
			p.At = at
			out = append(out, p)
		}
	}
	slices.SortStableFunc(out, func(a, b ThroughputPoint) int { return a.At.Compare(b.At) })
	return out
}
