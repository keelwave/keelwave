package store

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlertEventStore_UpsertGetLive(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "alertevent")
	rule := &AlertRule{ProjectID: p.ID, Name: "r", Class: "aggregate", Signal: "cost_burn",
		Comparator: ">", Threshold: 1, Severity: "page", Channel: "email",
		ChannelConfig: []byte(`{}`), Enabled: true}
	require.NoError(t, s.AlertRules.Create(ctx, rule))

	fp := []byte("fingerprint-1")
	now := time.Now()
	e := &AlertEvent{RuleID: rule.ID, ProjectID: p.ID, Fingerprint: fp, ScopeLabel: "agent-x",
		State: "pending", FirstBreachedAt: &now, LastEvaluatedAt: now}

	err := withTx(testPool, ctx, func(tx pgx.Tx) error { return s.AlertEvents.Upsert(ctx, tx, e) })
	require.NoError(t, err)
	require.NotEqual(t, "00000000-0000-0000-0000-000000000000", e.ID.String())

	live, err := s.AlertEvents.GetLive(ctx, rule.ID, fp)
	require.NoError(t, err)
	assert.Equal(t, "pending", live.State)

	// transition to firing on the same live row (same id)
	live.State = "firing"
	live.FiredAt = &now
	err = withTx(testPool, ctx, func(tx pgx.Tx) error { return s.AlertEvents.Upsert(ctx, tx, live) })
	require.NoError(t, err)
	again, err := s.AlertEvents.GetLive(ctx, rule.ID, fp)
	require.NoError(t, err)
	assert.Equal(t, "firing", again.State)
	assert.Equal(t, live.ID, again.ID)

	// condition clears -> recovering, stamping recovering_since (round-trips)
	recSince := now.Add(-30 * time.Second)
	again.State = "recovering"
	again.RecoveringSince = &recSince
	require.NoError(t, withTx(testPool, ctx, func(tx pgx.Tx) error { return s.AlertEvents.Upsert(ctx, tx, again) }))
	rec, err := s.AlertEvents.GetLive(ctx, rule.ID, fp)
	require.NoError(t, err)
	assert.Equal(t, "recovering", rec.State)
	require.NotNil(t, rec.RecoveringSince)
	assert.WithinDuration(t, recSince, *rec.RecoveringSince, time.Second)

	// resolving removes it from "live"
	rec.State = "resolved"
	rec.ResolvedAt = &now
	require.NoError(t, withTx(testPool, ctx, func(tx pgx.Tx) error { return s.AlertEvents.Upsert(ctx, tx, rec) }))
	_, err = s.AlertEvents.GetLive(ctx, rule.ID, fp)
	assert.ErrorIs(t, err, ErrNotFound)
}
