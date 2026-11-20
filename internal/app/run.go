package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/RamizUnal/notification-system/internal/config"
	"github.com/RamizUnal/notification-system/internal/delivery"
	"github.com/RamizUnal/notification-system/internal/observability"
	"github.com/RamizUnal/notification-system/internal/observability/metrics"
	"github.com/RamizUnal/notification-system/internal/provider/webhook"
	"github.com/RamizUnal/notification-system/internal/ratelimit"
	"github.com/RamizUnal/notification-system/internal/realtime"
	"github.com/RamizUnal/notification-system/internal/storage/postgres"
)

// NewLogger builds the process logger from the common configuration.
func NewLogger(cfg config.Common) (*slog.Logger, error) {
	level, err := observability.ParseLevel(cfg.LogLevel)
	if err != nil {
		return nil, err
	}
	return observability.NewLogger(os.Stdout, level, cfg.LogFormat), nil
}

// RunAPI serves the API until ctx is cancelled, then shuts down gracefully:
// it stops accepting connections and lets requests in flight finish.
func RunAPI(ctx context.Context, cfg config.API, logger *slog.Logger) error {
	shutdownTracing, err := observability.SetupTracing(ctx, "notification-api", cfg.OTLPEndpoint, cfg.TraceSampleRatio)
	if err != nil {
		return err
	}
	defer flushTraces(shutdownTracing, logger)

	if cfg.MigrateOnStart {
		version, err := postgres.MigrateUp(cfg.DatabaseURL)
		if err != nil {
			return err
		}
		logger.Info("database schema is up to date", slog.Uint64("version", uint64(version)))
	}

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL, postgres.PoolConfig{MaxConns: cfg.DBMaxConns})
	if err != nil {
		return err
	}
	defer pool.Close()

	registry := metrics.New()
	registry.Register(metrics.NewQueueDepthCollector(queueDepthReader(pool), logger))
	hub := realtime.NewHub()

	// Fan committed events out to WebSocket clients.
	listenerCtx, stopListener := context.WithCancel(context.WithoutCancel(ctx))
	defer stopListener()
	go func() { _ = postgres.NewListener(pool, logger).Listen(listenerCtx, hub.Publish) }()

	handler := BuildAPI(APIOptions{
		Pool:           pool,
		Hub:            hub,
		Metrics:        registry,
		Logger:         logger,
		RateLimit:      cfg.RateLimitPerSecond,
		MaxAttempts:    cfg.MaxAttempts,
		AllowedOrigins: cfg.AllowedOrigins,
	})
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           otelhttp.NewHandler(handler, "notification-api"),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No WriteTimeout: it would cut off long-lived WebSocket streams.
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	logger.Info("API listening", slog.String("addr", cfg.HTTPAddr))
	return serve(ctx, server, cfg.ShutdownTimeout, logger)
}

// RunWorker runs the delivery engine until ctx is cancelled, then stops
// claiming work and waits for deliveries in flight to finish.
func RunWorker(ctx context.Context, cfg config.Worker, logger *slog.Logger) error {
	shutdownTracing, err := observability.SetupTracing(ctx, "notification-worker", cfg.OTLPEndpoint, cfg.TraceSampleRatio)
	if err != nil {
		return err
	}
	defer flushTraces(shutdownTracing, logger)

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL, postgres.PoolConfig{MaxConns: cfg.DBMaxConns})
	if err != nil {
		return err
	}
	defer pool.Close()

	checks := map[string]func(context.Context) error{"postgres": pool.Ping}
	limiter, closeLimiter, err := newLimiter(ctx, cfg, checks, logger)
	if err != nil {
		return err
	}
	defer closeLimiter()

	sender, err := webhook.New(cfg.ProviderURL, webhook.WithHTTPClient(&http.Client{
		Timeout:   cfg.ProviderTimeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}))
	if err != nil {
		return err
	}

	workerID := cfg.WorkerID
	if workerID == "" {
		host, _ := os.Hostname()
		workerID = fmt.Sprintf("%s-%s", host, uuid.NewString()[:8])
	}
	registry := metrics.New()
	registry.Register(metrics.NewQueueDepthCollector(queueDepthReader(pool), logger))

	engine := BuildEngine(EngineOptions{
		Pool:    pool,
		Sender:  sender,
		Limiter: limiter,
		Policy:  delivery.ExponentialBackoff{Base: cfg.RetryBaseDelay, Max: cfg.RetryMaxDelay},
		Config: delivery.Config{
			WorkerID:          workerID,
			Concurrency:       cfg.Concurrency,
			PollInterval:      cfg.PollInterval,
			LeaseTTL:          cfg.LeaseTTL,
			SendTimeout:       cfg.ProviderTimeout,
			ReaperInterval:    cfg.ReaperInterval,
			HeartbeatInterval: cfg.HeartbeatInterval,
			BreakerThreshold:  cfg.BreakerThreshold,
			BreakerCooldown:   cfg.BreakerCooldown,
		},
		Observer: registry,
		Logger:   logger,
	})

	// The worker serves only operational endpoints: health and metrics.
	ops := http.NewServeMux()
	ops.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	ops.HandleFunc("GET /readyz", readinessHandler(checks))
	ops.Handle("GET /metrics", registry.Handler())
	opsServer := &http.Server{Addr: cfg.HTTPAddr, Handler: ops, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := opsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("worker ops server failed", slog.Any("error", err))
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = opsServer.Shutdown(shutdownCtx)
	}()

	logger.Info("worker started",
		slog.String("worker_id", workerID), slog.String("provider_url", cfg.ProviderURL),
		slog.Int("rate_limit_per_second", cfg.RateLimitPerSecond), slog.String("ops_addr", cfg.HTTPAddr))

	// Bound the drain on shutdown: deliveries still running when the
	// timeout expires are recovered by another worker once their lease ends.
	engineCtx, cancelEngine := context.WithCancel(context.Background())
	defer cancelEngine()
	done := make(chan error, 1)
	go func() { done <- engine.Run(engineCtx) }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
	}
	logger.Info("shutting down worker; waiting for deliveries in flight", slog.Duration("timeout", cfg.ShutdownTimeout))
	cancelEngine()
	select {
	case err := <-done:
		return err
	case <-time.After(cfg.ShutdownTimeout):
		return errors.New("deliveries did not finish before the shutdown timeout; their leases will be recovered")
	}
}

func newLimiter(ctx context.Context, cfg config.Worker, checks map[string]func(context.Context) error,
	logger *slog.Logger,
) (delivery.Limiter, func(), error) {
	limits := ratelimit.Config{PerSecond: cfg.RateLimitPerSecond, Burst: cfg.RateLimitBurst}
	if cfg.RedisURL == "" {
		logger.Warn("REDIS_URL is not set: rate limits are enforced per worker, so run a single worker")
		return ratelimit.NewLocal(limits), func() {}, nil
	}

	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	client := redis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("reach redis: %w", err)
	}
	checks["redis"] = func(ctx context.Context) error { return client.Ping(ctx).Err() }
	return ratelimit.NewRedis(client, limits), func() { _ = client.Close() }, nil
}

func readinessHandler(checks map[string]func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "application/json")
		for name, check := range checks {
			if err := check(ctx); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = fmt.Fprintf(w, `{"status":"unavailable","failed":%q}`, name)
				return
			}
		}
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}
}

func serve(ctx context.Context, server *http.Server, timeout time.Duration, logger *slog.Logger) error {
	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down; finishing requests in flight", slog.Duration("timeout", timeout))
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	return nil
}

func flushTraces(shutdown func(context.Context) error, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		logger.Warn("flushing traces failed", slog.Any("error", err))
	}
}
