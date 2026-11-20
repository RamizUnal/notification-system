//go:build integration

// Package e2e_test drives the whole system through its public HTTP API:
// the real API handler and the real delivery engine run against a real
// PostgreSQL database and deliver to a fake provider that records exactly
// what it receives.
package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/app"
	"github.com/RamizUnal/notification-system/internal/delivery"
	"github.com/RamizUnal/notification-system/internal/provider/webhook"
	"github.com/RamizUnal/notification-system/internal/ratelimit"
	"github.com/RamizUnal/notification-system/internal/realtime"
	"github.com/RamizUnal/notification-system/internal/storage/postgres"
	"github.com/RamizUnal/notification-system/internal/testutil/pgtest"
)

// fakeProvider answers like the configured webhook.site endpoint and
// records every request.
type fakeProvider struct {
	mu        sync.Mutex
	failFirst bool // answer 503 to the first attempt of each notification
	attempts  map[string]int
	accepted  map[string]int
	arrivals  []time.Time
}

func (p *fakeProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body struct{ To, Channel, Content string }
	_ = json.NewDecoder(r.Body).Decode(&body)
	id := r.Header.Get("Idempotency-Key")

	p.mu.Lock()
	defer p.mu.Unlock()
	p.attempts[id]++
	p.arrivals = append(p.arrivals, time.Now())

	switch {
	case strings.HasSuffix(body.To, "0000"):
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"error":"recipient is not reachable"}`)
	case p.failFirst && p.attempts[id] == 1:
		w.WriteHeader(http.StatusServiceUnavailable)
	default:
		p.accepted[id]++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprintf(w, `{"messageId":"msg-%s","status":"accepted","timestamp":"%s"}`,
			id, time.Now().UTC().Format(time.RFC3339))
	}
}

func (p *fakeProvider) snapshot() (attempts, accepted map[string]int, arrivals []time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	attempts, accepted = map[string]int{}, map[string]int{}
	for k, v := range p.attempts {
		attempts[k] = v
	}
	for k, v := range p.accepted {
		accepted[k] = v
	}
	return attempts, accepted, append([]time.Time(nil), p.arrivals...)
}

type fixedDelay time.Duration

func (d fixedDelay) Delay(int) time.Duration { return time.Duration(d) }

type system struct {
	baseURL  string
	provider *fakeProvider
}

type options struct {
	rateLimit int
	failFirst bool
}

func start(t *testing.T, opts options) *system {
	t.Helper()
	if opts.rateLimit == 0 {
		opts.rateLimit = 100
	}
	pool, _ := pgtest.New(t)
	logger := slog.New(slog.DiscardHandler)

	prov := &fakeProvider{failFirst: opts.failFirst, attempts: map[string]int{}, accepted: map[string]int{}}
	provSrv := httptest.NewServer(prov)
	sender, err := webhook.New(provSrv.URL + "/hook")
	require.NoError(t, err)

	hub := realtime.NewHub()
	apiSrv := httptest.NewServer(app.BuildAPI(app.APIOptions{
		Pool: pool, Hub: hub, Logger: logger, RateLimit: opts.rateLimit, MaxAttempts: 3,
	}))

	engine := app.BuildEngine(app.EngineOptions{
		Pool:    pool,
		Sender:  sender,
		Limiter: ratelimit.NewLocal(ratelimit.Config{PerSecond: opts.rateLimit, Burst: 1}),
		Policy:  fixedDelay(100 * time.Millisecond),
		Config: delivery.Config{
			WorkerID: "e2e-worker", Concurrency: 8, PollInterval: 50 * time.Millisecond,
			LeaseTTL: 10 * time.Second, SendTimeout: 2 * time.Second, ReaperInterval: time.Second,
			HeartbeatInterval: 500 * time.Millisecond, BreakerThreshold: 100, BreakerCooldown: time.Second,
		},
		Logger: logger,
	})

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		_ = engine.Run(ctx)
		close(stopped)
	}()
	go func() { _ = postgres.NewListener(pool, logger).Listen(ctx, hub.Publish) }()

	t.Cleanup(func() {
		cancel()
		<-stopped
		apiSrv.Close()
		provSrv.Close()
	})
	return &system{baseURL: apiSrv.URL, provider: prov}
}

type response struct {
	status  int
	headers http.Header
	body    map[string]any
}

func (s *system) call(t *testing.T, method, path string, body any, headers ...string) response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, s.baseURL+path, reader)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	out := response{status: resp.StatusCode, headers: resp.Header}
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	if len(raw) > 0 {
		require.NoError(t, json.Unmarshal(raw, &out.body), string(raw))
	}
	return out
}

func (s *system) status(t *testing.T, id string) string {
	t.Helper()
	return s.call(t, http.MethodGet, "/api/v1/notifications/"+id, nil).body["status"].(string)
}

func eventually(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting until %s", timeout, what)
}

func notification(i int) map[string]any {
	channels := []string{"sms", "email", "push"}
	recipients := map[string]string{"sms": "+905551234567", "email": "ada@example.com", "push": "device-token-0123456789"}
	priorities := []string{"high", "normal", "low"}
	ch := channels[i%3]
	return map[string]any{
		"channel": ch, "recipient": recipients[ch], "priority": priorities[(i/3)%3],
		"content": fmt.Sprintf("message %d", i),
	}
}

func TestBatchIsDeliveredExactlyOncePerNotification(t *testing.T) {
	t.Parallel()
	sys := start(t, options{})

	items := make([]map[string]any, 60)
	for i := range items {
		items[i] = notification(i)
	}
	created := sys.call(t, http.MethodPost, "/api/v1/batches", map[string]any{"notifications": items})
	require.Equal(t, http.StatusAccepted, created.status, created.body)
	batchID := created.body["id"].(string)
	ids := created.body["notification_ids"].([]any)
	require.Len(t, ids, 60)

	eventually(t, 20*time.Second, "the batch is done", func() bool {
		return sys.call(t, http.MethodGet, "/api/v1/batches/"+batchID, nil).body["done"] == true
	})

	batch := sys.call(t, http.MethodGet, "/api/v1/batches/"+batchID, nil).body
	assert.Equal(t, float64(60), batch["status_counts"].(map[string]any)["sent"])

	attempts, accepted, _ := sys.provider.snapshot()
	for _, id := range ids {
		assert.Equal(t, 1, accepted[id.(string)], "notification %s accepted exactly once", id)
		assert.Equal(t, 1, attempts[id.(string)])
	}

	first := sys.call(t, http.MethodGet, "/api/v1/notifications/"+ids[0].(string), nil).body
	assert.Equal(t, "msg-"+ids[0].(string), first["provider_message_id"])
	assert.NotNil(t, first["sent_at"])
	assert.Nil(t, first["next_attempt_at"], "a finished notification is no longer due")
}

func TestTransientFailuresAreRetried(t *testing.T) {
	t.Parallel()
	sys := start(t, options{failFirst: true})

	created := sys.call(t, http.MethodPost, "/api/v1/notifications", notification(0))
	require.Equal(t, http.StatusAccepted, created.status)
	id := created.body["id"].(string)

	eventually(t, 10*time.Second, "the retry succeeds", func() bool { return sys.status(t, id) == "sent" })

	attempts := sys.call(t, http.MethodGet, "/api/v1/notifications/"+id+"/attempts", nil).body["data"].([]any)
	require.Len(t, attempts, 2)
	assert.Equal(t, "retryable_failure", attempts[0].(map[string]any)["outcome"])
	assert.Equal(t, float64(503), attempts[0].(map[string]any)["status_code"])
	assert.Equal(t, "success", attempts[1].(map[string]any)["outcome"])
}

func TestPermanentFailuresStopAndCanBeRetriedByHand(t *testing.T) {
	t.Parallel()
	sys := start(t, options{})

	created := sys.call(t, http.MethodPost, "/api/v1/notifications", map[string]any{
		"channel": "sms", "recipient": "+905550000000", "content": "unreachable",
	})
	id := created.body["id"].(string)

	eventually(t, 10*time.Second, "the notification fails", func() bool { return sys.status(t, id) == "failed" })
	n := sys.call(t, http.MethodGet, "/api/v1/notifications/"+id, nil).body
	assert.Equal(t, float64(1), n["attempts"], "permanent failures are not retried automatically")
	assert.Contains(t, n["last_error"], "422")

	retried := sys.call(t, http.MethodPost, "/api/v1/notifications/"+id+"/retry", nil)
	require.Equal(t, http.StatusOK, retried.status)
	assert.Equal(t, "queued", retried.body["status"])
	eventually(t, 10*time.Second, "the manual retry is attempted", func() bool {
		attempts, _, _ := sys.provider.snapshot()
		return attempts[id] == 2
	})

	conflict := sys.call(t, http.MethodPost, "/api/v1/notifications/"+id+"/cancel", nil)
	eventually(t, 5*time.Second, "it fails again", func() bool { return sys.status(t, id) == "failed" })
	assert.Contains(t, []int{http.StatusOK, http.StatusConflict}, conflict.status)
}

func TestScheduledNotificationsCanBeCancelled(t *testing.T) {
	t.Parallel()
	sys := start(t, options{})

	body := notification(1)
	body["scheduled_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	created := sys.call(t, http.MethodPost, "/api/v1/notifications", body)
	require.Equal(t, http.StatusAccepted, created.status)
	assert.Equal(t, "scheduled", created.body["status"])
	id := created.body["id"].(string)

	cancelled := sys.call(t, http.MethodPost, "/api/v1/notifications/"+id+"/cancel", nil)
	require.Equal(t, http.StatusOK, cancelled.status)
	assert.Equal(t, "cancelled", cancelled.body["status"])

	again := sys.call(t, http.MethodPost, "/api/v1/notifications/"+id+"/cancel", nil)
	assert.Equal(t, http.StatusConflict, again.status)
	assert.Equal(t, "not_cancellable", again.body["code"])

	attempts, _, _ := sys.provider.snapshot()
	assert.Zero(t, attempts[id], "a cancelled notification never reaches the provider")
}

func TestIdempotencyKeyPreventsDuplicates(t *testing.T) {
	t.Parallel()
	sys := start(t, options{})

	first := sys.call(t, http.MethodPost, "/api/v1/notifications", notification(2), "Idempotency-Key", "order-42")
	second := sys.call(t, http.MethodPost, "/api/v1/notifications", notification(2), "Idempotency-Key", "order-42")

	require.Equal(t, http.StatusAccepted, first.status)
	require.Equal(t, http.StatusOK, second.status)
	assert.Equal(t, "true", second.headers.Get("Idempotent-Replayed"))
	assert.Equal(t, first.body["id"], second.body["id"])

	changed := notification(2)
	changed["content"] = "something else"
	conflict := sys.call(t, http.MethodPost, "/api/v1/notifications", changed, "Idempotency-Key", "order-42")
	assert.Equal(t, http.StatusConflict, conflict.status)
	assert.Equal(t, "idempotency_conflict", conflict.body["code"])

	id := first.body["id"].(string)
	eventually(t, 10*time.Second, "it is delivered", func() bool { return sys.status(t, id) == "sent" })
	_, accepted, _ := sys.provider.snapshot()
	assert.Equal(t, 1, accepted[id])
}

func TestRateLimitHoldsAtTheProvider(t *testing.T) {
	t.Parallel()
	const perSecond = 20
	sys := start(t, options{rateLimit: perSecond})

	items := make([]map[string]any, 60)
	for i := range items {
		items[i] = map[string]any{"channel": "sms", "recipient": "+905551234567", "content": fmt.Sprintf("burst %d", i)}
	}
	created := sys.call(t, http.MethodPost, "/api/v1/batches", map[string]any{"notifications": items})
	require.Equal(t, http.StatusAccepted, created.status)
	batchID := created.body["id"].(string)

	eventually(t, 20*time.Second, "the burst is drained", func() bool {
		return sys.call(t, http.MethodGet, "/api/v1/batches/"+batchID, nil).body["done"] == true
	})

	_, _, arrivals := sys.provider.snapshot()
	require.Len(t, arrivals, 60)
	peak := 0
	for i := range arrivals {
		j := i
		for j < len(arrivals) && arrivals[j].Sub(arrivals[i]) < time.Second {
			j++
		}
		peak = max(peak, j-i)
	}
	assert.LessOrEqual(t, peak, perSecond+1, "the provider never sees more than the limit in any second")
	assert.GreaterOrEqual(t, arrivals[59].Sub(arrivals[0]), 2500*time.Millisecond, "60 messages at 20/s take about 3s")

	metrics := sys.call(t, http.MethodGet, "/api/v1/metrics?window=5m", nil).body
	sms := metrics["channels"].([]any)[0].(map[string]any)
	assert.Equal(t, "sms", sms["channel"])
	assert.Equal(t, float64(60), sms["deliveries"].(map[string]any)["succeeded"])
	assert.Equal(t, float64(perSecond), sms["rate_limit_per_second"])
	assert.NotEmpty(t, metrics["workers"], "the worker's heartbeat is visible")
}

func TestStatusChangesAreStreamedOverWebSocket(t *testing.T) {
	t.Parallel()
	sys := start(t, options{})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(sys.baseURL, "http")+"/api/v1/events?channel=email", nil)
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()
	_, ready, err := conn.Read(ctx)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"ready"}`, string(ready))

	// The API's listener connects asynchronously; give it a moment.
	time.Sleep(500 * time.Millisecond)
	created := sys.call(t, http.MethodPost, "/api/v1/notifications", notification(1)) // email
	id := created.body["id"].(string)

	seen := map[string]bool{}
	for !seen["sent"] {
		_, data, err := conn.Read(ctx)
		require.NoError(t, err, "statuses seen so far: %v", seen)
		var msg struct {
			Type string         `json:"type"`
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(data, &msg))
		if msg.Data["notification_id"] == id {
			seen[msg.Data["status"].(string)] = true
		}
	}
	assert.True(t, seen["queued"], "creation is announced")
}
