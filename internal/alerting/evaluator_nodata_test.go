package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/keelwave/keelwave/internal/store"
)

// seedRunningRun inserts an agent_run at now() and leaves it unfinished (status
// "running", no duration), so it contributes to total_runs but not to the
// completed/failed denominator.
func seedRunningRun(t *testing.T, s store.Storage, projectID uuid.UUID, agentName string) {
	t.Helper()
	run := &store.AgentRun{
		ProjectID: projectID,
		AgentName: agentName,
		Status:    "running",
		Timestamp: time.Now(),
	}
	require.NoError(t, s.AgentRuns.Insert(context.Background(), run))
}

// An empty window (count==0) must never fire, whatever the comparator direction
// — the evaluator short-circuits to not-breached before compare().
func TestEvaluator_emptyWindowNeverFires(t *testing.T) {
	pool := testPool
	s := store.NewStorage(pool)
	p := seedProject(t, s)
	ev := NewEvaluator(pool, s, zap.NewNop().Sugar())

	win := 300
	// No runs seeded. A completion rule with threshold high enough that the
	// coalesced no-data value (1) would breach a naive `>` check. count==0 must
	// short-circuit to not-breached.
	rule := &store.AlertRule{ProjectID: p.ID, AgentName: new("nobody"), Name: "completion-empty",
		Class: "aggregate", Signal: "run_failure", Comparator: ">", Threshold: 0.5,
		Severity: "page", Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`),
		Enabled: true, WindowSeconds: &win, ForSeconds: 0}
	assertSilent(t, ev, s, rule)
}

// A firing alert whose volume drops below min_requests must advance off
// "firing" (recover), not freeze there forever.
func TestEvaluator_firingRecoversWhenVolumeDrops(t *testing.T) {
	pool := testPool
	s := store.NewStorage(pool)
	p := seedProject(t, s)
	ev := NewEvaluator(pool, s, zap.NewNop().Sugar())
	ctx := context.Background()

	// Seed enough failed volume for agent "x" to fire a termination_shift rule.
	seedFinishedRunWithTermination(t, s, p.ID, "x", "failed", "error", 100)
	seedFinishedRunWithTermination(t, s, p.ID, "x", "failed", "error", 100)

	win := 300
	rule := &store.AlertRule{ProjectID: p.ID, AgentName: new("x"), Name: "term-vol",
		Class: "aggregate", Signal: "termination_shift", Comparator: ">", Threshold: 0.2,
		Severity: "page", Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`),
		Enabled: true, WindowSeconds: &win, ForSeconds: 0, KeepFiringForSeconds: 0,
		MinRequests: 10}
	require.NoError(t, s.AlertRules.Create(ctx, rule))

	// Force the event into "firing" directly (bypassing the min_requests gate that
	// would otherwise stop it firing), then re-evaluate: with only 2 runs < the
	// min_requests of 10, the old code skipped and left it firing forever.
	fp := Fingerprint(rule.ID, "x")
	now := time.Now()
	fired := &store.AlertEvent{
		RuleID: rule.ID, ProjectID: p.ID, Fingerprint: fp, ScopeLabel: "x",
		State: "firing", FiredAt: &now, FirstBreachedAt: &now, LastEvaluatedAt: now,
	}
	require.NoError(t, withPoolTx(pool, ctx, func(tx pgx.Tx) error {
		return s.AlertEvents.Upsert(ctx, tx, fired)
	}))

	require.NoError(t, ev.EvaluateAggregate(ctx, rule))

	// The row must have advanced off "firing" (to recovering or, with
	// keep_firing_for=0, all the way to resolved). Read the latest row for the
	// project regardless of state — GetLive would hide a resolved row.
	events, err := s.AlertEvents.ListByProject(ctx, p.ID, 10)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	assert.NotEqual(t, "firing", events[0].State,
		"below min_requests, a firing alert must advance off firing (recover/resolve), not freeze")
}

// run_failure completion rate is computed over finished runs only — in-flight
// (running) runs must not depress the denominator.
func TestEvaluator_runFailureExcludesInFlight(t *testing.T) {
	pool := testPool
	s := store.NewStorage(pool)
	p := seedProject(t, s)
	ev := NewEvaluator(pool, s, zap.NewNop().Sugar())

	// 8 completed + 2 still running. Completion over finished = 8/8 = 1.0.
	// Over total = 8/10 = 0.8, which would falsely trip a `< 0.9` rule.
	for range 8 {
		seedFinishedRunWithTermination(t, s, p.ID, "x", "completed", "clean", 100)
	}
	seedRunningRun(t, s, p.ID, "x")
	seedRunningRun(t, s, p.ID, "x")

	win := 300
	rule := &store.AlertRule{ProjectID: p.ID, AgentName: new("x"), Name: "completion-inflight",
		Class: "aggregate", Signal: "run_failure", Comparator: "<", Threshold: 0.9,
		Severity: "page", Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`),
		Enabled: true, WindowSeconds: &win, ForSeconds: 0, MinRequests: 1}
	assertSilent(t, ev, s, rule)
}
