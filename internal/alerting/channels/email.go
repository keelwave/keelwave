package channels

import (
	"context"
	"encoding/json"

	"github.com/keelwave/keelwave/internal/mailer"
)

type emailSender struct{ m mailer.Client }

// NewEmail returns a Sender that renders the alert template and delivers the
// notification via the mailer to the payload's "to" address.
func NewEmail(m mailer.Client) Sender { return &emailSender{m: m} }

func (s *emailSender) Send(_ context.Context, payload []byte) error {
	var p struct {
		To string `json:"to"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return err
	}
	var data map[string]any
	_ = json.Unmarshal(payload, &data)
	return s.m.Send(mailer.AlertTemplate, p.To, data)
}
