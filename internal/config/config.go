// Package config loads process configuration from environment variables,
// following the twelve-factor convention. Every setting has a sensible
// default except the ones that identify external systems.
package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Common settings shared by every process.
type Common struct {
	DatabaseURL string `env:"DATABASE_URL,required,notEmpty"`
	DBMaxConns  int32  `env:"DB_MAX_CONNS" envDefault:"20"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat   string `env:"LOG_FORMAT" envDefault:"json"`
	// OTLPEndpoint enables tracing, e.g. http://jaeger:4318/v1/traces.
	OTLPEndpoint     string  `env:"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"`
	TraceSampleRatio float64 `env:"OTEL_TRACES_SAMPLE_RATIO" envDefault:"1"`
	// RateLimitPerSecond is the per-channel delivery ceiling.
	RateLimitPerSecond int `env:"RATE_LIMIT_PER_SECOND" envDefault:"100"`
	// MaxAttempts is the delivery attempt budget of a notification.
	MaxAttempts int `env:"MAX_ATTEMPTS" envDefault:"5"`
}

// API configures the HTTP API process.
type API struct {
	Common
	HTTPAddr        string        `env:"HTTP_ADDR" envDefault:":8080"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"15s"`
	// AllowedOrigins may open the WebSocket cross-origin (Vite dev server).
	AllowedOrigins []string `env:"WS_ALLOWED_ORIGINS" envDefault:"localhost:*,127.0.0.1:*" envSeparator:","`
	// MigrateOnStart applies pending migrations before serving, which is
	// handy locally; Docker Compose runs a dedicated migrate job instead.
	MigrateOnStart bool `env:"MIGRATE_ON_START" envDefault:"false"`
}

// Worker configures the delivery worker process.
type Worker struct {
	Common
	// WorkerID names the worker in heartbeats and logs; defaults to the hostname.
	WorkerID string `env:"WORKER_ID"`
	// HTTPAddr serves the worker's health checks and Prometheus metrics.
	HTTPAddr        string        `env:"HTTP_ADDR" envDefault:":9090"`
	ProviderURL     string        `env:"PROVIDER_URL,required,notEmpty"`
	ProviderTimeout time.Duration `env:"PROVIDER_TIMEOUT" envDefault:"5s"`
	// RedisURL enables the shared rate limiter; without it each worker
	// paces itself, which is only correct for a single worker.
	RedisURL          string        `env:"REDIS_URL"`
	RateLimitBurst    int           `env:"RATE_LIMIT_BURST" envDefault:"1"`
	Concurrency       int           `env:"WORKER_CONCURRENCY" envDefault:"20"`
	PollInterval      time.Duration `env:"POLL_INTERVAL" envDefault:"250ms"`
	LeaseTTL          time.Duration `env:"LEASE_TTL" envDefault:"30s"`
	RetryBaseDelay    time.Duration `env:"RETRY_BASE_DELAY" envDefault:"2s"`
	RetryMaxDelay     time.Duration `env:"RETRY_MAX_DELAY" envDefault:"5m"`
	BreakerThreshold  int           `env:"BREAKER_THRESHOLD" envDefault:"5"`
	BreakerCooldown   time.Duration `env:"BREAKER_COOLDOWN" envDefault:"15s"`
	ReaperInterval    time.Duration `env:"LEASE_RECOVERY_INTERVAL" envDefault:"10s"`
	HeartbeatInterval time.Duration `env:"HEARTBEAT_INTERVAL" envDefault:"5s"`
	ShutdownTimeout   time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"30s"`
}

// LoadAPI reads the API configuration from the environment.
func LoadAPI() (API, error) {
	cfg, err := env.ParseAs[API]()
	if err != nil {
		return API{}, fmt.Errorf("read configuration: %w", err)
	}
	return cfg, cfg.Common.validate()
}

// LoadWorker reads the worker configuration from the environment.
func LoadWorker() (Worker, error) {
	cfg, err := env.ParseAs[Worker]()
	if err != nil {
		return Worker{}, fmt.Errorf("read configuration: %w", err)
	}
	return cfg, cfg.validate()
}

func (c Common) validate() error {
	var errs []error
	if c.RateLimitPerSecond < 1 {
		errs = append(errs, errors.New("RATE_LIMIT_PER_SECOND must be at least 1"))
	}
	if c.MaxAttempts < 1 {
		errs = append(errs, errors.New("MAX_ATTEMPTS must be at least 1"))
	}
	if c.TraceSampleRatio < 0 || c.TraceSampleRatio > 1 {
		errs = append(errs, errors.New("OTEL_TRACES_SAMPLE_RATIO must be between 0 and 1"))
	}
	return errors.Join(errs...)
}

func (c Worker) validate() error {
	errs := []error{c.Common.validate()}
	if c.Concurrency < 1 {
		errs = append(errs, errors.New("WORKER_CONCURRENCY must be at least 1"))
	}
	if c.LeaseTTL < 2*c.ProviderTimeout {
		errs = append(errs, fmt.Errorf("LEASE_TTL (%s) must be at least twice PROVIDER_TIMEOUT (%s), "+
			"or slow sends would lose their lease", c.LeaseTTL, c.ProviderTimeout))
	}
	if c.RetryBaseDelay <= 0 || c.RetryMaxDelay < c.RetryBaseDelay {
		errs = append(errs, errors.New("RETRY_BASE_DELAY must be positive and at most RETRY_MAX_DELAY"))
	}
	if c.BreakerThreshold < 1 {
		errs = append(errs, errors.New("BREAKER_THRESHOLD must be at least 1"))
	}
	return errors.Join(errs...)
}
