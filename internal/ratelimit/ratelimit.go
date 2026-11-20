// Package ratelimit paces deliveries so that no channel exceeds the
// provider's throughput limit (100 messages per second per channel by
// default).
//
// Two implementations share one contract, Wait(ctx, channel):
//
//   - Local paces a single process with in-memory token buckets.
//   - Redis paces every worker process together with GCRA (the generic cell
//     rate algorithm), evaluated atomically inside Redis.
//
// With Burst set to 1 both enforce even spacing (one send every 10 ms at
// 100/s), so no rolling one-second window ever holds more than the limit.
// A larger burst trades that strictness for absorbing jitter.
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"

	"github.com/RamizUnal/notification-system/internal/domain"
)

// Config sets the pace of every channel.
type Config struct {
	// PerSecond is the sustained number of sends allowed per channel.
	PerSecond int
	// Burst is how many sends may happen back to back. Values below 1 mean 1.
	Burst int
}

func (c Config) burst() int { return max(1, c.Burst) }

// Local paces deliveries within one process. It is exact for a single
// worker, but N workers would together send N times the limit; deployments
// with more than one worker use Redis.
type Local struct {
	limiters map[domain.Channel]*rate.Limiter
}

// NewLocal returns a limiter with one token bucket per channel.
func NewLocal(cfg Config) *Local {
	l := &Local{limiters: make(map[domain.Channel]*rate.Limiter)}
	for _, ch := range domain.Channels() {
		l.limiters[ch] = rate.NewLimiter(rate.Limit(cfg.PerSecond), cfg.burst())
	}
	return l
}

// Wait blocks until one more message may be sent on ch.
func (l *Local) Wait(ctx context.Context, ch domain.Channel) error {
	lim, ok := l.limiters[ch]
	if !ok {
		return fmt.Errorf("ratelimit: no limit configured for channel %q", ch)
	}
	return lim.Wait(ctx)
}

// Redis paces deliveries across all worker processes that share the Redis
// instance. If Redis is unreachable, Wait fails and the engine pauses the
// channel instead of sending blind: exceeding the provider's limit would get
// messages throttled or rejected, while pausing only delays them.
type Redis struct {
	limiter *redis_rate.Limiter
	limit   redis_rate.Limit
	prefix  string
}

// NewRedis returns a limiter backed by client.
func NewRedis(client *redis.Client, cfg Config) *Redis {
	return &Redis{
		limiter: redis_rate.NewLimiter(client),
		limit:   redis_rate.Limit{Rate: cfg.PerSecond, Burst: cfg.burst(), Period: time.Second},
		prefix:  "notifications:channel:",
	}
}

// Wait blocks until one more message may be sent on ch.
func (r *Redis) Wait(ctx context.Context, ch domain.Channel) error {
	key := r.prefix + string(ch)
	for {
		res, err := r.limiter.Allow(ctx, key, r.limit)
		if err != nil {
			return fmt.Errorf("ratelimit: %w", err)
		}
		if res.Allowed > 0 {
			return nil
		}
		timer := time.NewTimer(res.RetryAfter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
