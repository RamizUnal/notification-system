package delivery_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/RamizUnal/notification-system/internal/delivery"
)

type manualClock struct {
	mu sync.Mutex
	t  time.Time
}

func newManualClock() *manualClock {
	return &manualClock{t: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)}
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *manualClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestCircuitBreakerLifecycle(t *testing.T) {
	t.Parallel()
	clk := newManualClock()
	var transitions []string
	b := delivery.NewCircuitBreaker(3, 30*time.Second,
		delivery.WithBreakerClock(clk.Now),
		delivery.OnStateChange(func(from, to delivery.CircuitState) {
			transitions = append(transitions, from.String()+"->"+to.String())
		}))

	// Closed: failures below the threshold, and a success resets the count.
	b.Failure()
	b.Failure()
	b.Success()
	b.Failure()
	b.Failure()
	ok, _ := b.Allow()
	assert.True(t, ok, "only consecutive failures count")
	n, _ := b.Capacity()
	assert.Equal(t, -1, n, "closed means no limit")

	// The third consecutive failure opens the circuit.
	b.Failure()
	ok, wait := b.Allow()
	assert.False(t, ok)
	assert.Equal(t, 30*time.Second, wait)
	assert.Equal(t, delivery.CircuitStatus{State: delivery.CircuitOpen, RetryAt: clk.Now().Add(30 * time.Second)}, b.Status())

	clk.Advance(10 * time.Second)
	n, wait = b.Capacity()
	assert.Zero(t, n)
	assert.Equal(t, 20*time.Second, wait)

	// After the cooldown exactly one probe is admitted.
	clk.Advance(20 * time.Second)
	n, _ = b.Capacity()
	assert.Equal(t, 1, n)
	ok, _ = b.Allow()
	assert.True(t, ok, "the probe")
	ok, wait = b.Allow()
	assert.False(t, ok, "only one probe at a time")
	assert.Equal(t, time.Second, wait)

	// A failed probe reopens the circuit for a full cooldown.
	b.Failure()
	ok, wait = b.Allow()
	assert.False(t, ok)
	assert.Equal(t, 30*time.Second, wait)

	// A successful probe closes it.
	clk.Advance(30 * time.Second)
	ok, _ = b.Allow()
	assert.True(t, ok)
	b.Success()
	assert.Equal(t, delivery.CircuitClosed, b.Status().State)

	assert.Equal(t, []string{
		"closed->open", "open->half_open", "half_open->open", "open->half_open", "half_open->closed",
	}, transitions)
}

func TestCircuitBreakerIgnoresStragglersWhileOpen(t *testing.T) {
	t.Parallel()
	clk := newManualClock()
	b := delivery.NewCircuitBreaker(1, time.Minute, delivery.WithBreakerClock(clk.Now))

	b.Failure()
	clk.Advance(50 * time.Second)
	b.Success() // started before the circuit opened
	b.Failure() // must not extend the cooldown

	_, wait := b.Allow()
	assert.Equal(t, 10*time.Second, wait)
	assert.Equal(t, delivery.CircuitOpen, b.Status().State)
}

func TestCircuitBreakerIsSafeForConcurrentUse(t *testing.T) {
	t.Parallel()
	b := delivery.NewCircuitBreaker(5, time.Millisecond)

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for j := range 1000 {
				if ok, _ := b.Allow(); ok && (i+j)%3 == 0 {
					b.Failure()
				} else {
					b.Success()
				}
				_ = b.Status()
				_, _ = b.Capacity()
			}
		})
	}
	wg.Wait()
}

func TestExponentialBackoff(t *testing.T) {
	t.Parallel()

	low := delivery.ExponentialBackoff{Base: 2 * time.Second, Max: time.Minute, Rand: func() float64 { return 0 }}
	high := delivery.ExponentialBackoff{Base: 2 * time.Second, Max: time.Minute, Rand: func() float64 { return 0.999999 }}

	tests := []struct {
		attempt int
		ceiling time.Duration
	}{
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{5, 32 * time.Second},
		{6, time.Minute}, // 64s capped at Max
		{60, time.Minute},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.ceiling/2, low.Delay(tc.attempt), "attempt %d lower bound", tc.attempt)
		assert.InDelta(t, float64(tc.ceiling), float64(high.Delay(tc.attempt)), float64(time.Millisecond),
			"attempt %d upper bound", tc.attempt)
	}

	// The default random source stays inside the same bounds.
	b := delivery.ExponentialBackoff{Base: time.Second, Max: time.Minute}
	for range 1000 {
		d := b.Delay(3)
		assert.GreaterOrEqual(t, d, 2*time.Second)
		assert.LessOrEqual(t, d, 4*time.Second)
	}
}
