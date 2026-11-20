package domain

import (
	"time"

	"github.com/google/uuid"
)

// AttemptOutcome classifies the result of one delivery attempt.
type AttemptOutcome string

// Attempt outcomes.
const (
	// OutcomeSuccess means the provider accepted the message.
	OutcomeSuccess AttemptOutcome = "success"
	// OutcomeRetryableFailure is a transient failure (timeout, 5xx, 429) that
	// may succeed if the attempt is repeated later.
	OutcomeRetryableFailure AttemptOutcome = "retryable_failure"
	// OutcomePermanentFailure means repeating the attempt cannot succeed,
	// for example because the provider rejected the request as invalid.
	OutcomePermanentFailure AttemptOutcome = "permanent_failure"
)

// DeliveryAttempt records one call to the provider, successful or not. The
// attempt log is the audit trail behind every status change and the source of
// the latency and success-rate metrics.
type DeliveryAttempt struct {
	ID                int64
	NotificationID    uuid.UUID
	Number            int
	Channel           Channel
	Outcome           AttemptOutcome
	StatusCode        int // HTTP status from the provider; 0 when no response arrived
	Error             string
	ProviderMessageID string
	StartedAt         time.Time
	FinishedAt        time.Time
}

// Duration is how long the provider call took.
func (a DeliveryAttempt) Duration() time.Duration {
	return a.FinishedAt.Sub(a.StartedAt)
}
