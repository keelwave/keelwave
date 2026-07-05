package alerting

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/keelwave/keelwave/internal/store"
)

func TestEvaluateRunFinish_loopFiresThenCooldownSuppresses(t *testing.T) {
	ctx := context.Background()
	pool := testPool
	s := store.NewStorage(pool)
	p := seedProject(t, s)

	rule := &store.AlertRule{ProjectID: p.ID, Name: "loop", Class: "event",
		Signal: "loop", Comparator: ">", Threshold: 0, Severity: "page",
		Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`), Enabled: true,
		CooldownSeconds: 900}
	require.NoError(t, s.AlertRules.Create(ctx, rule))

	ev := NewEvaluator(pool, s, zap.NewNop().Sugar())

	// A finished run with a detected loop fires the rule: one job queued.
	require.NoError(t, ev.EvaluateRunFinish(ctx, p.ID, "x", true, "completed"))
	jobs, err := s.NotificationJobs.Claim(ctx, 10)
	require.NoError(t, err)
	require.Len(t, jobs, 1, "loop event rule should enqueue one notification")
	require.NoError(t, s.NotificationJobs.MarkDone(ctx, jobs[0].ID))

	// Immediate re-fire within the cooldown window is suppressed: no new job.
	require.NoError(t, ev.EvaluateRunFinish(ctx, p.ID, "x", true, "completed"))
	jobs, err = s.NotificationJobs.Claim(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, jobs, 0, "second finish within cooldown must be suppressed")
}

// TestEvaluateRunFinish_continuesPastFirstRule proves the per-rule loop does not
// abort after the first rule: two enabled event rules matching the same finished
// run must BOTH fire (each enqueues its own job). A mid-loop `return err` would
// leave the second rule un-evaluated (missed page).
func TestEvaluateRunFinish_continuesPastFirstRule(t *testing.T) {
	ctx := context.Background()
	pool := testPool
	s := store.NewStorage(pool)
	p := seedProject(t, s)

	for _, name := range []string{"loop A", "loop B"} {
		rule := &store.AlertRule{ProjectID: p.ID, Name: name, Class: "event",
			Signal: "loop", Comparator: ">", Threshold: 0, Severity: "page",
			Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`), Enabled: true,
			CooldownSeconds: 900}
		require.NoError(t, s.AlertRules.Create(ctx, rule))
	}

	ev := NewEvaluator(pool, s, zap.NewNop().Sugar())

	require.NoError(t, ev.EvaluateRunFinish(ctx, p.ID, "x", true, "completed"))
	jobs, err := s.NotificationJobs.Claim(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, jobs, 2, "both matching event rules fire — loop must not abort after the first")
}
