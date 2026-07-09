package alerting

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/keelwave/keelwave/internal/store"
)

// Two concurrent run-finishes hitting the same event rule with NO prior event
// must produce exactly ONE notification_job — the partial unique index + ON
// CONFLICT arbiter makes only one INSERT win, and the loser enqueues nothing.
func TestEvaluateRunFinish_concurrentFirstFireEnqueuesOnce(t *testing.T) {
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

	runConcurrentFinishes(t, ev, p.ID)

	jobs, err := s.NotificationJobs.Claim(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, jobs, 1, "concurrent first-fires must enqueue exactly one job")
}

// With a live fired event whose last_fired_at is far in the past (cooldown long
// elapsed), two concurrent finishes must still enqueue exactly ONE job — the
// cooldown guard lives in the atomic FireEvent upsert, not just the read-side
// NextEvent check, so both readers seeing the stale last_fired_at can't both fire.
func TestEvaluateRunFinish_concurrentReFireEnqueuesOnce(t *testing.T) {
	ctx := context.Background()
	pool := testPool
	s := store.NewStorage(pool)
	p := seedProject(t, s)

	rule := &store.AlertRule{ProjectID: p.ID, Name: "loop", Class: "event",
		Signal: "loop", Comparator: ">", Threshold: 0, Severity: "page",
		Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`), Enabled: true,
		CooldownSeconds: 60}
	require.NoError(t, s.AlertRules.Create(ctx, rule))

	// Seed a live fired event whose last_fired_at is well past the cooldown.
	fp := Fingerprint(rule.ID, "x")
	old := time.Now().Add(-2 * time.Hour)
	live := &store.AlertEvent{
		RuleID: rule.ID, ProjectID: p.ID, Fingerprint: fp, ScopeLabel: "x",
		State: "fired", FiredAt: &old, LastFiredAt: &old, FirstBreachedAt: &old,
		LastEvaluatedAt: old,
	}
	require.NoError(t, withPoolTx(pool, ctx, func(tx pgx.Tx) error {
		return s.AlertEvents.Upsert(ctx, tx, live)
	}))

	ev := NewEvaluator(pool, s, zap.NewNop().Sugar())
	runConcurrentFinishes(t, ev, p.ID)

	jobs, err := s.NotificationJobs.Claim(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, jobs, 1, "concurrent re-fires past cooldown must enqueue exactly one job")
}

// runConcurrentFinishes releases two EvaluateRunFinish calls for agent "x"
// together via a start barrier to maximise the race window.
func runConcurrentFinishes(t *testing.T, ev *Evaluator, projectID uuid.UUID) {
	t.Helper()
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			errs[idx] = ev.EvaluateRunFinish(context.Background(), projectID, "x", true, "completed")
		}(i)
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
}
