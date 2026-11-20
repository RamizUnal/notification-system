package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/realtime"
)

const (
	subscriberBuffer = 1024
	pingInterval     = 25 * time.Second
	writeTimeout     = 5 * time.Second
	// closeResync tells a client it fell behind and must refetch state.
	closeResync websocket.StatusCode = 4000
)

// events handles GET /api/v1/events, upgrading to a WebSocket that streams
// status changes as they are committed. Optional query parameters narrow
// the stream: notification_id, batch_id, channel (repeatable).
func (a *API) events(w http.ResponseWriter, r *http.Request) {
	filter, err := parseEventFilter(r)
	if err != nil {
		a.handle(func(http.ResponseWriter, *http.Request) error { return err })(w, r)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: a.cfg.AllowedOrigins})
	if err != nil {
		a.logger.WarnContext(r.Context(), "websocket upgrade failed", slog.Any("error", err))
		return
	}
	defer func() { _ = conn.CloseNow() }()

	sub := a.cfg.Hub.Subscribe(filter, subscriberBuffer)
	defer a.cfg.Hub.Unsubscribe(sub)

	// Clients only listen; CloseRead discards anything they send and
	// cancels ctx when they disconnect.
	ctx := conn.CloseRead(r.Context())
	if err := writeMessage(ctx, conn, map[string]string{"type": "ready"}); err != nil {
		return
	}

	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-sub.Events():
			if !ok {
				_ = conn.Close(closeResync, "too slow: reconnect and refetch")
				return
			}
			if err := writeMessage(ctx, conn, toEventMessage(e)); err != nil {
				return
			}
		case <-ping.C:
			pingCtx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func writeMessage(ctx context.Context, conn *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, b)
}

func parseEventFilter(r *http.Request) (realtime.Filter, error) {
	var (
		f    realtime.Filter
		errs queryErrors
		q    = r.URL.Query()
	)
	if v := q.Get("notification_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			errs.add("notification_id", "must be a UUID")
		} else {
			f.NotificationID = &id
		}
	}
	if v := q.Get("batch_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			errs.add("batch_id", "must be a UUID")
		} else {
			f.BatchID = &id
		}
	}
	for _, v := range multiValues(q, "channel") {
		ch, err := domain.ParseChannel(v)
		if err != nil {
			errs.add("channel", "unknown channel "+v)
			continue
		}
		f.Channels = append(f.Channels, ch)
	}
	return f, errs.err()
}
