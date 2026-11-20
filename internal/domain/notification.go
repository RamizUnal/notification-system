package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// MaxIdempotencyKeyLength bounds client-supplied idempotency keys.
	MaxIdempotencyKeyLength = 255
	// MaxScheduleAhead is how far in the future a delivery may be scheduled.
	MaxScheduleAhead = 30 * 24 * time.Hour
	// ScheduleClockSkew tolerates clients whose clocks run slightly behind:
	// a scheduled time at most this far in the past means "deliver now".
	ScheduleClockSkew = time.Minute
)

// Draft is a request to send one notification, before it is validated.
type Draft struct {
	Channel        Channel
	Priority       Priority
	Recipient      string
	Content        string
	ScheduledAt    *time.Time
	IdempotencyKey string
	TemplateID     *uuid.UUID
}

// Notification is a message on its way to one recipient through one channel.
type Notification struct {
	ID                uuid.UUID
	BatchID           *uuid.UUID
	TemplateID        *uuid.UUID
	Channel           Channel
	Priority          Priority
	Recipient         string
	Content           string
	Status            Status
	IdempotencyKey    string
	Fingerprint       string
	Attempts          int
	MaxAttempts       int
	ScheduledAt       *time.Time
	NextAttemptAt     time.Time
	LastError         string
	ProviderMessageID string
	CorrelationID     string
	TraceParent       string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	SentAt            *time.Time
	FailedAt          *time.Time
	CancelledAt       *time.Time
}

// Normalize returns d with surrounding whitespace removed from the recipient
// and the default priority applied. Content is kept verbatim on purpose:
// leading or trailing whitespace may be meaningful to the sender.
func (d Draft) Normalize() Draft {
	d.Recipient = strings.TrimSpace(d.Recipient)
	if d.Priority == "" {
		d.Priority = PriorityNormal
	}
	return d
}

// Validate checks a normalized draft against the rules of its channel and
// returns every problem it finds as a *ValidationError, or nil.
func (d Draft) Validate(now time.Time) error {
	var verr ValidationError

	policy, known := PolicyFor(d.Channel)
	switch {
	case d.Channel == "":
		verr.Add("channel", CodeRequired, "channel is required")
	case !known:
		verr.Add("channel", CodeInvalidValue, fmt.Sprintf("channel must be one of %s", joinValues(Channels())))
	}

	if !d.Priority.Valid() {
		verr.Add("priority", CodeInvalidValue, fmt.Sprintf("priority must be one of %s", joinValues(Priorities())))
	}

	switch {
	case d.Recipient == "":
		verr.Add("recipient", CodeRequired, "recipient is required")
	case known && !policy.ValidRecipient(d.Recipient):
		verr.Add("recipient", CodeInvalidFormat, fmt.Sprintf("recipient must be %s", policy.RecipientFormat))
	}

	validateContent(&verr, d.Content, d.Channel, policy, known)

	if d.ScheduledAt != nil {
		switch {
		case d.ScheduledAt.Before(now.Add(-ScheduleClockSkew)):
			verr.Add("scheduled_at", CodeOutOfRange, "scheduled_at must not be in the past")
		case d.ScheduledAt.After(now.Add(MaxScheduleAhead)):
			verr.Add("scheduled_at", CodeOutOfRange, "scheduled_at must be within 30 days")
		}
	}

	if d.IdempotencyKey != "" && !validIdempotencyKey(d.IdempotencyKey) {
		verr.Add("idempotency_key", CodeInvalidFormat,
			fmt.Sprintf("idempotency_key must be 1-%d visible ASCII characters", MaxIdempotencyKeyLength))
	}

	return verr.Err()
}

// Fingerprint identifies the payload of a normalized draft. A request that
// reuses an idempotency key must carry the same fingerprint as the original.
func (d Draft) Fingerprint() string {
	scheduled := ""
	if d.ScheduledAt != nil {
		scheduled = d.ScheduledAt.UTC().Format(time.RFC3339Nano)
	}
	return fingerprint(string(d.Channel), string(d.Priority), d.Recipient, d.Content, scheduled)
}

// NewNotification validates d and builds a notification that is ready to be
// stored. Notifications scheduled for the future start as scheduled; all
// others start as queued and are due immediately.
func NewNotification(id uuid.UUID, d Draft, now time.Time, maxAttempts int) (Notification, error) {
	d = d.Normalize()
	if err := d.Validate(now); err != nil {
		return Notification{}, err
	}
	now = now.UTC()

	n := Notification{
		ID:             id,
		TemplateID:     d.TemplateID,
		Channel:        d.Channel,
		Priority:       d.Priority,
		Recipient:      d.Recipient,
		Content:        d.Content,
		Status:         StatusQueued,
		IdempotencyKey: d.IdempotencyKey,
		Fingerprint:    d.Fingerprint(),
		MaxAttempts:    maxAttempts,
		NextAttemptAt:  now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if d.ScheduledAt != nil && d.ScheduledAt.After(now) {
		at := d.ScheduledAt.UTC()
		n.Status = StatusScheduled
		n.ScheduledAt = &at
		n.NextAttemptAt = at
	}
	return n, nil
}

func validateContent(verr *ValidationError, content string, ch Channel, policy ChannelPolicy, known bool) {
	switch {
	case strings.TrimSpace(content) == "":
		verr.Add("content", CodeRequired, "content is required")
	case !utf8.ValidString(content):
		verr.Add("content", CodeInvalidFormat, "content must be valid UTF-8")
	case strings.ContainsRune(content, 0):
		verr.Add("content", CodeInvalidFormat, "content must not contain NUL characters")
	case known:
		if n := utf8.RuneCountInString(content); n > policy.MaxContentLength {
			verr.Add("content", CodeTooLong, fmt.Sprintf(
				"content is %d characters; %s allows at most %d", n, ch, policy.MaxContentLength))
		}
	}
}

func validIdempotencyKey(key string) bool {
	if len(key) > MaxIdempotencyKeyLength {
		return false
	}
	for i := range len(key) {
		if key[i] < '!' || key[i] > '~' {
			return false
		}
	}
	return true
}

func fingerprint(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0x1f}) // unit separator keeps ("ab","c") distinct from ("a","bc")
	}
	return hex.EncodeToString(h.Sum(nil))
}

func joinValues[T ~string](values []T) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = string(v)
	}
	return strings.Join(parts, ", ")
}
