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

func TestAlertEventStore_ListByProjectFiltered(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "alertfilter")
	rule := &AlertRule{ProjectID: p.ID, Name: "r", Class: "aggregate", Signal: "cost_burn",
		Comparator: ">", Threshold: 1, Severity: "page", Channel: "email",
		ChannelConfig: []byte(`{}`), Enabled: true}
	require.NoError(t, s.AlertRules.Create(ctx, rule))

	now := time.Now()
	firing := &AlertEvent{RuleID: rule.ID, ProjectID: p.ID, Fingerprint: []byte("fp-firing"),
		ScopeLabel: "agent-a", State: "firing", FiredAt: &now, LastEvaluatedAt: now}
	resolved := &AlertEvent{RuleID: rule.ID, ProjectID: p.ID, Fingerprint: []byte("fp-resolved"),
		ScopeLabel: "agent-b", State: "resolved", FiredAt: &now, ResolvedAt: &now, LastEvaluatedAt: now}
	for _, e := range []*AlertEvent{firing, resolved} {
		require.NoError(t, withTx(testPool, ctx, func(tx pgx.Tx) error {
			return s.AlertEvents.Upsert(ctx, tx, e)
		}))
	}

	// Two jobs for the firing alert; only the newest must surface.
	require.NoError(t, withTx(testPool, ctx, func(tx pgx.Tx) error {
		older := &NotificationJob{AlertEventID: firing.ID, Channel: "email",
			Payload: []byte(`{}`), RunAfter: now.Add(-time.Minute)}
		if err := s.NotificationJobs.Enqueue(ctx, tx, older); err != nil {
			return err
		}
		newest := &NotificationJob{AlertEventID: firing.ID, Channel: "email",
			Payload: []byte(`{}`), RunAfter: now}
		return s.NotificationJobs.Enqueue(ctx, tx, newest)
	}))

	all, err := s.AlertEvents.ListByProjectFiltered(ctx, p.ID, "", 50)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	active, err := s.AlertEvents.ListByProjectFiltered(ctx, p.ID, "active", 50)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "firing", active[0].State)
	require.NotNil(t, active[0].Delivery, "firing alert carries its latest notification job")
	assert.Equal(t, "pending", active[0].Delivery.Status)

	res, err := s.AlertEvents.ListByProjectFiltered(ctx, p.ID, "resolved", 50)
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, "resolved", res[0].State)
	assert.Nil(t, res[0].Delivery, "no job for this alert -> nil delivery")

	limited, err := s.AlertEvents.ListByProjectFiltered(ctx, p.ID, "", 1)
	require.NoError(t, err)
	assert.Len(t, limited, 1)
}
