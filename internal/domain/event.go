package domain

import (
	"time"

	"github.com/google/uuid"
)

// EventType names a state change that other components can react to.
type EventType string

// Event types published after a change has been committed.
const (
	// EventNotificationCreated: a single notification was accepted.
	EventNotificationCreated EventType = "notification.created"
	// EventNotificationUpdated: a notification changed status.
	EventNotificationUpdated EventType = "notification.updated"
	// EventBatchCreated: a batch of notifications was accepted. Items of a
	// batch do not publish individual creation events.
	EventBatchCreated EventType = "batch.created"
)

// Event describes a committed state change. Events are published in the same
// database transaction as the change itself, so subscribers never observe a
// change that was rolled back.
type Event struct {
	Type           EventType
	NotificationID uuid.UUID // zero for batch events
	BatchID        *uuid.UUID
	Channel        Channel
	Priority       Priority
	Status         Status
	Attempts       int
	LastError      string
	BatchSize      int // set for batch events only
	OccurredAt     time.Time
}

// NotificationEvent builds an event of type t describing n's current state.
func NotificationEvent(t EventType, n Notification) Event {
	return Event{
		Type:           t,
		NotificationID: n.ID,
		BatchID:        n.BatchID,
		Channel:        n.Channel,
		Priority:       n.Priority,
		Status:         n.Status,
		Attempts:       n.Attempts,
		LastError:      n.LastError,
		OccurredAt:     n.UpdatedAt,
	}
}
