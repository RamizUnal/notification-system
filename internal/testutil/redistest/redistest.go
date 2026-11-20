// Package redistest gives integration tests a Redis server.
//
// The server comes from TEST_REDIS_ADDR when it is set and from a
// testcontainers-managed redis:8-alpine container otherwise. Each call
// flushes the database, so tests that use Redis must not run in parallel.
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
	addr    string
	initErr error
)

// New returns a client to an empty Redis database.
func New(t testing.TB) *redis.Client {
	t.Helper()
	once.Do(func() { addr, initErr = start() })
	if initErr != nil {
		t.Fatalf("redistest: %v", initErr)
	}

	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("redistest: flush: %v", err)
	}
	return client
}

func start() (string, error) {
	if a := os.Getenv("TEST_REDIS_ADDR"); a != "" {
		return a, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	ctr, err := tcredis.Run(ctx, "redis:8-alpine")
	if err != nil {
		return "", fmt.Errorf("start redis container (is Docker running?): %w", err)
	}
	endpoint, err := ctr.Endpoint(ctx, "")
	if err != nil {
		return "", fmt.Errorf("redis endpoint: %w", err)
	}
	return endpoint, nil
}
