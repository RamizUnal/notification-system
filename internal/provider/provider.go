// Package provider defines the contract between the delivery engine and the
// external services that actually deliver messages (SMS gateways, email
// services, push services). Implementations live in sub-packages.
package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/RamizUnal/notification-system/internal/domain"
)

// Message is one notification handed to a provider.
type Message struct {
	// NotificationID is sent to the provider as an idempotency key, so a
	// provider that supports it can drop the duplicate created when a worker
	// dies after a successful send but before recording it.
	NotificationID uuid.UUID
	Channel        domain.Channel
	Recipient      string
	Content        string
	CorrelationID  string
}

// Receipt is the provider's acknowledgement of an accepted message.
type Receipt struct {
	MessageID  string
	Status     string
	AcceptedAt time.Time
	StatusCode int
}

// Sender delivers messages to an external provider.
type Sender interface {
	Send(ctx context.Context, msg Message) (Receipt, error)
}

// Error is a failed delivery, classified so the engine can decide whether
// repeating the attempt can succeed.
type Error struct {
	// StatusCode is the provider's HTTP status, or 0 if no response arrived.
	StatusCode int
	// Retryable is true for transient failures: timeouts, network errors,
	// throttling (429) and server errors (5xx).
	Retryable bool
	// RetryAfter is the delay the provider asked for, if any.
	RetryAfter time.Duration
	// Message summarizes the failure for operators.
	Message string
	Err     error
}

func (e *Error) Error() string {
	switch {
	case e.StatusCode != 0 && e.Err != nil:
		return fmt.Sprintf("provider responded %d: %s: %v", e.StatusCode, e.Message, e.Err)
	case e.StatusCode != 0:
		return fmt.Sprintf("provider responded %d: %s", e.StatusCode, e.Message)
	case e.Err != nil:
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	default:
		return e.Message
	}
}

func (e *Error) Unwrap() error { return e.Err }

// IsRetryable reports whether a failed delivery may succeed if repeated.
// Errors that are not classified are treated as transient: retrying them is
// bounded by the attempt budget, while dropping a deliverable message is not
// recoverable.
func IsRetryable(err error) bool {
	var pe *Error
	if errors.As(err, &pe) {
		return pe.Retryable
	}
	return err != nil
}

// RetryAfter returns the delay the provider asked for, or 0.
func RetryAfter(err error) time.Duration {
	var pe *Error
	if errors.As(err, &pe) {
		return pe.RetryAfter
	}
	return 0
}

// StatusCode returns the provider's HTTP status, or 0 if none was received.
func StatusCode(err error) int {
	var pe *Error
	if errors.As(err, &pe) {
		return pe.StatusCode
	}
	return 0
}
