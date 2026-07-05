package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentRunStore_GetByID_returnsRowForProject(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "getbyid")

	input := "find X"
	run := &AgentRun{
		ProjectID: p.ID,
		AgentName: "researcher",
		Status:    "running",
		Input:     &input,
	}
	require.NoError(t, s.AgentRuns.Insert(ctx, run))

	from := run.Timestamp.Add(-time.Hour)
	to := run.Timestamp.Add(time.Hour)
	got, err := s.AgentRuns.GetByID(ctx, p.ID, run.ID, from, to)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, run.ID, got.ID)
	assert.Equal(t, p.ID, got.ProjectID)
	assert.Equal(t, "researcher", got.AgentName)
	assert.Equal(t, "running", got.Status)
	require.NotNil(t, got.Input)
	assert.Equal(t, "find X", *got.Input)
}

func TestAgentRunStore_GetByID_returnsNotFoundForOtherProject(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	owner := testProject(t, s, "owner")
	other := testProject(t, s, "other")

	run := &AgentRun{ProjectID: owner.ID, AgentName: "a", Status: "running"}
	require.NoError(t, s.AgentRuns.Insert(ctx, run))

	from := run.Timestamp.Add(-time.Hour)
	to := run.Timestamp.Add(time.Hour)
	got, err := s.AgentRuns.GetByID(ctx, other.ID, run.ID, from, to)

	assert.ErrorIs(t, err, ErrNotFound)
	assert.Nil(t, got)
}

func TestAgentRunStore_Insert_assignsIDAndDefaultTimestamp(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "runinsert")

	run := &AgentRun{ProjectID: p.ID, AgentName: "a", Status: "running"}
	require.NoError(t, s.AgentRuns.Insert(ctx, run))

	assert.NotEqual(t, uuid.Nil, run.ID)
	assert.False(t, run.Timestamp.IsZero())
}

func TestAgentRunStore_Finish_setsTerminalFields(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "runfinish")

	run := &AgentRun{ProjectID: p.ID, AgentName: "a", Status: "running"}
	require.NoError(t, s.AgentRuns.Insert(ctx, run))

	out := "answer"
	reason := "clean"
	require.NoError(t, s.AgentRuns.Finish(ctx, run.ID, run.Timestamp, AgentRunFinish{
		Status:            "completed",
		TerminationReason: &reason,
		TotalSteps:        3,
		TotalTokens:       100,
		Output:            &out,
	}))

	got, err := s.AgentRuns.GetByID(ctx, p.ID, run.ID, run.Timestamp.Add(-time.Second), run.Timestamp.Add(time.Second))
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
	require.NotNil(t, got.TerminationReason)
	assert.Equal(t, "clean", *got.TerminationReason)
	assert.Equal(t, 3, got.TotalSteps)
	assert.Equal(t, 100, got.TotalTokens)
	require.NotNil(t, got.FinishedAt)
}

func TestAgentRunStore_Finish_notFoundOnWrongTimestamp(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "runfinish-miss")

	run := &AgentRun{ProjectID: p.ID, AgentName: "a", Status: "running"}
	require.NoError(t, s.AgentRuns.Insert(ctx, run))

	err := s.AgentRuns.Finish(ctx, run.ID, time.Now().Add(time.Hour), AgentRunFinish{Status: "completed"})
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestAgentRunStore_ListByProject_returnsEmptySliceNotNil(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "runs-empty")

	from := time.Now().Add(-time.Hour)
	to := time.Now().Add(time.Hour)
	runs, err := s.AgentRuns.ListByProject(ctx, p.ID, from, to, 10, 0)
	require.NoError(t, err)
	require.NotNil(t, runs)
	assert.Len(t, runs, 0)
}

func TestAgentRunStore_ListByProject_paginatesNewestFirst(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "list")

	now := time.Now()
	for i := 0; i < 5; i++ {
		run := &AgentRun{
			ProjectID: p.ID,
			AgentName: "a",
			Status:    "running",
			Timestamp: now.Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, s.AgentRuns.Insert(ctx, run))
	}

	from := now.Add(-time.Hour)
	to := now.Add(time.Hour)

	first, err := s.AgentRuns.ListByProject(ctx, p.ID, from, to, 2, 0)
	require.NoError(t, err)
	require.Len(t, first, 2)
	assert.True(t, first[0].Timestamp.After(first[1].Timestamp), "newest first")

	second, err := s.AgentRuns.ListByProject(ctx, p.ID, from, to, 2, 2)
	require.NoError(t, err)
	require.Len(t, second, 2)
	assert.True(t, first[1].Timestamp.After(second[0].Timestamp), "page 2 older than page 1")
}

func TestAgentRunStore_RunHealth_rollsUpPerAgentName(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "runhealth")

	// research-agent: 3 runs — 2 completed, 1 failed-with-loop
	// triage:         1 run  — completed
	mk := func(name, status string, loop bool, cost float64, tokens int) {
		run := &AgentRun{ProjectID: p.ID, AgentName: name, Status: "running"}
		require.NoError(t, s.AgentRuns.Insert(ctx, run))
		require.NoError(t, s.AgentRuns.Finish(ctx, run.ID, run.Timestamp, AgentRunFinish{
			Status:       status,
			LoopDetected: loop,
			TotalTokens:  tokens,
		}))
		// Run cost derives from linked ai_traces, not the run's stored rollup.
		require.NoError(t, s.AITraces.Insert(ctx, &AITrace{
			ProjectID: p.ID, Model: "test", Status: "success",
			AgentRunID: &run.ID, CostUSD: &cost, Timestamp: run.Timestamp,
		}))
	}
	mk("research-agent", "completed", false, 0.02, 1000)
	mk("research-agent", "completed", false, 0.04, 2000)
	mk("research-agent", "failed", true, 0.06, 3000)
	mk("triage", "completed", false, 0.01, 500)

	from := time.Now().Add(-time.Hour)
	to := time.Now().Add(time.Hour)
	rows, err := s.AgentRuns.RunHealth(ctx, p.ID, from, to)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	byName := map[string]*RunHealthRow{}
	for _, r := range rows {
		byName[r.AgentName] = r
	}

	ra := byName["research-agent"]
	require.NotNil(t, ra)
	assert.Equal(t, 3, ra.TotalRuns)
	assert.Equal(t, 2, ra.CompletedRuns)
	assert.Equal(t, 1, ra.LoopRuns)
	assert.InDelta(t, 2.0/3.0, ra.CompletionRate, 0.001)
	assert.InDelta(t, 1.0/3.0, ra.LoopRate, 0.001)
	require.NotNil(t, ra.AvgCostUSD)
	assert.InDelta(t, 0.04, *ra.AvgCostUSD, 0.001)
	assert.InDelta(t, 2000.0, ra.AvgTokens, 0.001)

	assert.Equal(t, 0, ra.PrevTotalRuns) // no runs in the prior window

	tr := byName["triage"]
	require.NotNil(t, tr)
	assert.Equal(t, 1, tr.TotalRuns)
	assert.InDelta(t, 1.0, tr.CompletionRate, 0.001)
}

func TestAgentRunStore_RunHealth_countsPriorWindow(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "runhealth-prev")

	now := time.Now()
	mk := func(name string, ts time.Time) {
		run := &AgentRun{ProjectID: p.ID, AgentName: name, Status: "running", Timestamp: ts}
		require.NoError(t, s.AgentRuns.Insert(ctx, run))
	}

	from := now.Add(-1 * time.Hour)
	to := now.Add(1 * time.Hour)
	// prevFrom = from - (to-from) = now-3h; prior window is [now-3h, now-1h).

	// Current window: 2 runs for "a".
	mk("a", now)
	mk("a", now.Add(-30*time.Minute))
	// Prior window: 3 runs for "a".
	mk("a", now.Add(-2*time.Hour))
	mk("a", now.Add(-2*time.Hour))
	mk("a", now.Add(-2*time.Hour))
	// Prior-only agent — present only before the window, must be excluded.
	mk("ghost", now.Add(-2*time.Hour))

	rows, err := s.AgentRuns.RunHealth(ctx, p.ID, from, to)
	require.NoError(t, err)
	require.Len(t, rows, 1) // ghost has no current-window runs → dropped by HAVING

	a := rows[0]
	assert.Equal(t, "a", a.AgentName)
	assert.Equal(t, 2, a.TotalRuns)
	assert.Equal(t, 3, a.PrevTotalRuns)
}

func TestAgentRunStore_RunHealth_returnsEmptySliceNotNil(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "runhealth-empty")

	rows, err := s.AgentRuns.RunHealth(ctx, p.ID,
		time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.NotNil(t, rows)
	assert.Len(t, rows, 0)
}

func TestAgentRunStore_RunsTimeseries_bucketsByInterval(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "timeseries")

	base := time.Now().Truncate(time.Hour).Add(-3 * time.Hour)
	// 3 runs in the same hour bucket: 2 completed, 1 failed+loop.
	mk := func(offsetMin int, status string, loop bool) {
		run := &AgentRun{
			ProjectID: p.ID,
			AgentName: "a",
			Status:    "running",
			Timestamp: base.Add(time.Duration(offsetMin) * time.Minute),
		}
		require.NoError(t, s.AgentRuns.Insert(ctx, run))
		require.NoError(t, s.AgentRuns.Finish(ctx, run.ID, run.Timestamp, AgentRunFinish{
			Status:       status,
			LoopDetected: loop,
		}))
	}
	mk(1, "completed", false)
	mk(5, "completed", false)
	mk(10, "failed", true)

	from := base.Add(-time.Hour)
	to := base.Add(time.Hour)
	buckets, err := s.AgentRuns.RunsTimeseries(ctx, p.ID, from, to, "1 hour")
	require.NoError(t, err)
	require.Len(t, buckets, 1, "all three runs fall in one hourly bucket")

	b := buckets[0]
	assert.Equal(t, 3, b.Total)
	assert.Equal(t, 2, b.Completed)
	assert.Equal(t, 1, b.Failed)
	assert.Equal(t, 1, b.Loop)
}

func TestAgentRunStore_Summary_aggregatesRunsAndPercentiles(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "summary")

	base := time.Now().Add(-time.Hour)
	mk := func(status string, loop bool, cost float64, tokens, steps, dur int) {
		run := &AgentRun{ProjectID: p.ID, AgentName: "a", Status: "running", Timestamp: base}
		require.NoError(t, s.AgentRuns.Insert(ctx, run))
		require.NoError(t, s.AgentRuns.Finish(ctx, run.ID, run.Timestamp, AgentRunFinish{
			Status: status, LoopDetected: loop, TotalSteps: steps,
			TotalTokens: tokens, TotalCostUSD: &cost, DurationMs: new(dur),
		}))
	}
	mk("completed", false, 0.02, 1000, 5, 100)
	mk("completed", false, 0.04, 2000, 7, 200)
	mk("failed", true, 0.06, 3000, 9, 300)

	got, prev, err := s.AgentRuns.SummaryWithPrev(ctx, p.ID, base.Add(-time.Hour), base.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 3, got.TotalRuns)
	assert.Equal(t, 2, got.CompletedRuns)
	assert.Equal(t, 1, got.LoopRuns)
	assert.InDelta(t, 2.0/3.0, got.CompletionRate, 0.001)
	assert.InDelta(t, 1.0/3.0, got.LoopRate, 0.001)
	require.NotNil(t, got.AvgCostUSD)
	assert.InDelta(t, 0.04, *got.AvgCostUSD, 0.001)
	assert.InDelta(t, 2000.0, got.AvgTokens, 0.001)
	assert.Equal(t, 21, got.TotalSteps)     // 5+7+9
	assert.Equal(t, 300, got.DurationP99Ms) // percentile_disc(0.99) of {100,200,300}
	assert.Equal(t, 200, got.DurationP50Ms)
	assert.Equal(t, 1, got.UniqueAgents) // folded in from the same scan

	// All runs sit in the current window; the prior window is empty.
	assert.Equal(t, 0, prev.TotalRuns)
	assert.Equal(t, 0, prev.UniqueAgents)
}

func TestAgentRunStore_Summary_priorWindowAggregatesSeparately(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "summary-prev")

	now := time.Now()
	mk := func(name string, ts time.Time) {
		run := &AgentRun{ProjectID: p.ID, AgentName: name, Status: "running", Timestamp: ts}
		require.NoError(t, s.AgentRuns.Insert(ctx, run))
	}

	from := now.Add(-1 * time.Hour)
	to := now.Add(1 * time.Hour)
	// prevFrom = now-3h; prior window is [now-3h, now-1h).

	mk("a", now)                    // current
	mk("b", now.Add(-30*time.Minute)) // current, distinct agent
	mk("a", now.Add(-2*time.Hour))  // prior
	mk("c", now.Add(-2*time.Hour))  // prior, distinct agent

	cur, prev, err := s.AgentRuns.SummaryWithPrev(ctx, p.ID, from, to)
	require.NoError(t, err)
	assert.Equal(t, 2, cur.TotalRuns)
	assert.Equal(t, 2, cur.UniqueAgents) // a, b
	assert.Equal(t, 2, prev.TotalRuns)
	assert.Equal(t, 2, prev.UniqueAgents) // a, c
}

func TestAgentRunStore_Summary_emptyWindowZeroed(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "summary-empty")
	got, _, err := s.AgentRuns.SummaryWithPrev(ctx, p.ID, time.Now().Add(-time.Hour), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 0, got.TotalRuns)
	assert.Equal(t, 0, got.DurationP95Ms)
	assert.Nil(t, got.AvgCostUSD)
}

func TestAgentRunStore_TerminationCounts_groupsCoalescingNull(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "terms")
	mk := func(reason *string) {
		run := &AgentRun{ProjectID: p.ID, AgentName: "a", Status: "running"}
		require.NoError(t, s.AgentRuns.Insert(ctx, run))
		require.NoError(t, s.AgentRuns.Finish(ctx, run.ID, run.Timestamp, AgentRunFinish{
			Status: "completed", TerminationReason: reason,
		}))
	}
	mk(new("clean"))
	mk(new("clean"))
	mk(new("error"))
	mk(nil) // → "unknown"

	rows, err := s.AgentRuns.TerminationCounts(ctx, p.ID,
		time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	require.NoError(t, err)
	m := map[string]int{}
	for _, r := range rows {
		m[r.TerminationReason] = r.Count
	}
	assert.Equal(t, 2, m["clean"])
	assert.Equal(t, 1, m["error"])
	assert.Equal(t, 1, m["unknown"])
}

func TestAgentRunStore_RunsTimeseries_returnsEmptySliceNotNil(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "timeseries-empty")

	buckets, err := s.AgentRuns.RunsTimeseries(ctx, p.ID,
		time.Now().Add(-time.Hour), time.Now().Add(time.Hour), "1 hour")
	require.NoError(t, err)
	require.NotNil(t, buckets)
	assert.Len(t, buckets, 0)
}
