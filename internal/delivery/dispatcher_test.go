package delivery_test

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/delivery"
	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/ratelimit"
)

// Dispatcher tests run inside synctest bubbles: time is virtual and
// synctest.Wait returns once every goroutine is blocked, which makes
// concurrency and timing assertions exact instead of sleep-and-hope.

var dispatcherConfig = delivery.DispatcherConfig{
	Concurrency:  4,
	PollInterval: 200 * time.Millisecond,
	LeaseTTL:     30 * time.Second,
	ErrorBackoff: time.Second,
}

func newDispatcher(store delivery.Store, limiter delivery.Limiter, breaker *delivery.CircuitBreaker,
	process func(context.Context, delivery.Job),
) *delivery.Dispatcher {
	return delivery.NewDispatcher(domain.ChannelSMS, store, limiter, breaker, process,
		dispatcherConfig, slog.New(slog.DiscardHandler), time.Now)
}

func TestDispatcherBoundsConcurrencyAndDrainsTheQueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &fakeStore{queue: jobs(30)}
		var running, peak, done atomic.Int64
		process := func(context.Context, delivery.Job) {
			now := running.Add(1)
			for {
				old := peak.Load()
				if now <= old || peak.CompareAndSwap(old, now) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
			running.Add(-1)
			done.Add(1)
		}
		d := newDispatcher(store, unlimited{}, delivery.NewCircuitBreaker(3, time.Minute), process)

		ctx, cancel := context.WithCancel(context.Background())
		stopped := make(chan struct{})
		go func() {
			assert.NoError(t, d.Run(ctx))
			close(stopped)
		}()

		time.Sleep(time.Second)
		assert.EqualValues(t, 30, done.Load(), "every queued job is delivered")
		assert.EqualValues(t, 4, peak.Load(), "never more than Concurrency deliveries at once")

		_, _, limits := store.snapshot()
		for _, l := range limits {
			assert.LessOrEqual(t, l, 4, "claims never exceed the free slots")
		}

		cancel()
		synctest.Wait()
		select {
		case <-stopped:
		default:
			t.Fatal("Run did not return after cancellation")
		}
	})
}

func TestDispatcherPacesSendsThroughTheRateLimiter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &fakeStore{queue: jobs(50)}
		var done atomic.Int64
		limiter := ratelimit.NewLocal(ratelimit.Config{PerSecond: 10, Burst: 1})
		d := newDispatcher(store, limiter, delivery.NewCircuitBreaker(3, time.Minute),
			func(context.Context, delivery.Job) { done.Add(1) })

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { _ = d.Run(ctx) }()

		// One send immediately, then one every 100 ms.
		time.Sleep(950 * time.Millisecond)
		synctest.Wait()
		assert.EqualValues(t, 10, done.Load())

		time.Sleep(time.Second)
		synctest.Wait()
		assert.EqualValues(t, 20, done.Load())
	})
}

func TestDispatcherHoldsBackWhileTheCircuitIsOpen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &fakeStore{queue: jobs(10)}
		breaker := delivery.NewCircuitBreaker(1, 5*time.Second)
		breaker.Failure() // open
		d := newDispatcher(store, unlimited{}, breaker, func(context.Context, delivery.Job) {})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { _ = d.Run(ctx) }()

		time.Sleep(4 * time.Second)
		synctest.Wait()
		_, _, limits := store.snapshot()
		assert.Empty(t, limits, "nothing is claimed while the circuit is open")

		time.Sleep(2 * time.Second)
		synctest.Wait()
		_, _, limits = store.snapshot()
		require.NotEmpty(t, limits)
		assert.Equal(t, 1, limits[0], "a half-open circuit claims a single probe")
	})
}

func TestDispatcherWaitsForDeliveriesInFlightOnShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &fakeStore{queue: jobs(2)}
		release := make(chan struct{})
		var finished atomic.Int64
		d := newDispatcher(store, unlimited{}, delivery.NewCircuitBreaker(3, time.Minute),
			func(context.Context, delivery.Job) {
				<-release
				finished.Add(1)
			})

		ctx, cancel := context.WithCancel(context.Background())
		stopped := make(chan struct{})
		go func() {
			_ = d.Run(ctx)
			close(stopped)
		}()
		synctest.Wait()
		require.Equal(t, 2, d.InFlight())

		cancel()
		synctest.Wait()
		select {
		case <-stopped:
			t.Fatal("Run returned while deliveries were still in flight")
		default:
		}

		close(release)
		synctest.Wait()
		<-stopped
		assert.EqualValues(t, 2, finished.Load())
		assert.Zero(t, d.InFlight())
	})
}

func TestDispatcherReturnsClaimsWhenTheLimiterFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &fakeStore{queue: jobs(3)}
		var processed sync.Map
		d := newDispatcher(store, &failingLimiter{allowance: 1}, delivery.NewCircuitBreaker(3, time.Minute),
			func(_ context.Context, j delivery.Job) { processed.Store(j.Notification.ID, true) })

		ctx, cancel := context.WithCancel(context.Background())
		go func() { _ = d.Run(ctx) }()
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		cancel()
		synctest.Wait()

		_, released, _ := store.snapshot()
		assert.Len(t, released, 2, "jobs that could not be paced go back to the queue")
		for _, r := range released {
			_, wasProcessed := processed.Load(r.Job.Notification.ID)
			assert.False(t, wasProcessed)
		}
	})
}

func TestEngineRecoversLeasesAndReportsHeartbeats(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &fakeStore{}
		hb := &fakeHeartbeats{}
		engine := delivery.NewEngine(delivery.Config{
			WorkerID: "worker-1", Concurrency: 2, PollInterval: time.Second, LeaseTTL: 30 * time.Second,
			SendTimeout: 5 * time.Second, ReaperInterval: 5 * time.Second, HeartbeatInterval: 2 * time.Second,
			BreakerThreshold: 3, BreakerCooldown: time.Minute,
		}, delivery.Dependencies{
			Store: store, Heartbeats: hb, Sender: senderFunc(accepted), Limiter: unlimited{},
			Policy: fixedPolicy(time.Second),
		})

		ctx, cancel := context.WithCancel(context.Background())
		stopped := make(chan struct{})
		go func() {
			assert.NoError(t, engine.Run(ctx))
			close(stopped)
		}()

		time.Sleep(11 * time.Second)
		synctest.Wait()
		cancel()
		<-stopped

		store.mu.Lock()
		assert.Equal(t, 2, store.recoveries, "lease recovery runs every ReaperInterval")
		store.mu.Unlock()

		hb.mu.Lock()
		defer hb.mu.Unlock()
		assert.Equal(t, 6, hb.beats, "one heartbeat at start and one per interval")
		assert.Equal(t, []string{"worker-1"}, hb.removed, "the heartbeat is withdrawn on shutdown")
		assert.Len(t, hb.last.Circuits, 3)
	})
}

type fakeHeartbeats struct {
	mu      sync.Mutex
	beats   int
	last    delivery.Heartbeat
	removed []string
}

func (h *fakeHeartbeats) Beat(_ context.Context, hb delivery.Heartbeat) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.beats++
	h.last = hb
	return nil
}

func (h *fakeHeartbeats) Remove(_ context.Context, id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removed = append(h.removed, id)
	return nil
}
