package delivery

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/provider"
)

// Config tunes the engine.
type Config struct {
	WorkerID          string
	Concurrency       int           // deliveries in flight per channel
	PollInterval      time.Duration // idle wait between queue checks
	LeaseTTL          time.Duration // how long a claim stays valid
	SendTimeout       time.Duration // provider request timeout
	ReaperInterval    time.Duration // how often expired leases are recovered
	HeartbeatInterval time.Duration // how often the worker reports its status
	BreakerThreshold  int           // consecutive failures that open a circuit
	BreakerCooldown   time.Duration // how long an open circuit waits before probing
}

// Dependencies are the engine's collaborators.
type Dependencies struct {
	Store      Store
	Heartbeats Heartbeats
	Sender     provider.Sender
	Limiter    Limiter
	Policy     RetryPolicy
	Observer   Observer     // optional
	Logger     *slog.Logger // optional
	Tracer     trace.Tracer // optional
	Clock      func() time.Time
}

// Engine runs a dispatcher per channel plus the background maintenance that
// keeps delivery reliable: recovering expired leases and reporting worker
// status.
type Engine struct {
	cfg         Config
	store       Store
	heartbeats  Heartbeats
	breakers    map[domain.Channel]*CircuitBreaker
	dispatchers []*Dispatcher
	observer    Observer
	logger      *slog.Logger
	now         func() time.Time
	startedAt   time.Time
}

// NewEngine assembles the engine.
func NewEngine(cfg Config, deps Dependencies) *Engine {
	if deps.Clock == nil {
		deps.Clock = time.Now
	}
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.DiscardHandler)
	}
	if deps.Observer == nil {
		deps.Observer = NopObserver{}
	}
	logger := deps.Logger.With(slog.String("worker_id", cfg.WorkerID))

	e := &Engine{
		cfg:        cfg,
		store:      deps.Store,
		heartbeats: deps.Heartbeats,
		breakers:   make(map[domain.Channel]*CircuitBreaker),
		observer:   deps.Observer,
		logger:     logger,
		now:        deps.Clock,
		startedAt:  deps.Clock(),
	}

	for _, ch := range domain.Channels() {
		e.breakers[ch] = NewCircuitBreaker(cfg.BreakerThreshold, cfg.BreakerCooldown,
			WithBreakerClock(deps.Clock),
			OnStateChange(func(from, to CircuitState) {
				deps.Observer.CircuitChanged(ch, to)
				logger.Warn("circuit breaker changed state",
					slog.String("channel", string(ch)), slog.String("from", from.String()), slog.String("to", to.String()))
			}))
	}

	processor := NewProcessor(ProcessorConfig{
		Store:       deps.Store,
		Sender:      deps.Sender,
		Breakers:    e.breakers,
		Policy:      deps.Policy,
		Observer:    deps.Observer,
		Logger:      logger,
		Tracer:      deps.Tracer,
		Clock:       deps.Clock,
		SendTimeout: cfg.SendTimeout,
	})

	dcfg := DispatcherConfig{
		Concurrency:  cfg.Concurrency,
		PollInterval: cfg.PollInterval,
		LeaseTTL:     cfg.LeaseTTL,
		ErrorBackoff: time.Second,
	}
	for _, ch := range domain.Channels() {
		e.dispatchers = append(e.dispatchers,
			NewDispatcher(ch, deps.Store, deps.Limiter, e.breakers[ch], processor.Process, dcfg, logger, deps.Clock))
	}
	return e
}

// Run processes notifications until ctx is cancelled and the deliveries in
// flight have finished.
func (e *Engine) Run(ctx context.Context) error {
	e.logger.InfoContext(ctx, "delivery engine started",
		slog.Int("concurrency_per_channel", e.cfg.Concurrency), slog.Duration("lease_ttl", e.cfg.LeaseTTL))

	g, ctx := errgroup.WithContext(ctx)
	for _, d := range e.dispatchers {
		g.Go(func() error { return d.Run(ctx) })
	}
	g.Go(func() error { return e.recoverLeases(ctx) })
	if e.heartbeats != nil {
		g.Go(func() error { return e.reportStatus(ctx) })
	}
	err := g.Wait()
	e.logger.Info("delivery engine stopped")
	return err
}

// Status returns the worker's current status report.
func (e *Engine) Status() Heartbeat {
	hb := Heartbeat{
		WorkerID:  e.cfg.WorkerID,
		StartedAt: e.startedAt,
		SeenAt:    e.now(),
		Circuits:  make(map[domain.Channel]CircuitStatus, len(e.breakers)),
	}
	for ch, b := range e.breakers {
		hb.Circuits[ch] = b.Status()
	}
	for _, d := range e.dispatchers {
		hb.InFlight += d.InFlight()
	}
	return hb
}

// recoverLeases periodically requeues notifications whose worker died
// mid-delivery. Every worker runs it; the store's update is idempotent.
func (e *Engine) recoverLeases(ctx context.Context) error {
	ticker := time.NewTicker(e.cfg.ReaperInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			n, err := e.store.RecoverExpiredLeases(ctx, e.now())
			switch {
			case err != nil && ctx.Err() == nil:
				e.logger.ErrorContext(ctx, "recovering expired leases failed", slog.Any("error", err))
			case n > 0:
				e.observer.LeasesRecovered(n)
				e.logger.WarnContext(ctx, "recovered notifications from expired leases", slog.Int("count", n))
			}
		}
	}
}

// reportStatus publishes a heartbeat periodically and removes it on exit,
// so the API can show which workers are alive and how their circuits stand.
func (e *Engine) reportStatus(ctx context.Context) error {
	beat := func() {
		if err := e.heartbeats.Beat(ctx, e.Status()); err != nil && ctx.Err() == nil {
			e.logger.WarnContext(ctx, "publishing heartbeat failed", slog.Any("error", err))
		}
	}
	beat()

	ticker := time.NewTicker(e.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			removeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := e.heartbeats.Remove(removeCtx, e.cfg.WorkerID); err != nil {
				e.logger.Warn("removing heartbeat failed", slog.Any("error", err))
			}
			return nil
		case <-ticker.C:
			beat()
		}
	}
}
