package domain

import (
	"fmt"
	"slices"
)

// Status is a notification's position in its delivery lifecycle.
//
//	scheduled ─┐
//	queued ────┼─▶ processing ─▶ sent
//	retrying ──┘        │
//	    ▲               ├─▶ retrying (transient failure, attempts left)
//	    └───────────────┤
//	                    └─▶ failed ─▶ queued (manual retry)
//
// Any waiting status (scheduled, queued, retrying) can move to cancelled.
type Status string

// Lifecycle states.
const (
	// StatusScheduled waits for a future delivery time chosen by the client.
	StatusScheduled Status = "scheduled"
	// StatusQueued waits for a worker.
	StatusQueued Status = "queued"
	// StatusProcessing is leased by a worker that is delivering it right now.
	StatusProcessing Status = "processing"
	// StatusRetrying failed transiently and waits for its next attempt.
	StatusRetrying Status = "retrying"
	// StatusSent was accepted by the provider.
	StatusSent Status = "sent"
	// StatusFailed failed permanently or ran out of attempts (dead letter).
	StatusFailed Status = "failed"
	// StatusCancelled was cancelled by a client before delivery started.
	StatusCancelled Status = "cancelled"
)

// transitions is the lifecycle state machine. Storage enforces the same rules
// with conditional updates, so concurrent writers can never break them.
var transitions = map[Status][]Status{
	StatusScheduled:  {StatusProcessing, StatusCancelled},
	StatusQueued:     {StatusProcessing, StatusCancelled},
	StatusRetrying:   {StatusProcessing, StatusCancelled},
	StatusProcessing: {StatusSent, StatusRetrying, StatusFailed, StatusQueued},
	StatusFailed:     {StatusQueued},
}

// Statuses returns every status in lifecycle order.
func Statuses() []Status {
	return []Status{
		StatusScheduled, StatusQueued, StatusProcessing, StatusRetrying,
		StatusSent, StatusFailed, StatusCancelled,
	}
}

// WaitingStatuses returns the statuses of notifications that wait for delivery.
func WaitingStatuses() []Status {
	return []Status{StatusScheduled, StatusQueued, StatusRetrying}
}

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	return slices.Contains(Statuses(), s)
}

// IsWaiting reports whether the notification still waits for delivery and can
// therefore be cancelled.
func (s Status) IsWaiting() bool {
	return slices.Contains(WaitingStatuses(), s)
}

// IsFinal reports whether the engine will not touch the notification again on
// its own. A failed notification can still be retried manually.
func (s Status) IsFinal() bool {
	return s == StatusSent || s == StatusFailed || s == StatusCancelled
}

// CanTransitionTo reports whether moving from s to next is a legal step.
func (s Status) CanTransitionTo(next Status) bool {
	return slices.Contains(transitions[s], next)
}

// ParseStatus converts s into a Status, rejecting unknown values.
func ParseStatus(s string) (Status, error) {
	st := Status(s)
	if !st.Valid() {
		return "", fmt.Errorf("unknown status %q", s)
	}
	return st, nil
}
