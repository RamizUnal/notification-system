package domain

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
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

// KeyConflict is a batch item whose idempotency key already belongs to an
// existing notification.
type KeyConflict struct {
	Index      int
	Key        string
	ExistingID uuid.UUID
}

// IdempotencyConflictError reports batch items whose idempotency keys are
// already taken. Batches are atomic, so nothing of the batch was stored.
// It matches ErrIdempotencyConflict with errors.Is.
type IdempotencyConflictError struct {
	Conflicts []KeyConflict
}

func (e *IdempotencyConflictError) Error() string {
	return fmt.Sprintf("%d batch item(s) reuse idempotency keys of existing notifications", len(e.Conflicts))
}

// Is makes errors.Is(err, ErrIdempotencyConflict) true.
func (e *IdempotencyConflictError) Is(target error) bool { return target == ErrIdempotencyConflict }
