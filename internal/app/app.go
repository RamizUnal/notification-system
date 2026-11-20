// Package app is the composition root: it wires configuration, adapters,
// services and transports into runnable processes. It is the only package
// that knows every concrete type, which keeps every other package
// independent of the others' implementations.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"

	"github.com/RamizUnal/notification-system/api"
	"github.com/RamizUnal/notification-system/internal/delivery"
	"github.com/RamizUnal/notification-system/internal/httpapi"
	"github.com/RamizUnal/notification-system/internal/observability/metrics"
	"github.com/RamizUnal/notification-system/internal/provider"
	"github.com/RamizUnal/notification-system/internal/realtime"
	"github.com/RamizUnal/notification-system/internal/service"
	"github.com/RamizUnal/notification-system/internal/storage/postgres"
)

// APIOptions configures BuildAPI.
type APIOptions struct {
	Pool           *pgxpool.Pool
	Hub            *realtime.Hub
	Metrics        *metrics.Registry
	Logger         *slog.Logger
	RateLimit      int
	MaxAttempts    int
	AllowedOrigins []string
	// ExtraChecks are added to the readiness probe next to PostgreSQL.
	ExtraChecks map[string]func(context.Context) error
}

// BuildAPI assembles the HTTP handler of the API process. Tests use it to
// run the real API against a test database.
func BuildAPI(opts APIOptions) http.Handler {
	notifications := postgres.NewNotificationRepository(opts.Pool)
	batches := postgres.NewBatchRepository(opts.Pool)
	templates := postgres.NewTemplateRepository(opts.Pool)
	metricsRepo := postgres.NewMetricsRepository(opts.Pool)

	checks := map[string]func(context.Context) error{"postgres": opts.Pool.Ping}
	for name, check := range opts.ExtraChecks {
		checks[name] = check
	}

	cfg := httpapi.Config{
		Notifications: service.NewNotificationService(notifications, batches, templates,
			service.WithMaxAttempts(opts.MaxAttempts)),
		Templates:       service.NewTemplateService(templates),
		Metrics:         service.NewMetricsService(metricsRepo, opts.RateLimit),
		Hub:             opts.Hub,
		ReadinessChecks: checks,
		Logger:          opts.Logger,
		AllowedOrigins:  opts.AllowedOrigins,
		OpenAPISpec:     api.Spec,
	}
	if opts.Metrics != nil {
		cfg.PrometheusHandler = opts.Metrics.Handler()
		cfg.HTTPObserver = opts.Metrics
	}
	return httpapi.NewRouter(cfg)
}

// EngineOptions configures BuildEngine.
type EngineOptions struct {
	Pool     *pgxpool.Pool
	Sender   provider.Sender
	Limiter  delivery.Limiter
	Policy   delivery.RetryPolicy
	Config   delivery.Config
	Observer delivery.Observer
	Logger   *slog.Logger
	Clock    func() time.Time
}

// BuildEngine assembles the delivery engine of the worker process.
func BuildEngine(opts EngineOptions) *delivery.Engine {
	return delivery.NewEngine(opts.Config, delivery.Dependencies{
		Store:      postgres.NewDeliveryStore(opts.Pool),
		Heartbeats: postgres.NewHeartbeats(opts.Pool),
		Sender:     opts.Sender,
		Limiter:    opts.Limiter,
		Policy:     opts.Policy,
		Observer:   opts.Observer,
		Logger:     opts.Logger,
		Tracer:     otel.Tracer("github.com/RamizUnal/notification-system/delivery"),
		Clock:      opts.Clock,
	})
}

// queueDepthReader adapts the metrics repository for the Prometheus collector.
func queueDepthReader(pool *pgxpool.Pool) func(context.Context) ([]metrics.QueueSample, error) {
	repo := postgres.NewMetricsRepository(pool)
	return func(ctx context.Context) ([]metrics.QueueSample, error) {
		depth, err := repo.QueueDepth(ctx, time.Now())
		if err != nil {
			return nil, err
		}
		samples := make([]metrics.QueueSample, len(depth))
		for i, d := range depth {
			samples[i] = metrics.QueueSample{
				Channel: string(d.Channel), Priority: string(d.Priority), Status: string(d.Status), Count: d.Count,
			}
		}
		return samples, nil
	}
}
