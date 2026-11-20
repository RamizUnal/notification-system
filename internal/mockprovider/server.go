// Package mockprovider is a stand-in for the external notification provider.
// It answers like the webhook.site endpoint described in the assessment
// (202 {"messageId", "status": "accepted", "timestamp"}), and it can inject
// latency, transient failures and full outages at runtime, so retries and
// the circuit breaker can be demonstrated without touching the internet.
//
// It also measures what it receives: the per-channel rate observed on the
// provider's side is independent evidence that rate limiting works.
package mockprovider

import (
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	recentLimit  = 25
	rateWindow   = 10 * time.Second
	maxBodyBytes = 64 << 10
)

// Settings control the simulated behaviour.
type Settings struct {
	// FailureRate is the share of requests answered 503 (0 to 1).
	FailureRate  float64 `json:"failure_rate"`
	MinLatencyMS int     `json:"min_latency_ms"`
	MaxLatencyMS int     `json:"max_latency_ms"`
	// Outage answers every request 503, as if the provider were down.
	Outage bool `json:"outage"`
}

func (s Settings) valid() bool {
	return s.FailureRate >= 0 && s.FailureRate <= 1 &&
		s.MinLatencyMS >= 0 && s.MaxLatencyMS >= s.MinLatencyMS && s.MaxLatencyMS <= 30_000
}

// Message is a request the provider received.
type Message struct {
	MessageID      string    `json:"message_id,omitempty"`
	To             string    `json:"to"`
	Channel        string    `json:"channel"`
	Content        string    `json:"content"`
	Status         int       `json:"status"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	CorrelationID  string    `json:"correlation_id,omitempty"`
	ReceivedAt     time.Time `json:"received_at"`
}

// Server is the mock provider.
type Server struct {
	mu       sync.Mutex
	settings Settings
	totals   map[string]int
	arrivals map[string][]time.Time
	recent   []Message
	random   func() float64
	now      func() time.Time
}

// New returns a server with the given initial settings.
func New(settings Settings) *Server {
	return &Server{
		settings: settings,
		totals:   map[string]int{},
		arrivals: map[string][]time.Time{},
		random:   rand.Float64,
		now:      time.Now,
	}
}

// Handler routes the provider endpoint and the admin API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /admin/settings", s.getSettings)
	mux.HandleFunc("PUT /admin/settings", s.putSettings)
	mux.HandleFunc("GET /admin/stats", s.stats)
	mux.HandleFunc("POST /admin/reset", s.reset)
	mux.HandleFunc("POST /", s.receive)
	return mux
}

type sendRequest struct {
	To      string `json:"to"`
	Channel string `json:"channel"`
	Content string `json:"content"`
}

func (s *Server) receive(w http.ResponseWriter, r *http.Request) {
	var req sendRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be JSON"})
		return
	}

	s.mu.Lock()
	settings := s.settings
	latency := settings.MinLatencyMS
	if spread := settings.MaxLatencyMS - settings.MinLatencyMS; spread > 0 {
		latency += int(s.random() * float64(spread))
	}
	failing := settings.Outage || s.random() < settings.FailureRate
	s.mu.Unlock()

	select {
	case <-time.After(time.Duration(latency) * time.Millisecond):
	case <-r.Context().Done():
		return
	}

	msg := Message{
		To: req.To, Channel: req.Channel, Content: req.Content,
		IdempotencyKey: r.Header.Get("Idempotency-Key"), CorrelationID: r.Header.Get("X-Correlation-ID"),
	}
	switch {
	case req.To == "" || req.Channel == "" || req.Content == "":
		msg.Status = http.StatusBadRequest
		s.record(msg)
		writeJSON(w, msg.Status, map[string]string{"error": "to, channel and content are required"})
	case rejectedRecipient(req.To):
		// Lets a demo trigger a permanent failure on purpose.
		msg.Status = http.StatusUnprocessableEntity
		s.record(msg)
		writeJSON(w, msg.Status, map[string]string{"error": "recipient is not reachable"})
	case failing:
		msg.Status = http.StatusServiceUnavailable
		s.record(msg)
		writeJSON(w, msg.Status, map[string]string{"error": "provider temporarily unavailable"})
	default:
		msg.Status = http.StatusAccepted
		msg.MessageID = uuid.NewString()
		at := s.record(msg)
		writeJSON(w, msg.Status, map[string]string{
			"messageId": msg.MessageID, "status": "accepted", "timestamp": at.Format(time.RFC3339Nano),
		})
	}
}

// rejectedRecipient reports recipients the mock refuses permanently: phone
// numbers ending in 0000 and addresses or tokens containing "invalid".
func rejectedRecipient(to string) bool {
	return strings.HasSuffix(to, "0000") || strings.Contains(strings.ToLower(to), "invalid")
}

func (s *Server) record(m Message) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	m.ReceivedAt = now

	s.totals["received"]++
	switch {
	case m.Status == http.StatusAccepted:
		s.totals["accepted"]++
	case m.Status >= 500:
		s.totals["failed"]++
	default:
		s.totals["rejected"]++
	}

	s.arrivals[m.Channel] = append(s.arrivals[m.Channel], now)
	times := s.arrivals[m.Channel]
	cut := 0
	for cut < len(times) && now.Sub(times[cut]) > rateWindow {
		cut++
	}
	s.arrivals[m.Channel] = times[cut:]

	s.recent = append([]Message{m}, s.recent...)
	if len(s.recent) > recentLimit {
		s.recent = s.recent[:recentLimit]
	}
	return now
}

type channelStats struct {
	LastSecond    int `json:"last_second"`
	PeakPerSecond int `json:"peak_per_second"`
}

type statsResponse struct {
	Settings Settings                `json:"settings"`
	Totals   map[string]int          `json:"totals"`
	Channels map[string]channelStats `json:"channels"`
	Recent   []Message               `json:"recent"`
}

// stats reports totals, the arrival rate per channel over the last ten
// seconds, and the most recent messages.
func (s *Server) stats(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()

	resp := statsResponse{
		Settings: s.settings,
		Totals:   map[string]int{"received": 0, "accepted": 0, "failed": 0, "rejected": 0},
		Channels: map[string]channelStats{},
		Recent:   slices.Clone(s.recent),
	}
	for k, v := range s.totals {
		resp.Totals[k] = v
	}
	for _, ch := range []string{"sms", "email", "push"} {
		resp.Channels[ch] = rate(s.arrivals[ch], now)
	}
	if resp.Recent == nil {
		resp.Recent = []Message{}
	}
	writeJSON(w, http.StatusOK, resp)
}

// rate counts arrivals in the last second and the busiest rolling second
// among the retained arrivals (which are sorted by time).
func rate(times []time.Time, now time.Time) channelStats {
	var st channelStats
	for _, t := range times {
		if now.Sub(t) <= time.Second {
			st.LastSecond++
		}
	}
	end := 0
	for start := range times {
		for end < len(times) && times[end].Sub(times[start]) < time.Second {
			end++
		}
		st.PeakPerSecond = max(st.PeakPerSecond, end-start)
	}
	return st
}

func (s *Server) getSettings(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, s.settings)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var next Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&next); err != nil || !next.valid() {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": "failure_rate must be 0-1 and 0 <= min_latency_ms <= max_latency_ms <= 30000",
		})
		return
	}
	s.mu.Lock()
	s.settings = next
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, next)
}

func (s *Server) reset(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	s.totals = map[string]int{}
	s.arrivals = map[string][]time.Time{}
	s.recent = nil
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
