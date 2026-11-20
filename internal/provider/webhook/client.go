// Package webhook delivers messages by POSTing them as JSON to an HTTP
// endpoint. It is the integration used with webhook.site (the provider
// simulator named in the assessment) and with the bundled mock provider.
//
// Request:  POST {endpoint} {"to": "+905551234567", "channel": "sms", "content": "..."}
// Response: 202 {"messageId": "...", "status": "accepted", "timestamp": "ISO8601"}
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/RamizUnal/notification-system/internal/provider"
)

const (
	defaultTimeout   = 5 * time.Second
	maxResponseBytes = 64 << 10
	maxErrorBytes    = 512
)

// Client posts messages to one provider endpoint.
type Client struct {
	endpoint  string
	http      *http.Client
	userAgent string
	now       func() time.Time
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the HTTP client, for example to add tracing.
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) { cl.http = c }
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(cl *Client) { cl.userAgent = ua }
}

// New returns a client for endpoint, which must be an absolute http(s) URL.
func New(endpoint string, opts ...Option) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("provider URL %q must be an absolute http(s) URL", endpoint)
	}
	c := &Client{
		endpoint:  endpoint,
		http:      &http.Client{Timeout: defaultTimeout},
		userAgent: "notification-system/1.0",
		now:       time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

type requestBody struct {
	To      string `json:"to"`
	Channel string `json:"channel"`
	Content string `json:"content"`
}

type responseBody struct {
	MessageID string `json:"messageId"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// Send delivers one message. Failures are returned as *provider.Error,
// classified as retryable or permanent.
func (c *Client) Send(ctx context.Context, msg provider.Message) (provider.Receipt, error) {
	payload, err := json.Marshal(requestBody{To: msg.Recipient, Channel: string(msg.Channel), Content: msg.Content})
	if err != nil {
		return provider.Receipt{}, &provider.Error{Message: "encode request", Err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return provider.Receipt{}, &provider.Error{Message: "build request", Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	// Lets a provider that supports idempotency drop a resend of the same
	// notification (see "at-least-once delivery" in docs/architecture.md).
	req.Header.Set("Idempotency-Key", msg.NotificationID.String())
	if msg.CorrelationID != "" {
		req.Header.Set("X-Correlation-ID", msg.CorrelationID)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return provider.Receipt{}, &provider.Error{Retryable: true, Message: describeTransportError(err), Err: err}
	}
	defer func() {
		// Drain so the connection can be reused for the next message.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return c.decodeReceipt(resp), nil
	}
	return provider.Receipt{}, c.classify(resp)
}

// decodeReceipt reads the provider's acknowledgement. Any 2xx means the
// provider took the message, so an unexpected body is tolerated rather than
// treated as a failure: retrying it would send the message twice.
func (c *Client) decodeReceipt(resp *http.Response) provider.Receipt {
	receipt := provider.Receipt{StatusCode: resp.StatusCode, Status: "accepted", AcceptedAt: c.now().UTC()}

	var body responseBody
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&body); err != nil {
		return receipt
	}
	receipt.MessageID = body.MessageID
	if body.Status != "" {
		receipt.Status = body.Status
	}
	if ts, err := time.Parse(time.RFC3339Nano, body.Timestamp); err == nil {
		receipt.AcceptedAt = ts.UTC()
	}
	return receipt
}

// classify turns a non-2xx response into a provider error. Throttling (429),
// request timeouts (408, 425) and server errors (5xx) are transient; any
// other 4xx means the request itself is wrong and resending cannot help.
func (c *Client) classify(resp *http.Response) *provider.Error {
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBytes))
	message := strings.Join(strings.Fields(string(snippet)), " ")
	if message == "" {
		message = http.StatusText(resp.StatusCode)
	}

	perr := &provider.Error{StatusCode: resp.StatusCode, Message: message}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests,
		resp.StatusCode == http.StatusRequestTimeout,
		resp.StatusCode == http.StatusTooEarly,
		resp.StatusCode >= 500:
		perr.Retryable = true
		perr.RetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"), c.now())
	}
	return perr
}

// parseRetryAfter understands both forms of the header: delay-seconds and an
// HTTP date.
func parseRetryAfter(value string, now time.Time) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}

func describeTransportError(err error) string {
	var netErr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return "provider did not respond in time"
	case errors.Is(err, context.Canceled):
		return "request cancelled"
	default:
		return "provider unreachable"
	}
}
