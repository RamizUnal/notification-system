package mockprovider_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/RamizUnal/notification-system/internal/mockprovider"
)

func send(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAnswersLikeTheConfiguredWebhook(t *testing.T) {
	t.Parallel()
	h := mockprovider.New(mockprovider.Settings{}).Handler()

	rec := send(t, h, `{"to": "+905551234567", "channel": "sms", "content": "Your message"}`)

	require.Equal(t, http.StatusAccepted, rec.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "accepted", body["status"])
	assert.NotEmpty(t, body["messageId"])
	assert.NotEmpty(t, body["timestamp"])
}

func TestFailureModes(t *testing.T) {
	t.Parallel()

	outage := mockprovider.New(mockprovider.Settings{Outage: true}).Handler()
	assert.Equal(t, http.StatusServiceUnavailable,
		send(t, outage, `{"to": "+905551234567", "channel": "sms", "content": "x"}`).Code)

	always := mockprovider.New(mockprovider.Settings{FailureRate: 1}).Handler()
	assert.Equal(t, http.StatusServiceUnavailable,
		send(t, always, `{"to": "ada@example.com", "channel": "email", "content": "x"}`).Code)

	healthy := mockprovider.New(mockprovider.Settings{}).Handler()
	assert.Equal(t, http.StatusUnprocessableEntity,
		send(t, healthy, `{"to": "+905550000000", "channel": "sms", "content": "x"}`).Code,
		"reserved recipients trigger permanent failures on purpose")
	assert.Equal(t, http.StatusBadRequest, send(t, healthy, `{"channel": "sms"}`).Code)
}

func TestSettingsAndStats(t *testing.T) {
	t.Parallel()
	h := mockprovider.New(mockprovider.Settings{}).Handler()

	req := httptest.NewRequest(http.MethodPut, "/admin/settings", strings.NewReader(`{"failure_rate": 2}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	for range 3 {
		send(t, h, `{"to": "+905551234567", "channel": "sms", "content": "x"}`)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/stats", nil))

	var stats struct {
		Totals   map[string]int `json:"totals"`
		Channels map[string]struct {
			LastSecond    int `json:"last_second"`
			PeakPerSecond int `json:"peak_per_second"`
		} `json:"channels"`
		Recent []map[string]any `json:"recent"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &stats))
	assert.Equal(t, 3, stats.Totals["accepted"])
	assert.Equal(t, 3, stats.Channels["sms"].LastSecond)
	assert.Equal(t, 3, stats.Channels["sms"].PeakPerSecond)
	assert.Len(t, stats.Recent, 3)
}
