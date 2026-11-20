// Package metrics exports Prometheus metrics: HTTP latency, delivery
// outcomes and latency, circuit breaker state, lease recoveries and the
// queue depth read from the database at scrape time.
package metrics

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/RamizUnal/notification-system/internal/delivery"
	"github.com/RamizUnal/notification-system/internal/domain"
)

// Registry holds the Prometheus collectors of one process. It implements
// delivery.Observer, so the engine reports into it without depending on
// Prometheus.
type Registry struct {
	registry        *prometheus.Registry
	httpDuration    *prometheus.HistogramVec
	deliveries      *prometheus.CounterVec
	deliveryLatency *prometheus.HistogramVec
	releases        *prometheus.CounterVec
	circuitState    *prometheus.GaugeVec
	leasesRecovered prometheus.Counter
}

var _ delivery.Observer = (*Registry)(nil)

// New creates and registers the collectors, plus Go runtime and process
// metrics.
func New() *Registry {
	m := &Registry{
		registry: prometheus.NewRegistry(),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency by method, route pattern and status code.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route", "status"}),
		deliveries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "notification_delivery_attempts_total",
			Help: "Delivery attempts by channel and outcome (success, retryable_failure, permanent_failure).",
		}, []string{"channel", "outcome"}),
		deliveryLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "notification_delivery_duration_seconds",
			Help:    "Provider response time per delivery attempt.",
			Buckets: []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"channel"}),
		releases: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "notification_releases_total",
			Help: "Claimed notifications returned to the queue without an attempt, by reason.",
		}, []string{"channel", "reason"}),
		circuitState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "notification_circuit_state",
			Help: "Circuit breaker state per channel: 0 closed, 1 open, 2 half-open.",
		}, []string{"channel"}),
		leasesRecovered: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "notification_leases_recovered_total",
			Help: "Notifications requeued after their worker stopped mid-delivery.",
		}),
	}
	m.registry.MustRegister(
		m.httpDuration, m.deliveries, m.deliveryLatency, m.releases, m.circuitState, m.leasesRecovered,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	for _, ch := range domain.Channels() {
		m.circuitState.WithLabelValues(string(ch)).Set(0)
	}
	return m
}

// Handler serves the metrics in the Prometheus exposition format.
func (m *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{Registry: m.registry})
}

// Register adds further collectors, such as a QueueDepthCollector.
func (m *Registry) Register(c prometheus.Collector) {
	m.registry.MustRegister(c)
}

// ObserveHTTP records one served request.
func (m *Registry) ObserveHTTP(method, route string, status int, elapsed time.Duration) {
	m.httpDuration.WithLabelValues(method, route, strconv.Itoa(status)).Observe(elapsed.Seconds())
}

// Delivered implements delivery.Observer.
func (m *Registry) Delivered(ch domain.Channel, outcome domain.AttemptOutcome, latency time.Duration) {
	m.deliveries.WithLabelValues(string(ch), string(outcome)).Inc()
	m.deliveryLatency.WithLabelValues(string(ch)).Observe(latency.Seconds())
}

// Released implements delivery.Observer.
func (m *Registry) Released(ch domain.Channel, reason string) {
	m.releases.WithLabelValues(string(ch), reason).Inc()
}

// CircuitChanged implements delivery.Observer.
func (m *Registry) CircuitChanged(ch domain.Channel, state delivery.CircuitState) {
	m.circuitState.WithLabelValues(string(ch)).Set(float64(state))
}

// LeasesRecovered implements delivery.Observer.
func (m *Registry) LeasesRecovered(n int) {
	m.leasesRecovered.Add(float64(n))
}

// QueueSample is one queue-depth measurement.
type QueueSample struct {
	Channel, Priority, Status string
	Count                     int
}

// QueueDepthCollector exports the current queue depth, read from the
// database at scrape time so every replica reports the same truth.
type QueueDepthCollector struct {
	read   func(context.Context) ([]QueueSample, error)
	desc   *prometheus.Desc
	logger *slog.Logger
}

// NewQueueDepthCollector returns a collector that calls read on each scrape.
func NewQueueDepthCollector(read func(context.Context) ([]QueueSample, error), logger *slog.Logger) *QueueDepthCollector {
	return &QueueDepthCollector{
		read:   read,
		logger: logger,
		desc: prometheus.NewDesc("notification_queue_depth",
			"Unfinished notifications by channel, priority and status.",
			[]string{"channel", "priority", "status"}, nil),
	}
}

// Describe implements prometheus.Collector.
func (c *QueueDepthCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

// Collect implements prometheus.Collector.
func (c *QueueDepthCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	samples, err := c.read(ctx)
	if err != nil {
		c.logger.Warn("reading queue depth for metrics failed", slog.Any("error", err))
		return
	}
	for _, s := range samples {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(s.Count), s.Channel, s.Priority, s.Status)
	}
}
