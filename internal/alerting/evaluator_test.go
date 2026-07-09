package alerting

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/keelwave/keelwave/internal/store"
)

func TestBuildPayload_MergesRecipientAndKeys(t *testing.T) {
	win := 300
	rule := &store.AlertRule{
		Name: "cost", Signal: "cost_burn", Severity: "page",
		Comparator: ">", Threshold: 5, WindowSeconds: &win,
		ChannelConfig: json.RawMessage(`{"to":"ops@acme.com"}`),
	}

	b, err := buildPayload(rule, "agent-x", 9.0, "fire", time.Now())
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	assert.Equal(t, "ops@acme.com", m["to"], "recipient from channel_config must flow through")
	assert.Equal(t, rule.Name, m["rule_name"])
	assert.Equal(t, "fire", m["transition"])
	assert.Contains(t, m, "window", "window must be present when window_seconds is set")
}

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

	// The rule is unscoped (no agent_name), so it aggregates project-wide under
	// the empty scope: a live firing event + a queued email job exist.
	fp := Fingerprint(rule.ID, "")
	live, err := s.AlertEvents.GetLive(ctx, rule.ID, fp)
	require.NoError(t, err)
	assert.Equal(t, "firing", live.State)

	jobs, err := s.NotificationJobs.Claim(ctx, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(jobs), 1)
}
