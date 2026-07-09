package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/keelwave/keelwave/internal/store"
)

func TestScheduler_tickEvaluatesAggregate(t *testing.T) {
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

	// An enabled event-class rule with a would-breach cost_burn signal: the tick
	// must skip it because it filters to class=="aggregate".
	evtRule := &store.AlertRule{ProjectID: p.ID, Name: "cost-evt", Class: "event",
		Signal: "cost_burn", Comparator: ">", Threshold: 5, Severity: "page",
		Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`), Enabled: true,
		WindowSeconds: &win, ForSeconds: 0}
	require.NoError(t, s.AlertRules.Create(ctx, evtRule))

	ev := NewEvaluator(pool, s, zap.NewNop().Sugar())
	sc := NewScheduler(ev, s, zap.NewNop().Sugar(), time.Minute)
	sc.tick(ctx)

	// aggregate rule evaluated → live firing event + queued email job. The rule
	// is unscoped, so it fires on the project-wide empty scope.
	live, err := s.AlertEvents.GetLive(ctx, rule.ID, Fingerprint(rule.ID, ""))
	require.NoError(t, err)
	assert.Equal(t, "firing", live.State)

	jobs, err := s.NotificationJobs.Claim(ctx, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(jobs), 1)

	// event-class rule skipped → no event written for it
	_, err = s.AlertEvents.GetLive(ctx, evtRule.ID, Fingerprint(evtRule.ID, ""))
	assert.ErrorIs(t, err, store.ErrNotFound, "event-class rule must be skipped by the aggregate tick")
}
