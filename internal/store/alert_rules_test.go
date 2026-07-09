package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlertRuleStore_CreateGetListUpdateDelete(t *testing.T) {
	ctx := context.Background()
	s := testStorage(t)
	p := testProject(t, s, "alertrule")

	win := 300
	r := &AlertRule{
		ProjectID: p.ID, Name: "loop page", Class: "event", Signal: "loop",
		Comparator: ">", Threshold: 0, Severity: "page", CooldownSeconds: 900,
		Channel: "email", ChannelConfig: []byte(`{"to":"a@b.c"}`), Enabled: true,
		WindowSeconds: nil,
	}
	require.NoError(t, s.AlertRules.Create(ctx, r))
	require.NotEqual(t, "00000000-0000-0000-0000-000000000000", r.ID.String())

	got, err := s.AlertRules.GetByID(ctx, r.ID, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "loop page", got.Name)
	assert.Equal(t, "event", got.Class)

	list, err := s.AlertRules.ListByProject(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)

	agg := &AlertRule{
		ProjectID: p.ID, Name: "cost", Class: "aggregate", Signal: "cost_burn",
		Comparator: ">", Threshold: 5, Severity: "page", Channel: "email",
		ChannelConfig: []byte(`{}`), Enabled: true, WindowSeconds: &win,
	}
	require.NoError(t, s.AlertRules.Create(ctx, agg))
	enabled, err := s.AlertRules.ListEnabled(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(enabled), 2)

	got.Name = "loop page v2"
	got.Enabled = false
	require.NoError(t, s.AlertRules.Update(ctx, got))
	after, err := s.AlertRules.GetByID(ctx, got.ID, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "loop page v2", after.Name)
	assert.False(t, after.Enabled)

	require.NoError(t, s.AlertRules.Delete(ctx, r.ID, p.ID))
	_, err = s.AlertRules.GetByID(ctx, r.ID, p.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}
