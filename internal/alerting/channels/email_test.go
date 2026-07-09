package channels

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMailer struct {
	to     string
	tmpl   string
	called bool
}

func (f *fakeMailer) Send(tmpl, email string, _ any) error {
	f.tmpl, f.to, f.called = tmpl, email, true
	return nil
}

func TestEmailSender_Send(t *testing.T) {
	fm := &fakeMailer{}
	s := NewEmail(fm)
	payload, _ := json.Marshal(map[string]any{"to": "ops@acme.com", "rule_name": "cost", "value": 9.0})
	require.NoError(t, s.Send(context.Background(), payload))
	assert.True(t, fm.called)
	assert.Equal(t, "ops@acme.com", fm.to)
	assert.Equal(t, "alert.tmpl", fm.tmpl)
}
