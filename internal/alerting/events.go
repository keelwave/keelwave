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
// just-finished run. For each rule whose signal matches the run's outcome
// (loop -> loopDetected, run_failure -> status=="failed") and whose optional
// agent_name filter matches, it advances the point-in-time state machine
// (cooldown dedup via the live event's last_fired_at) and, on a fire decision,
// persists a fired event + enqueues its notification in one transaction
// (transactional outbox). Called off the ingest hot path.
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
			return err
		}
		var evt *Event
		if live != nil {
			evt = &Event{State: live.State, LastFiredAt: live.LastFiredAt}
		}

		d := NextEvent(Rule{CooldownSeconds: rule.CooldownSeconds}, evt, Eval{Breached: true, Now: now})
		if d.Notify != "fire" {
			continue // within cooldown: suppressed
		}
		if err := ev.persistEvent(ctx, rule, live, fp, agentName, now); err != nil {
			return err
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

// persistEvent writes the fired event + enqueues its notification in one
// transaction. Unlike persist (aggregate), event rules are point-in-time: the
// row is always "fired", last_fired_at is stamped to now for cooldown dedup, and
// a notification is always enqueued — the caller only reaches here on a fire
// decision.
func (ev *Evaluator) persistEvent(ctx context.Context, rule *store.AlertRule, live *store.AlertEvent, fp []byte, scope string, now time.Time) error {
	return withPoolTx(ev.pool, ctx, func(tx pgx.Tx) error {
		e := live
		if e == nil {
			e = &store.AlertEvent{RuleID: rule.ID, ProjectID: rule.ProjectID, Fingerprint: fp, ScopeLabel: scope}
			e.FirstBreachedAt = &now
		}
		e.State = "fired"
		e.FiredAt = &now
		e.LastFiredAt = &now
		e.LastEvaluatedAt = now
		if err := ev.s.AlertEvents.Upsert(ctx, tx, e); err != nil {
			return err
		}
		payload := buildPayload(rule, scope, 0, "fire", now)
		return ev.s.NotificationJobs.Enqueue(ctx, tx, &store.NotificationJob{
			AlertEventID: e.ID, Channel: rule.Channel, Payload: payload, RunAfter: now,
		})
	})
}
