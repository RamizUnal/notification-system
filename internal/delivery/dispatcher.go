package delivery

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/RamizUnal/notification-system/internal/domain"
)

// DispatcherConfig tunes a Dispatcher.
type DispatcherConfig struct {
	// Concurrency is the most deliveries of the channel in flight at once.
	Concurrency int
	// PollInterval is how long an idle dispatcher waits before looking for
	// due notifications again.
	PollInterval time.Duration
	// LeaseTTL is how long a claim stays valid. It must comfortably exceed
	// the provider send timeout.
	LeaseTTL time.Duration
	// ErrorBackoff is the pause after a failed claim.
	ErrorBackoff time.Duration
}

// Dispatcher feeds one channel's due notifications to a bounded pool of
// concurrent deliveries. Each channel has its own dispatcher, pool, rate
// limit and circuit breaker (bulkheads), so a slow or failing SMS provider
// never holds up email or push.
type Dispatcher struct {
	channel  domain.Channel
	store    Store
	limiter  Limiter
	breaker  *CircuitBreaker
	process  func(context.Context, Job)
	cfg      DispatcherConfig
	logger   *slog.Logger
	now      func() time.Time
	inFlight atomic.Int64
}

// NewDispatcher returns a dispatcher that hands claimed jobs to process.
func NewDispatcher(
	ch domain.Channel,
	store Store,
	limiter Limiter,
	breaker *CircuitBreaker,
	process func(context.Context, Job),
	cfg DispatcherConfig,
	logger *slog.Logger,
	now func() time.Time,
) *Dispatcher {
	cfg.Concurrency = max(1, cfg.Concurrency)
	return &Dispatcher{
		channel: ch, store: store, limiter: limiter, breaker: breaker, process: process,
		cfg: cfg, logger: logger.With(slog.String("channel", string(ch))), now: now,
	}
}

// InFlight is the number of deliveries currently running.
func (d *Dispatcher) InFlight() int { return int(d.inFlight.Load()) }

// Run claims and dispatches notifications until ctx is cancelled, then waits
// for the deliveries in flight to finish.
//
// It only claims as many notifications as it has free delivery slots (and as
// the circuit breaker admits), so a claim is sent within moments and the
// queue's priority order is respected at claim time.
func (d *Dispatcher) Run(ctx context.Context) error {
	slots := make(chan struct{}, d.cfg.Concurrency)
	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		free, ok := acquireSlots(ctx, slots)
		if !ok {
			return nil
		}

		want := free
		switch capacity, wait := d.breaker.Capacity(); {
		case capacity == 0:
			releaseSlots(slots, free)
			if !sleep(ctx, wait) {
				return nil
			}
			continue
		case capacity > 0:
			want = min(want, capacity)
		}

		jobs, err := d.store.Claim(ctx, d.channel, want, d.now(), d.cfg.LeaseTTL)
		if err != nil {
			releaseSlots(slots, free)
			if ctx.Err() != nil {
				return nil
			}
			d.logger.ErrorContext(ctx, "claiming notifications failed", slog.Any("error", err))
			if !sleep(ctx, d.cfg.ErrorBackoff) {
				return nil
			}
			continue
		}
		releaseSlots(slots, free-len(jobs))

		if !d.dispatch(ctx, jobs, slots, &wg) {
			if ctx.Err() != nil {
				return nil
			}
			if !sleep(ctx, d.cfg.ErrorBackoff) {
				return nil
			}
			continue
		}

		// Fewer jobs than asked for means the queue is drained for now.
		if len(jobs) < want && !sleep(ctx, d.cfg.PollInterval) {
			return nil
		}
	}
}

// dispatch paces the jobs through the rate limiter and starts one delivery
// per job. If the limiter fails (shutdown, or its backend is unavailable),
// the jobs not yet started go back to the queue and dispatch reports false.
func (d *Dispatcher) dispatch(ctx context.Context, jobs []Job, slots chan struct{}, wg *sync.WaitGroup) bool {
	for i, job := range jobs {
		if err := d.limiter.Wait(ctx, d.channel); err != nil {
			if ctx.Err() == nil {
				d.logger.ErrorContext(ctx, "rate limiter unavailable; pausing the channel", slog.Any("error", err))
			}
			d.releaseAll(jobs[i:])
			releaseSlots(slots, len(jobs)-i)
			return false
		}

		d.inFlight.Add(1)
		wg.Add(1)
		go func() {
			defer func() {
				d.inFlight.Add(-1)
				<-slots
				wg.Done()
			}()
			d.process(ctx, job)
		}()
	}
	return true
}

// releaseAll returns claimed jobs to the queue. It runs during shutdown too,
// so it uses its own deadline instead of the (possibly cancelled) run context.
func (d *Dispatcher) releaseAll(jobs []Job) {
	ctx, cancel := context.WithTimeout(context.Background(), recordTimeout)
	defer cancel()
	for _, job := range jobs {
		if err := d.store.Release(ctx, job, d.now()); err != nil {
			d.logger.WarnContext(ctx, "returning notification to the queue failed; lease recovery will requeue it",
				slog.String("notification_id", job.Notification.ID.String()), slog.Any("error", err))
		}
	}
}

// acquireSlots blocks until at least one delivery slot is free, then takes
// every other free slot as well. It reports false once ctx is cancelled.
func acquireSlots(ctx context.Context, slots chan struct{}) (int, bool) {
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return 0, false
	}
	n := 1
	for n < cap(slots) {
		select {
		case slots <- struct{}{}:
			n++
		default:
			return n, true
		}
	}
	return n, true
}

func releaseSlots(slots chan struct{}, n int) {
	for range n {
		<-slots
	}
}

// sleep waits for d or until ctx is cancelled, reporting false in the latter case.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
