// Package realtime fans committed domain events out to live subscribers,
// such as WebSocket clients of the console.
package realtime

import (
	"slices"
	"sync"

	"github.com/google/uuid"

	"github.com/RamizUnal/notification-system/internal/domain"
)

// Filter selects the events a subscriber receives. Zero-valued fields match
// everything.
type Filter struct {
	NotificationID *uuid.UUID
	BatchID        *uuid.UUID
	Channels       []domain.Channel
}

// Matches reports whether e passes the filter.
func (f Filter) Matches(e domain.Event) bool {
	if f.NotificationID != nil && e.NotificationID != *f.NotificationID {
		return false
	}
	if f.BatchID != nil && (e.BatchID == nil || *e.BatchID != *f.BatchID) {
		return false
	}
	if len(f.Channels) > 0 && e.Channel != "" && !slices.Contains(f.Channels, e.Channel) {
		return false
	}
	return true
}

// Subscriber receives the events matching its filter.
type Subscriber struct {
	filter Filter
	events chan domain.Event
	once   sync.Once
}

// Events delivers matching events. The channel is closed when the
// subscriber falls too far behind (see Hub.Publish) or unsubscribes.
func (s *Subscriber) Events() <-chan domain.Event { return s.events }

func (s *Subscriber) close() { s.once.Do(func() { close(s.events) }) }

// Hub is an observer registry: publishers hand it events, it forwards each
// one to every subscriber whose filter matches.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[*Subscriber]struct{}
}

// NewHub returns an empty hub.
func NewHub() *Hub {
	return &Hub{subscribers: make(map[*Subscriber]struct{})}
}

// Subscribe registers a subscriber with room for buffer pending events.
func (h *Hub) Subscribe(f Filter, buffer int) *Subscriber {
	s := &Subscriber{filter: f, events: make(chan domain.Event, max(1, buffer))}
	h.mu.Lock()
	h.subscribers[s] = struct{}{}
	h.mu.Unlock()
	return s
}

// Unsubscribe removes s and closes its channel.
func (h *Hub) Unsubscribe(s *Subscriber) {
	h.mu.Lock()
	delete(h.subscribers, s)
	h.mu.Unlock()
	s.close()
}

// Publish forwards e to every matching subscriber without ever blocking.
//
// A subscriber whose buffer is full is disconnected rather than silently
// skipped: missing one status change would leave a client showing stale
// state, while a closed stream tells it to reconnect and refetch.
func (h *Hub) Publish(e domain.Event) {
	var lagging []*Subscriber
	h.mu.RLock()
	for s := range h.subscribers {
		if !s.filter.Matches(e) {
			continue
		}
		select {
		case s.events <- e:
		default:
			lagging = append(lagging, s)
		}
	}
	h.mu.RUnlock()

	for _, s := range lagging {
		h.Unsubscribe(s)
	}
}

// Subscribers returns the number of connected subscribers.
func (h *Hub) Subscribers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}
