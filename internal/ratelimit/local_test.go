package ratelimit_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/ratelimit"
)

// The tests run inside a synctest bubble: time is virtual, so pacing 100
// sends per second is checked exactly and instantly.

func TestLocalEnforcesTheRatePerChannel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := ratelimit.NewLocal(ratelimit.Config{PerSecond: 100, Burst: 1})
		ctx := context.Background()

		start := time.Now()
		for range 101 {
			require.NoError(t, l.Wait(ctx, domain.ChannelSMS))
		}

		// The first send is immediate, the next 100 are spaced 10 ms apart.
		assert.Equal(t, time.Second, time.Since(start))
	})
}

func TestLocalChannelsDoNotShareABudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := ratelimit.NewLocal(ratelimit.Config{PerSecond: 100, Burst: 1})
		ctx := context.Background()
		for range 50 {
			require.NoError(t, l.Wait(ctx, domain.ChannelSMS))
		}

		start := time.Now()
		require.NoError(t, l.Wait(ctx, domain.ChannelEmail))
		assert.Zero(t, time.Since(start), "a busy SMS channel must not slow email down")
	})
}

func TestLocalWaitHonoursCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := ratelimit.NewLocal(ratelimit.Config{PerSecond: 1, Burst: 1})
		require.NoError(t, l.Wait(context.Background(), domain.ChannelPush))

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		assert.Error(t, l.Wait(ctx, domain.ChannelPush), "the next slot is a second away")
	})
}

func TestLocalRejectsUnknownChannels(t *testing.T) {
	t.Parallel()
	l := ratelimit.NewLocal(ratelimit.Config{PerSecond: 100})
	assert.Error(t, l.Wait(context.Background(), "fax"))
}
