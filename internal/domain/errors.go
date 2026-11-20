package domain

import (
	"errors"
	"fmt"
)

// Sentinel errors describe business outcomes, not technical failures. Lower
// layers wrap them with context; transports translate them into responses with
// errors.Is, so no layer needs to know another layer's concrete error types.
var (
	// ErrNotFound means the requested resource does not exist.
	ErrNotFound = errors.New("not found")
	// ErrNotCancellable means the notification already left the waiting
	// states (it is being delivered or has finished) and cannot be cancelled.
	ErrNotCancellable = errors.New("notification can no longer be cancelled")
	// ErrNotRetryable means only failed notifications can be retried manually.
	ErrNotRetryable = errors.New("only failed notifications can be retried")
	// ErrIdempotencyConflict means an idempotency key was reused with a
	// different payload than the request that first used it.
	ErrIdempotencyConflict = errors.New("idempotency key already used with a different payload")
	// ErrTemplateNameTaken means another template already uses the name.
	ErrTemplateNameTaken = errors.New("template name already in use")
	// ErrLeaseLost means a worker's claim on a notification expired or was
	// taken over, so the worker must not record an outcome for it.
	ErrLeaseLost = errors.New("delivery lease lost")
)

// NotFoundError reports that a specific resource does not exist.
// It matches ErrNotFound with errors.Is.
type NotFoundError struct {
	Resource string
	ID       string
}

// NotFound returns an error reporting that resource id does not exist.
func NotFound(resource, id string) error {
	return &NotFoundError{Resource: resource, ID: id}
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s %s not found", e.Resource, e.ID)
}

// Is makes errors.Is(err, ErrNotFound) true for every NotFoundError.
func (e *NotFoundError) Is(target error) bool { return target == ErrNotFound }
