package domain

import "fmt"

// Channel is the medium a notification is delivered through.
type Channel string

// Supported channels.
const (
	ChannelSMS   Channel = "sms"
	ChannelEmail Channel = "email"
	ChannelPush  Channel = "push"
)

// Channels returns every supported channel in a stable order.
func Channels() []Channel {
	return []Channel{ChannelSMS, ChannelEmail, ChannelPush}
}

// Valid reports whether c is a supported channel.
func (c Channel) Valid() bool {
	switch c {
	case ChannelSMS, ChannelEmail, ChannelPush:
		return true
	default:
		return false
	}
}

// ParseChannel converts s into a Channel, rejecting unknown values.
func ParseChannel(s string) (Channel, error) {
	c := Channel(s)
	if !c.Valid() {
		return "", fmt.Errorf("unknown channel %q", s)
	}
	return c, nil
}
