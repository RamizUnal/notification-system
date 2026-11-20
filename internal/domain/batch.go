package domain

import (
	"time"

	"github.com/google/uuid"
)

// MaxBatchSize is the largest number of notifications one batch may carry.
const MaxBatchSize = 1000

// Batch groups notifications created by a single request so their progress
// can be tracked and cancelled together.
type Batch struct {
	ID             uuid.UUID
	Size           int
	IdempotencyKey string
	Fingerprint    string
	CreatedAt      time.Time
}

// BatchFingerprint identifies the payload of a whole batch from the
// fingerprints of its items, in order.
func BatchFingerprint(itemFingerprints []string) string {
	return fingerprint(itemFingerprints...)
}

// BatchSummary is a batch with the number of its notifications in each status.
type BatchSummary struct {
	Batch
	Counts map[Status]int
}

// Pending is the number of notifications that have not reached a final status.
func (s BatchSummary) Pending() int {
	pending := 0
	for status, n := range s.Counts {
		if !status.IsFinal() {
			pending += n
		}
	}
	return pending
}

// Done reports whether every notification of the batch reached a final status.
func (s BatchSummary) Done() bool { return s.Pending() == 0 }
