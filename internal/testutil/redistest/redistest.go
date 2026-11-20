// Package redistest gives integration tests a Redis server.
//
// The server comes from TEST_REDIS_URL (e.g. redis://redis:6379/15) when it
// is set and from a testcontainers-managed redis:8-alpine container
// otherwise. Each call flushes the database, so tests that use Redis must
// not run in parallel, and TEST_REDIS_URL should name a database nothing
// else uses.
package redistest

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

var (
	once    sync.Once
	options *redis.Options
	initErr error
)

// New returns a client to an empty Redis database.
func New(tb testing.TB) *redis.Client {
	tb.Helper()
	once.Do(func() { options, initErr = start() })
	if initErr != nil {
		tb.Fatalf("redistest: %v", initErr)
	}

	client := redis.NewClient(options)
	tb.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.FlushDB(ctx).Err(); err != nil {
		tb.Fatalf("redistest: flush: %v", err)
	}
	return client
}

func start() (*redis.Options, error) {
	if u := os.Getenv("TEST_REDIS_URL"); u != "" {
		return redis.ParseURL(u)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	ctr, err := tcredis.Run(ctx, "redis:8-alpine")
	if err != nil {
		return nil, fmt.Errorf("start redis container (is Docker running?): %w", err)
	}
	endpoint, err := ctr.Endpoint(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("redis endpoint: %w", err)
	}
	return &redis.Options{Addr: endpoint}, nil
}
