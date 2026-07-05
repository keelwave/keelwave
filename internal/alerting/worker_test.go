package alerting

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/keelwave/keelwave/internal/alerting/channels"
	"github.com/keelwave/keelwave/internal/store"
)

type countSender struct{ n int }

func (c *countSender) Send(context.Context, []byte) error { c.n++; return nil }

func TestWorker_drainOnceDelivers(t *testing.T) {
	ctx := context.Background()
	pool := testPool
	s := store.NewStorage(pool)
	p := seedProject(t, s)
	rule := &store.AlertRule{ProjectID: p.ID, Name: "r", Class: "event", Signal: "loop",
		Comparator: ">", Threshold: 0, Severity: "page", Channel: "email",
		ChannelConfig: []byte(`{}`), Enabled: true}
	require.NoError(t, s.AlertRules.Create(ctx, rule))
	enqueueTestJob(t, s, p.ID, rule.ID) // helper: upsert event + enqueue job in a tx

	cs := &countSender{}
	w := NewWorker(s, channels.Registry{"email": cs}, zap.NewNop().Sugar())
	n, err := w.drainOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, 1, cs.n)

	remaining, err := s.NotificationJobs.Claim(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, remaining, 0) // delivered job not re-claimable
}
