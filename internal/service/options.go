package service

import (
	"bytes"
	"slices"
	"time"

	"github.com/google/uuid"
)

// DefaultMaxAttempts is how many delivery attempts a notification gets
// before it is marked failed.
const DefaultMaxAttempts = 5

// settings holds the collaborators every service shares. Tests replace the
// clock and the ID generator to make results deterministic.
type settings struct {
	now         func() time.Time
	newID       func() uuid.UUID
	maxAttempts int
}

func newSettings(opts []Option) settings {
	s := settings{
		now:         time.Now,
		newID:       func() uuid.UUID { return uuid.Must(uuid.NewV7()) },
		maxAttempts: DefaultMaxAttempts,
	}
	for _, opt := range opts {
		opt(&s)
	}
	return s
}

// Option configures a service.
type Option func(*settings)

// WithClock replaces time.Now.
func WithClock(now func() time.Time) Option {
	return func(s *settings) { s.now = now }
}

// WithIDGenerator replaces the UUIDv7 generator.
func WithIDGenerator(newID func() uuid.UUID) Option {
	return func(s *settings) { s.newID = newID }
}

// WithMaxAttempts sets how many delivery attempts a notification gets, and
// how many more a manual retry grants.
func WithMaxAttempts(n int) Option {
	return func(s *settings) {
		if n > 0 {
			s.maxAttempts = n
		}
	}
}

// newIDs returns n fresh IDs in ascending order. UUIDv7 values already sort
// by creation time; sorting removes any doubt for IDs minted within the same
// millisecond, so ordering a batch by ID reproduces the request order.
func (s settings) newIDs(n int) []uuid.UUID {
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = s.newID()
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	return ids
}
