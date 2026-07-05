package alerting

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/keelwave/keelwave/internal/store"
)

func TestEvaluator_costBurnFires(t *testing.T) {
	ctx := context.Background()
	pool := testPool
	s := store.NewStorage(pool)
	p := seedProject(t, s)

	// two runs in the last window totalling $9 cost for agent "x"
	seedFinishedRun(t, s, p.ID, "x", 6.0)
	seedFinishedRun(t, s, p.ID, "x", 3.0)

	win := 300
	rule := &store.AlertRule{ProjectID: p.ID, Name: "cost", Class: "aggregate",
		Signal: "cost_burn", Comparator: ">", Threshold: 5, Severity: "page",
		Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`), Enabled: true,
		WindowSeconds: &win, ForSeconds: 0}
	require.NoError(t, s.AlertRules.Create(ctx, rule))

	ev := NewEvaluator(pool, s, zap.NewNop().Sugar())
	require.NoError(t, ev.EvaluateAggregate(ctx, rule))

	// a live firing event + a queued email job exist
	fp := Fingerprint(rule.ID, "x")
	live, err := s.AlertEvents.GetLive(ctx, rule.ID, fp)
	require.NoError(t, err)
	assert.Equal(t, "firing", live.State)

	jobs, err := s.NotificationJobs.Claim(ctx, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(jobs), 1)
}
