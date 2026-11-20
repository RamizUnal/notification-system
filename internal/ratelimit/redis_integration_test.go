//go:build integration

package ratelimit_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/ratelimit"
	"github.com/RamizUnal/notification-system/internal/testutil/redistest"
)

// TestRedisLimitIsSharedByAllWorkers simulates three worker processes, each
// with its own limiter and several goroutines, hammering one channel. The
// limit must hold for all of them together: no rolling one-second window may
// contain more sends than the configured rate.
func TestRedisLimitIsSharedByAllWorkers(t *testing.T) {
	client := redistest.New(t)
	const perSecond = 50
	cfg := ratelimit.Config{PerSecond: perSecond, Burst: 1}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var (
		mu    sync.Mutex
		sends []time.Time
		wg    sync.WaitGroup
	)
	for range 3 { // workers
		limiter := ratelimit.NewRedis(client, cfg)
		for range 4 { // goroutines per worker
			wg.Go(func() {
				for {
					if err := limiter.Wait(ctx, domain.ChannelSMS); err != nil {
						return
					}
					mu.Lock()
					sends = append(sends, time.Now())
					mu.Unlock()
				}
			})
		}
	}
	wg.Wait()

	require.NotEmpty(t, sends)
	maxInWindow := 0
	for i := range sends {
		j := i
		for j < len(sends) && sends[j].Sub(sends[i]) < time.Second {
			j++
		}
		maxInWindow = max(maxInWindow, j-i)
	}
	// One extra send is allowed for clock jitter between processes.
	assert.LessOrEqual(t, maxInWindow, perSecond+1, "no rolling second may exceed the limit")
	assert.GreaterOrEqual(t, len(sends), 2*perSecond, "the limit is a ceiling, not a stall")
}

func TestRedisChannelsHaveSeparateBudgets(t *testing.T) {
	client := redistest.New(t)
	limiter := ratelimit.NewRedis(client, ratelimit.Config{PerSecond: 1, Burst: 1})
	ctx := context.Background()

	require.NoError(t, limiter.Wait(ctx, domain.ChannelSMS))
	start := time.Now()
	require.NoError(t, limiter.Wait(ctx, domain.ChannelEmail))
	assert.Less(t, time.Since(start), 200*time.Millisecond)
}

func TestRedisWaitFailsWhenRedisIsDown(t *testing.T) {
	client := redistest.New(t)
	limiter := ratelimit.NewRedis(client, ratelimit.Config{PerSecond: 100})
	require.NoError(t, client.Close())

	err := limiter.Wait(context.Background(), domain.ChannelSMS)
	assert.Error(t, err, "the engine must pause rather than send unpaced")
}
