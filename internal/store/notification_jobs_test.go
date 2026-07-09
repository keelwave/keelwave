package store

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationJobStore_EnqueueClaimComplete(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "notifjob")
	rule := &AlertRule{ProjectID: p.ID, Name: "r", Class: "event", Signal: "loop",
		Comparator: ">", Threshold: 0, Severity: "page", Channel: "email",
		ChannelConfig: []byte(`{}`), Enabled: true}
	require.NoError(t, s.AlertRules.Create(ctx, rule))
	now := time.Now()
	ev := &AlertEvent{RuleID: rule.ID, ProjectID: p.ID, Fingerprint: []byte("fp"),
		State: "fired", LastFiredAt: &now, LastEvaluatedAt: now}

	// enqueue in the same tx as the event write (outbox)
	err := withTx(testPool, ctx, func(tx pgx.Tx) error {
		if err := s.AlertEvents.Upsert(ctx, tx, ev); err != nil {
			return err
		}
		return s.NotificationJobs.Enqueue(ctx, tx, &NotificationJob{
			AlertEventID: ev.ID, Channel: "email", Payload: []byte(`{"to":"a@b.c"}`), RunAfter: now,
		})
	})
	require.NoError(t, err)

	claimed, err := s.NotificationJobs.Claim(ctx, 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, "email", claimed[0].Channel)

	require.NoError(t, s.NotificationJobs.MarkDone(ctx, claimed[0].ID))
	again, err := s.NotificationJobs.Claim(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, again, 0) // done job not re-claimed
}
