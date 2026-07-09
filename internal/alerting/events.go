package alerting

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/keelwave/keelwave/internal/store"
)

// EvaluateRunFinish evaluates the project's enabled event-class rules against a
// just-finished run: for each matching rule (signal fits the outcome, agent filter
// matches, past cooldown) it fires a notification via the transactional outbox.
// Called off the ingest hot path.
func (ev *Evaluator) EvaluateRunFinish(ctx context.Context, projectID uuid.UUID, agentName string, loopDetected bool, status string) error {
	rules, err := ev.s.AlertRules.ListByProject(ctx, projectID)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, rule := range rules {
		if !rule.Enabled || rule.Class != "event" {
			continue
		}
		if rule.AgentName != nil && *rule.AgentName != agentName {
			continue
		}
		if !eventBreached(rule.Signal, loopDetected, status) {
			continue
		}

		fp := Fingerprint(rule.ID, agentName)
		live, err := ev.s.AlertEvents.GetLive(ctx, rule.ID, fp)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			ev.log.Warnw("alert event live lookup failed", "rule", rule.ID, "err", err)
			continue
		}
		var evt *Event
		if live != nil {
			evt = &Event{State: live.State, LastFiredAt: live.LastFiredAt}
		}

		d := NextEvent(Rule{CooldownSeconds: rule.CooldownSeconds}, evt, Eval{Breached: true, Now: now})
		if d.Notify != "fire" {
			continue // within cooldown: suppressed
		}
		// A per-rule persist error must not skip the remaining rules. The
		// authoritative cooldown/first-fire guard is inside persistEvent's atomic
		// FireEvent, so a concurrent finish that loses the race enqueues nothing
		// rather than erroring here.
		if err := ev.persistEvent(ctx, rule, fp, agentName, now); err != nil {
			ev.log.Warnw("alert event persist failed", "rule", rule.ID, "err", err)
			continue
		}
	}
	return nil
}

// eventBreached maps an event rule's signal to the just-finished run's outcome.
func eventBreached(signal string, loopDetected bool, status string) bool {
	switch signal {
	case "loop":
		return loopDetected
	case "run_failure":
		return status == "failed"
	default:
		return false
	}
}

// persistEvent atomically fires the event + enqueues its notification in one
// transaction. FireEvent is the authoritative guard: its cooldown-checked upsert
// on the partial unique index means only one of two racing finishes commits a fire
// (the GetLive+NextEvent fast-path in the caller only avoids the tx in the common
// suppressed case). If this call doesn't win the fire, it enqueues nothing.
func (ev *Evaluator) persistEvent(ctx context.Context, rule *store.AlertRule, fp []byte, scope string, now time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, store.QueryTimeoutDuration)
	defer cancel()
	return withPoolTx(ev.pool, ctx, func(tx pgx.Tx) error {
		e := &store.AlertEvent{RuleID: rule.ID, ProjectID: rule.ProjectID, Fingerprint: fp, ScopeLabel: scope}
		id, fired, err := ev.s.AlertEvents.FireEvent(ctx, tx, e, rule.CooldownSeconds)
		if err != nil {
			return err
		}
		if !fired {
			return nil
		}
		payload := buildPayload(rule, scope, 0, "fire", now)
		return ev.s.NotificationJobs.Enqueue(ctx, tx, &store.NotificationJob{
			AlertEventID: id, Channel: rule.Channel, Payload: payload, RunAfter: now,
		})
	})
}
