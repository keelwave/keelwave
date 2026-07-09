package channels

import "context"

// Sender delivers an alert notification payload over one channel (email,
// webhook, etc). Implementations own their own transport and serialization.
type Sender interface {
	Send(ctx context.Context, payload []byte) error
}

// Registry maps a channel name to its Sender.
type Registry map[string]Sender

// For returns the Sender registered under channel, reporting whether one exists.
func (r Registry) For(channel string) (Sender, bool) {
	s, ok := r[channel]
	return s, ok
}
