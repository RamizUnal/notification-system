package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/domain"
)

func TestStatusTransitions(t *testing.T) {
	t.Parallel()

	allowed := map[domain.Status][]domain.Status{
		domain.StatusScheduled:  {domain.StatusProcessing, domain.StatusCancelled},
		domain.StatusQueued:     {domain.StatusProcessing, domain.StatusCancelled},
		domain.StatusRetrying:   {domain.StatusProcessing, domain.StatusCancelled},
		domain.StatusProcessing: {domain.StatusSent, domain.StatusRetrying, domain.StatusFailed, domain.StatusQueued},
		domain.StatusFailed:     {domain.StatusQueued},
		domain.StatusSent:       nil,
		domain.StatusCancelled:  nil,
	}

	// Check the full cross product so that adding a transition by accident
	// fails this test, not only removing one.
	for _, from := range domain.Statuses() {
		for _, to := range domain.Statuses() {
			want := false
			for _, ok := range allowed[from] {
				if ok == to {
					want = true
				}
			}
			assert.Equalf(t, want, from.CanTransitionTo(to), "%s -> %s", from, to)
		}
	}
}

func TestStatusClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status  domain.Status
		waiting bool
		final   bool
	}{
		{domain.StatusScheduled, true, false},
		{domain.StatusQueued, true, false},
		{domain.StatusRetrying, true, false},
		{domain.StatusProcessing, false, false},
		{domain.StatusSent, false, true},
		{domain.StatusFailed, false, true},
		{domain.StatusCancelled, false, true},
	}
	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.waiting, tc.status.IsWaiting(), "IsWaiting")
			assert.Equal(t, tc.final, tc.status.IsFinal(), "IsFinal")
		})
	}
}

func TestParseEnums(t *testing.T) {
	t.Parallel()

	s, err := domain.ParseStatus("retrying")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusRetrying, s)
	_, err = domain.ParseStatus("delivered")
	require.Error(t, err)

	c, err := domain.ParseChannel("email")
	require.NoError(t, err)
	assert.Equal(t, domain.ChannelEmail, c)
	_, err = domain.ParseChannel("fax")
	require.Error(t, err)

	p, err := domain.ParsePriority("low")
	require.NoError(t, err)
	assert.Equal(t, domain.PriorityLow, p)
	_, err = domain.ParsePriority("urgent")
	require.Error(t, err)
}

func TestPriorityRankOrdersHighFirst(t *testing.T) {
	t.Parallel()
	assert.Less(t, domain.PriorityHigh.Rank(), domain.PriorityNormal.Rank())
	assert.Less(t, domain.PriorityNormal.Rank(), domain.PriorityLow.Rank())
}
