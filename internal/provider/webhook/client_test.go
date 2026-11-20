package webhook_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/domain"
	"github.com/RamizUnal/notification-system/internal/provider"
	"github.com/RamizUnal/notification-system/internal/provider/webhook"
)

func message() provider.Message {
	return provider.Message{
		NotificationID: uuid.MustParse("0193a0b2-7c1e-7d4f-9a51-2b3c4d5e6f70"),
		Channel:        domain.ChannelSMS,
		Recipient:      "+905551234567",
		Content:        "Your message",
		CorrelationID:  "req-42",
	}
}

func newClient(t *testing.T, handler http.HandlerFunc, opts ...webhook.Option) *webhook.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := webhook.New(srv.URL+"/hook", opts...)
	require.NoError(t, err)
	return c
}

func TestSendPostsTheDocumentedRequest(t *testing.T) {
	t.Parallel()

	var (
		gotBody    map[string]string
		gotHeaders http.Header
		gotPath    string
	)
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeaders, gotPath = r.Header.Clone(), r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"messageId":"m-1","status":"accepted","timestamp":"2026-11-20T12:00:01Z"}`)
	})

	receipt, err := c.Send(context.Background(), message())

	require.NoError(t, err)
	assert.Equal(t, "/hook", gotPath)
	assert.Equal(t, map[string]string{"to": "+905551234567", "channel": "sms", "content": "Your message"}, gotBody)
	assert.Equal(t, "application/json", gotHeaders.Get("Content-Type"))
	assert.Equal(t, "0193a0b2-7c1e-7d4f-9a51-2b3c4d5e6f70", gotHeaders.Get("Idempotency-Key"))
	assert.Equal(t, "req-42", gotHeaders.Get("X-Correlation-ID"))

	assert.Equal(t, provider.Receipt{
		MessageID:  "m-1",
		Status:     "accepted",
		AcceptedAt: time.Date(2026, 11, 20, 12, 0, 1, 0, time.UTC),
		StatusCode: http.StatusAccepted,
	}, receipt)
}

func TestSendToleratesUnexpectedSuccessBodies(t *testing.T) {
	t.Parallel()

	// webhook.site answers 200 with plain text until a response is configured.
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "This URL has no default content configured.")
	})

	receipt, err := c.Send(context.Background(), message())

	require.NoError(t, err, "a 2xx means the provider took the message")
	assert.Equal(t, http.StatusOK, receipt.StatusCode)
	assert.Equal(t, "accepted", receipt.Status)
	assert.Empty(t, receipt.MessageID)
}

func TestSendClassifiesFailures(t *testing.T) {
	t.Parallel()

	retryDate := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)
	tests := []struct {
		name       string
		status     int
		retryAfter string
		body       string
		retryable  bool
		minDelay   time.Duration
	}{
		{"throttled with delay-seconds", http.StatusTooManyRequests, "7", "slow down", true, 7 * time.Second},
		{"unavailable with HTTP date", http.StatusServiceUnavailable, retryDate, "", true, 80 * time.Second},
		{"internal server error", http.StatusInternalServerError, "", "boom", true, 0},
		{"bad gateway", http.StatusBadGateway, "", "", true, 0},
		{"request timeout", http.StatusRequestTimeout, "", "", true, 0},
		{"bad request", http.StatusBadRequest, "", "invalid recipient", false, 0},
		{"not found", http.StatusNotFound, "", "", false, 0},
		{"gone (expired webhook.site URL)", http.StatusGone, "", "", false, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
				if tc.retryAfter != "" {
					w.Header().Set("Retry-After", tc.retryAfter)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})

			_, err := c.Send(context.Background(), message())

			var perr *provider.Error
			require.ErrorAs(t, err, &perr)
			assert.Equal(t, tc.status, perr.StatusCode)
			assert.Equal(t, tc.retryable, perr.Retryable)
			assert.Equal(t, tc.retryable, provider.IsRetryable(err))
			assert.GreaterOrEqual(t, perr.RetryAfter, tc.minDelay)
			if tc.body != "" {
				assert.Contains(t, perr.Error(), tc.body)
			}
		})
	}
}

func TestSendTreatsTransportFailuresAsRetryable(t *testing.T) {
	t.Parallel()

	t.Run("timeout", func(t *testing.T) {
		t.Parallel()
		release := make(chan struct{})
		c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body) // lets the server notice the client hanging up
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}, webhook.WithHTTPClient(&http.Client{Timeout: 50 * time.Millisecond}))
		t.Cleanup(func() { close(release) }) // runs before the server's Close

		_, err := c.Send(context.Background(), message())

		var perr *provider.Error
		require.ErrorAs(t, err, &perr)
		assert.True(t, perr.Retryable)
		assert.Zero(t, perr.StatusCode)
		assert.Contains(t, perr.Error(), "did not respond in time")
	})

	t.Run("connection refused", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close()
		c, err := webhook.New(url)
		require.NoError(t, err)

		_, err = c.Send(context.Background(), message())

		assert.True(t, provider.IsRetryable(err))
		assert.Contains(t, err.Error(), "provider unreachable")
	})
}

func TestNewRejectsInvalidEndpoints(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{"", "webhook.site/abc", "ftp://example.com", "http://"} {
		_, err := webhook.New(endpoint)
		assert.Errorf(t, err, "endpoint %q", endpoint)
	}
}
