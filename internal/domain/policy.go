package domain

import (
	"net/mail"
	"regexp"
	"strings"
)

// ChannelPolicy captures the delivery rules that differ between channels:
// what a valid recipient looks like and how long content may be. Each channel
// gets its own policy value (a strategy), so adding a channel means adding one
// entry to channelPolicies rather than touching validation code.
type ChannelPolicy struct {
	// MaxContentLength is the content limit in characters (Unicode code points).
	MaxContentLength int
	// RecipientFormat describes a valid recipient, for error messages and docs.
	RecipientFormat string

	validRecipient func(string) bool
}

// ValidRecipient reports whether recipient is well-formed for the channel.
func (p ChannelPolicy) ValidRecipient(recipient string) bool {
	return p.validRecipient(recipient)
}

var (
	// E.164: a plus sign, a country code that does not start with 0, and at
	// most 15 digits in total.
	e164Pattern = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
	// APNs and FCM tokens are opaque ASCII strings of letters, digits and a
	// few separators.
	deviceTokenPattern = regexp.MustCompile(`^[A-Za-z0-9._:\-]{16,512}$`)
)

var channelPolicies = map[Channel]ChannelPolicy{
	ChannelSMS: {
		// 1,600 characters is the longest concatenated SMS (about ten
		// segments) that carriers and providers commonly accept.
		MaxContentLength: 1600,
		RecipientFormat:  "an E.164 phone number such as +905551234567",
		validRecipient:   e164Pattern.MatchString,
	},
	ChannelEmail: {
		MaxContentLength: 10000,
		RecipientFormat:  "a plain email address such as ada@example.com",
		validRecipient:   isPlainEmail,
	},
	ChannelPush: {
		// 500 characters stays below the 4 KB APNs and FCM payload limits
		// even if every character needs four bytes of UTF-8.
		MaxContentLength: 500,
		RecipientFormat:  "a device token of 16-512 letters, digits or . _ : -",
		validRecipient:   deviceTokenPattern.MatchString,
	},
}

// PolicyFor returns the delivery rules of c. The second result is false for
// unknown channels.
func PolicyFor(c Channel) (ChannelPolicy, bool) {
	p, ok := channelPolicies[c]
	return p, ok
}

// isPlainEmail accepts a bare address (no display name) whose domain has at
// least one dot, which is what a notification provider can actually deliver to.
func isPlainEmail(s string) bool {
	if len(s) > 254 {
		return false
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s {
		return false
	}
	at := strings.LastIndexByte(s, '@')
	domain := s[at+1:]
	return strings.Contains(domain, ".") && !strings.HasSuffix(domain, ".")
}
