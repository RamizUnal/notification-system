package realtime_test

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/realtime"
)

func event(ch domain.Channel, batch *uuid.UUID) domain.Event {
	return domain.Event{
		Type: domain.EventNotificationUpdated, NotificationID: uuid.New(), BatchID: batch,
		Channel: ch, Status: domain.StatusSent,
	}
}

func drain(s *realtime.Subscriber) []domain.Event {
	var out []domain.Event
	for {
		select {
		case e, ok := <-s.Events():
			if !ok {
				return out
			}
			out = append(out, e)
		default:
			return out
		}
	}
}

func TestFilters(t *testing.T) {
	t.Parallel()
	batch := uuid.New()
	inBatch := event(domain.ChannelSMS, &batch)
	other := event(domain.ChannelEmail, nil)

	tests := []struct {
		name   string
		filter realtime.Filter
		want   []domain.Event
	}{
		{"everything", realtime.Filter{}, []domain.Event{inBatch, other}},
		{"one batch", realtime.Filter{BatchID: &batch}, []domain.Event{inBatch}},
		{"one notification", realtime.Filter{NotificationID: &other.NotificationID}, []domain.Event{other}},
		{"one channel", realtime.Filter{Channels: []domain.Channel{domain.ChannelEmail}}, []domain.Event{other}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hub := realtime.NewHub()
			s := hub.Subscribe(tc.filter, 10)
			hub.Publish(inBatch)
			hub.Publish(other)
			assert.Equal(t, tc.want, drain(s))
		})
	}
}

func TestSlowSubscribersAreDisconnected(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub()
	slow := hub.Subscribe(realtime.Filter{}, 2)
	fast := hub.Subscribe(realtime.Filter{}, 100)

	for range 5 {
		hub.Publish(event(domain.ChannelPush, nil))
	}

	got := drain(slow)
	assert.Len(t, got, 2, "the buffered events are still delivered")
	_, open := <-slow.Events()
	assert.False(t, open, "then the stream ends so the client resyncs")
	assert.Len(t, drain(fast), 5, "other subscribers are unaffected")
	assert.Equal(t, 1, hub.Subscribers())
}

func TestConcurrentPublishAndSubscribe(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 200 {
				s := hub.Subscribe(realtime.Filter{}, 4)
				hub.Publish(event(domain.ChannelSMS, nil))
				hub.Unsubscribe(s)
			}
		})
	}
	wg.Wait()
	require.Zero(t, hub.Subscribers())
}
