package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/config"
)

func TestLoadWorkerAppliesDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("PROVIDER_URL", "https://webhook.site/abc")

	cfg, err := config.LoadWorker()

	require.NoError(t, err)
	assert.Equal(t, 100, cfg.RateLimitPerSecond)
	assert.Equal(t, 20, cfg.Concurrency)
	assert.Equal(t, 30*time.Second, cfg.LeaseTTL)
	assert.Equal(t, 5, cfg.MaxAttempts)
}

func TestLoadWorkerReportsEveryProblem(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("PROVIDER_URL", "https://webhook.site/abc")
	t.Setenv("RATE_LIMIT_PER_SECOND", "0")
	t.Setenv("WORKER_CONCURRENCY", "0")
	t.Setenv("LEASE_TTL", "5s")
	t.Setenv("PROVIDER_TIMEOUT", "5s")

	_, err := config.LoadWorker()

	require.Error(t, err)
	for _, want := range []string{"RATE_LIMIT_PER_SECOND", "WORKER_CONCURRENCY", "LEASE_TTL"} {
		assert.Contains(t, err.Error(), want)
	}
}

func TestLoadAPIRequiresADatabase(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	_, err := config.LoadAPI()
	assert.ErrorContains(t, err, "DATABASE_URL")
}
