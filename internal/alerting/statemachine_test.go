package alerting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNextAggregate_pendingThenFireAfterFor(t *testing.T) {
	now := time.Now()
	rule := Rule{ForSeconds: 60}
	// first breach: inactive -> pending, no notify
	d := NextAggregate(rule, nil, Eval{Breached: true, Value: 9, Now: now})
	assert.Equal(t, "pending", d.NextState)
	assert.Equal(t, "", d.Notify)

	// still breaching but `for` not elapsed: stay pending
	breached := now.Add(-30 * time.Second)
	ev := &Event{State: "pending", FirstBreachedAt: &breached}
	d = NextAggregate(rule, ev, Eval{Breached: true, Value: 9, Now: now})
	assert.Equal(t, "pending", d.NextState)

	// `for` elapsed: pending -> firing, notify fire
	breached = now.Add(-90 * time.Second)
	ev = &Event{State: "pending", FirstBreachedAt: &breached}
	d = NextAggregate(rule, ev, Eval{Breached: true, Value: 9, Now: now})
	assert.Equal(t, "firing", d.NextState)
	assert.Equal(t, "fire", d.Notify)
}

func TestNextAggregate_resolveWithHysteresis(t *testing.T) {
	now := time.Now()
	rule := Rule{KeepFiringForSeconds: 60}
	fired := now.Add(-10 * time.Minute)
	ev := &Event{State: "firing", FiredAt: &fired}
	// condition clears -> recovering (not resolved yet)
	d := NextAggregate(rule, ev, Eval{Breached: false, Now: now})
	assert.Equal(t, "recovering", d.NextState)
	assert.Equal(t, "", d.Notify)
}

func TestNextAggregate_recoveringResolvePath(t *testing.T) {
	now := time.Now()
	fired := now.Add(-10 * time.Minute) // original fire time, long ago
	tenAgo := now.Add(-10 * time.Second)
	twoMinAgo := now.Add(-120 * time.Second)

	tests := []struct {
		name      string
		rule      Rule
		ev        *Event
		wantState string
		wantNotif string
	}{
		{
			name:      "firing -> recovering when condition clears",
			rule:      Rule{KeepFiringForSeconds: 60},
			ev:        &Event{State: "firing", FiredAt: &fired},
			wantState: "recovering",
			wantNotif: "",
		},
		{
			name:      "recovering holds before keep_firing_for elapses",
			rule:      Rule{KeepFiringForSeconds: 60},
			ev:        &Event{State: "recovering", FiredAt: &fired, RecoveringSince: &tenAgo},
			wantState: "recovering",
			wantNotif: "",
		},
		{
			name:      "recovering -> resolved after keep_firing_for elapses",
			rule:      Rule{KeepFiringForSeconds: 60},
			ev:        &Event{State: "recovering", FiredAt: &fired, RecoveringSince: &twoMinAgo},
			wantState: "resolved",
			wantNotif: "resolve",
		},
		{
			name:      "recovering with nil RecoveringSince stays recovering",
			rule:      Rule{KeepFiringForSeconds: 60},
			ev:        &Event{State: "recovering", FiredAt: &fired, RecoveringSince: nil},
			wantState: "recovering",
			wantNotif: "",
		},
		{
			name:      "pending clears before firing -> resolved, no notify",
			rule:      Rule{ForSeconds: 60},
			ev:        &Event{State: "pending", FirstBreachedAt: &tenAgo},
			wantState: "resolved",
			wantNotif: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NextAggregate(tt.rule, tt.ev, Eval{Breached: false, Now: now})
			assert.Equal(t, tt.wantState, d.NextState)
			assert.Equal(t, tt.wantNotif, d.Notify)
		})
	}
}

func TestNextEvent_notifyOnceWithinCooldown(t *testing.T) {
	now := time.Now()
	rule := Rule{CooldownSeconds: 900}
	// no prior event: fire
	d := NextEvent(rule, nil, Eval{Breached: true, Now: now})
	assert.Equal(t, "fired", d.NextState)
	assert.Equal(t, "fire", d.Notify)

	// within cooldown: suppress
	last := now.Add(-5 * time.Minute)
	ev := &Event{State: "fired", LastFiredAt: &last}
	d = NextEvent(rule, ev, Eval{Breached: true, Now: now})
	assert.Equal(t, "fired", d.NextState)
	assert.Equal(t, "", d.Notify)

	// after cooldown: fire again
	last = now.Add(-20 * time.Minute)
	ev = &Event{State: "fired", LastFiredAt: &last}
	d = NextEvent(rule, ev, Eval{Breached: true, Now: now})
	assert.Equal(t, "fire", d.Notify)
}
