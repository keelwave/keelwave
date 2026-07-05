package alerting

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/keelwave/keelwave/internal/store"
)

// scopeOf mirrors queryMetric's scope resolution: a scoped rule labels the alert
// stream with its agent, an unscoped rule aggregates project-wide (scope "").
func scopeOf(rule *store.AlertRule) string {
	if rule.AgentName != nil {
		return *rule.AgentName
	}
	return ""
}

// assertFires creates the rule, evaluates it once, and asserts a live firing
// event exists on the rule's scope. Returns the event for further assertions.
func assertFires(t *testing.T, ev *Evaluator, s store.Storage, rule *store.AlertRule) *store.AlertEvent {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, s.AlertRules.Create(ctx, rule))
	require.NoError(t, ev.EvaluateAggregate(ctx, rule))
	live, err := s.AlertEvents.GetLive(ctx, rule.ID, Fingerprint(rule.ID, scopeOf(rule)))
	require.NoError(t, err)
	require.NotNil(t, live)
	assert.Equal(t, "firing", live.State)
	return live
}

// assertSilent creates the rule, evaluates it once, and asserts no live event was
// written (the condition never breached).
func assertSilent(t *testing.T, ev *Evaluator, s store.Storage, rule *store.AlertRule) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, s.AlertRules.Create(ctx, rule))
	require.NoError(t, ev.EvaluateAggregate(ctx, rule))
	_, err := s.AlertEvents.GetLive(ctx, rule.ID, Fingerprint(rule.ID, scopeOf(rule)))
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func agentPtr(name string) *string { return &name }

func TestEvaluator_runFailureFiresBelowCompletionThreshold(t *testing.T) {
	pool := testPool
	s := store.NewStorage(pool)
	p := seedProject(t, s)
	ev := NewEvaluator(pool, s, zap.NewNop().Sugar())

	// 3 completed + 1 failed for agent "x" => completion rate 0.75.
	seedFinishedRunWithTermination(t, s, p.ID, "x", "completed", "clean", 100)
	seedFinishedRunWithTermination(t, s, p.ID, "x", "completed", "clean", 100)
	seedFinishedRunWithTermination(t, s, p.ID, "x", "completed", "clean", 100)
	seedFinishedRunWithTermination(t, s, p.ID, "x", "failed", "error", 100)

	win := 300
	// Fires: completion rate 0.75 drops below the 0.9 threshold (comparator <).
	fire := &store.AlertRule{ProjectID: p.ID, AgentName: agentPtr("x"), Name: "completion",
		Class: "aggregate", Signal: "run_failure", Comparator: "<", Threshold: 0.9,
		Severity: "page", Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`),
		Enabled: true, WindowSeconds: &win, ForSeconds: 0, MinRequests: 1}
	live := assertFires(t, ev, s, fire)
	require.NotNil(t, live.LastValue)
	assert.InDelta(t, 0.75, *live.LastValue, 1e-9)

	// Silent: 0.75 is not below 0.5.
	silent := &store.AlertRule{ProjectID: p.ID, AgentName: agentPtr("x"), Name: "completion-lo",
		Class: "aggregate", Signal: "run_failure", Comparator: "<", Threshold: 0.5,
		Severity: "page", Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`),
		Enabled: true, WindowSeconds: &win, ForSeconds: 0, MinRequests: 1}
	assertSilent(t, ev, s, silent)
}

func TestEvaluator_terminationShiftFires(t *testing.T) {
	pool := testPool
	s := store.NewStorage(pool)
	p := seedProject(t, s)
	ev := NewEvaluator(pool, s, zap.NewNop().Sugar())

	// 1 error-terminated + 3 clean => bad-termination share 0.25.
	seedFinishedRunWithTermination(t, s, p.ID, "x", "failed", "error", 100)
	seedFinishedRunWithTermination(t, s, p.ID, "x", "completed", "clean", 100)
	seedFinishedRunWithTermination(t, s, p.ID, "x", "completed", "clean", 100)
	seedFinishedRunWithTermination(t, s, p.ID, "x", "completed", "clean", 100)

	win := 300
	fire := &store.AlertRule{ProjectID: p.ID, AgentName: agentPtr("x"), Name: "term",
		Class: "aggregate", Signal: "termination_shift", Comparator: ">", Threshold: 0.2,
		Severity: "page", Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`),
		Enabled: true, WindowSeconds: &win, ForSeconds: 0, MinRequests: 1}
	live := assertFires(t, ev, s, fire)
	require.NotNil(t, live.LastValue)
	assert.InDelta(t, 0.25, *live.LastValue, 1e-9)

	silent := &store.AlertRule{ProjectID: p.ID, AgentName: agentPtr("x"), Name: "term-hi",
		Class: "aggregate", Signal: "termination_shift", Comparator: ">", Threshold: 0.5,
		Severity: "page", Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`),
		Enabled: true, WindowSeconds: &win, ForSeconds: 0, MinRequests: 1}
	assertSilent(t, ev, s, silent)
}

func TestEvaluator_durationP95Fires(t *testing.T) {
	pool := testPool
	s := store.NewStorage(pool)
	p := seedProject(t, s)
	ev := NewEvaluator(pool, s, zap.NewNop().Sugar())

	// durations [100,200,300,5000] => percentile_disc(0.95) = 5000.
	seedFinishedRunWithTermination(t, s, p.ID, "x", "completed", "clean", 100)
	seedFinishedRunWithTermination(t, s, p.ID, "x", "completed", "clean", 200)
	seedFinishedRunWithTermination(t, s, p.ID, "x", "completed", "clean", 300)
	seedFinishedRunWithTermination(t, s, p.ID, "x", "completed", "clean", 5000)

	win := 300
	fire := &store.AlertRule{ProjectID: p.ID, AgentName: agentPtr("x"), Name: "p95",
		Class: "aggregate", Signal: "duration_p95", Comparator: ">", Threshold: 1000,
		Severity: "page", Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`),
		Enabled: true, WindowSeconds: &win, ForSeconds: 0, MinRequests: 1}
	live := assertFires(t, ev, s, fire)
	require.NotNil(t, live.LastValue)
	assert.InDelta(t, 5000, *live.LastValue, 1e-9)

	silent := &store.AlertRule{ProjectID: p.ID, AgentName: agentPtr("x"), Name: "p95-hi",
		Class: "aggregate", Signal: "duration_p95", Comparator: ">", Threshold: 10000,
		Severity: "page", Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`),
		Enabled: true, WindowSeconds: &win, ForSeconds: 0, MinRequests: 1}
	assertSilent(t, ev, s, silent)
}

func TestEvaluator_toolFailureFires(t *testing.T) {
	pool := testPool
	s := store.NewStorage(pool)
	p := seedProject(t, s)
	ev := NewEvaluator(pool, s, zap.NewNop().Sugar())

	// 3 failed + 1 successful tool call on agent "x" => fail rate 0.75.
	runID := seedFinishedRunWithTermination(t, s, p.ID, "x", "completed", "clean", 100)
	seedToolStep(t, s, p.ID, runID, 0, "search", false)
	seedToolStep(t, s, p.ID, runID, 1, "search", false)
	seedToolStep(t, s, p.ID, runID, 2, "search", false)
	seedToolStep(t, s, p.ID, runID, 3, "search", true)

	win := 300
	fire := &store.AlertRule{ProjectID: p.ID, AgentName: agentPtr("x"), Name: "tool",
		Class: "aggregate", Signal: "tool_failure", Comparator: ">", Threshold: 0.5,
		Severity: "page", Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`),
		Enabled: true, WindowSeconds: &win, ForSeconds: 0, MinRequests: 1}
	live := assertFires(t, ev, s, fire)
	require.NotNil(t, live.LastValue)
	assert.InDelta(t, 0.75, *live.LastValue, 1e-9)

	silent := &store.AlertRule{ProjectID: p.ID, AgentName: agentPtr("x"), Name: "tool-hi",
		Class: "aggregate", Signal: "tool_failure", Comparator: ">", Threshold: 0.9,
		Severity: "page", Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`),
		Enabled: true, WindowSeconds: &win, ForSeconds: 0, MinRequests: 1}
	assertSilent(t, ev, s, silent)
}

func TestEvaluator_toolFailureMinRequestsGuardSkips(t *testing.T) {
	pool := testPool
	s := store.NewStorage(pool)
	p := seedProject(t, s)
	ev := NewEvaluator(pool, s, zap.NewNop().Sugar())

	// Only 2 tool calls: below min_requests=5, so the rule must not fire even
	// though the fail rate (0.5) crosses the threshold.
	runID := seedFinishedRunWithTermination(t, s, p.ID, "x", "completed", "clean", 100)
	seedToolStep(t, s, p.ID, runID, 0, "search", false)
	seedToolStep(t, s, p.ID, runID, 1, "search", true)

	win := 300
	rule := &store.AlertRule{ProjectID: p.ID, AgentName: agentPtr("x"), Name: "tool-guard",
		Class: "aggregate", Signal: "tool_failure", Comparator: ">", Threshold: 0.1,
		Severity: "page", Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`),
		Enabled: true, WindowSeconds: &win, ForSeconds: 0, MinRequests: 5}
	assertSilent(t, ev, s, rule)
}

func TestEvaluator_evalRegressionFiresBelowThreshold(t *testing.T) {
	pool := testPool
	s := store.NewStorage(pool)
	p := seedProject(t, s)
	ev := NewEvaluator(pool, s, zap.NewNop().Sugar())

	// correctness [0.4,0.5,0.6] => avg 0.5.
	r1 := seedFinishedRunWithTermination(t, s, p.ID, "x", "completed", "clean", 100)
	r2 := seedFinishedRunWithTermination(t, s, p.ID, "x", "completed", "clean", 100)
	r3 := seedFinishedRunWithTermination(t, s, p.ID, "x", "completed", "clean", 100)
	seedEval(t, s, p.ID, r1, 0.4)
	seedEval(t, s, p.ID, r2, 0.5)
	seedEval(t, s, p.ID, r3, 0.6)

	win := 300
	fire := &store.AlertRule{ProjectID: p.ID, AgentName: agentPtr("x"), Name: "eval",
		Class: "aggregate", Signal: "eval_regression", Comparator: "<", Threshold: 0.7,
		Severity: "page", Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`),
		Enabled: true, WindowSeconds: &win, ForSeconds: 0, MinRequests: 1}
	live := assertFires(t, ev, s, fire)
	require.NotNil(t, live.LastValue)
	assert.InDelta(t, 0.5, *live.LastValue, 1e-9)

	silent := &store.AlertRule{ProjectID: p.ID, AgentName: agentPtr("x"), Name: "eval-lo",
		Class: "aggregate", Signal: "eval_regression", Comparator: "<", Threshold: 0.3,
		Severity: "page", Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`),
		Enabled: true, WindowSeconds: &win, ForSeconds: 0, MinRequests: 1}
	assertSilent(t, ev, s, silent)
}

// An unscoped rule (AgentName == nil) aggregates project-wide: the two agents'
// costs sum into a single firing event labelled with the empty scope.
func TestEvaluator_unscopedAggregatesProjectWide(t *testing.T) {
	pool := testPool
	s := store.NewStorage(pool)
	p := seedProject(t, s)
	ev := NewEvaluator(pool, s, zap.NewNop().Sugar())

	seedFinishedRun(t, s, p.ID, "a", 4.0)
	seedFinishedRun(t, s, p.ID, "b", 4.0)

	win := 300
	// Neither agent alone ($4) crosses $5, but project-wide ($8) does.
	unscoped := &store.AlertRule{ProjectID: p.ID, Name: "cost-all",
		Class: "aggregate", Signal: "cost_burn", Comparator: ">", Threshold: 5,
		Severity: "page", Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`),
		Enabled: true, WindowSeconds: &win, ForSeconds: 0}
	live := assertFires(t, ev, s, unscoped)
	assert.Equal(t, "", live.ScopeLabel, "unscoped rule fires on the project-wide scope")
	require.NotNil(t, live.LastValue)
	assert.InDelta(t, 8.0, *live.LastValue, 1e-9)

	// A rule scoped to a single agent sees only that agent's $4 and stays silent.
	scoped := &store.AlertRule{ProjectID: p.ID, AgentName: agentPtr("a"), Name: "cost-a",
		Class: "aggregate", Signal: "cost_burn", Comparator: ">", Threshold: 5,
		Severity: "page", Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`),
		Enabled: true, WindowSeconds: &win, ForSeconds: 0}
	assertSilent(t, ev, s, scoped)
}
