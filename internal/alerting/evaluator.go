package alerting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/keelwave/keelwave/internal/store"
)

// Evaluator runs aggregate alert rules: it queries the rule's metric over its
// window/scope, advances the condition state machine, and persists any transition
// + enqueues delivery in a single transaction (transactional outbox).
type Evaluator struct {
	pool *pgxpool.Pool
	s    store.Storage
	log  *zap.SugaredLogger
}

func NewEvaluator(pool *pgxpool.Pool, s store.Storage, log *zap.SugaredLogger) *Evaluator {
	return &Evaluator{pool: pool, s: s, log: log}
}

// Fingerprint identifies a (rule, scope) alert stream. The NUL separator keeps
// ruleID and scopeLabel from colliding across boundaries.
func Fingerprint(ruleID uuid.UUID, scopeLabel string) []byte {
	h := sha256.New()
	h.Write([]byte(ruleID.String()))
	h.Write([]byte{0})
	h.Write([]byte(scopeLabel))
	return h.Sum(nil)
}

// EvaluateAggregate queries the rule's metric over its window, runs the state
// machine, and persists any transition + enqueues delivery in one transaction.
func (ev *Evaluator) EvaluateAggregate(ctx context.Context, rule *store.AlertRule) error {
	value, scope, count, err := ev.queryMetric(ctx, rule)
	if err != nil {
		return err
	}
	if rule.MinRequests > 0 && count < rule.MinRequests {
		return nil // not enough volume; skip (anti-noise)
	}
	breached := compare(value, rule.Comparator, rule.Threshold)
	fp := Fingerprint(rule.ID, scope)
	now := time.Now()

	live, err := ev.s.AlertEvents.GetLive(ctx, rule.ID, fp)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	var evt *Event
	if live != nil {
		evt = &Event{State: live.State, FirstBreachedAt: live.FirstBreachedAt,
			FiredAt: live.FiredAt, RecoveringSince: live.RecoveringSince}
	}

	smRule := Rule{ForSeconds: rule.ForSeconds, KeepFiringForSeconds: rule.KeepFiringForSeconds}
	d := settle(smRule, evt, Eval{Breached: breached, Value: value, Now: now})

	// Nothing live and nothing to do: don't write a no-op row.
	if live == nil && d.Notify == "" && (d.NextState == "resolved" || d.NextState == "inactive") {
		return nil
	}
	return ev.persist(ctx, rule, live, fp, scope, d, value, now)
}

// settle advances the aggregate state machine within a single evaluation tick.
// The pure machine deliberately stamps first_breached_at on the inactive->pending
// edge before it can measure the `for` duration, so a zero-duration rule
// (for_seconds=0) needs two applications to reach firing. A scheduled tick must
// fully resolve such zero-duration transitions in one pass, so we iterate —
// projecting the event forward exactly as persist would — until the state stops
// changing, carrying the strongest fire/resolve notify seen along the way.
func settle(rule Rule, evt *Event, e Eval) Decision {
	notify := ""
	var d Decision
	for i := 0; i < 8; i++ {
		d = NextAggregate(rule, evt, e)
		if d.Notify != "" {
			notify = d.Notify
		}
		cur := "inactive"
		if evt != nil {
			cur = evt.State
		}
		if d.NextState == cur {
			break // settled: no further transition this tick
		}
		evt = projectEvent(evt, d, e.Now)
	}
	return Decision{NextState: d.NextState, Notify: notify}
}

// projectEvent mirrors persist's field stamping so the next settle iteration sees
// the event as it would be written: first_breached_at on entry to pending,
// fired_at on fire, recovering_since on entry to recovering.
func projectEvent(prev *Event, d Decision, now time.Time) *Event {
	next := &Event{State: d.NextState}
	prevState := "inactive"
	if prev != nil {
		prevState = prev.State
		next.FirstBreachedAt = prev.FirstBreachedAt
		next.FiredAt = prev.FiredAt
		next.RecoveringSince = prev.RecoveringSince
	}
	if d.NextState == "pending" && next.FirstBreachedAt == nil {
		next.FirstBreachedAt = &now
	}
	if d.Notify == "fire" {
		next.FiredAt = &now
	}
	if d.NextState == "recovering" && prevState != "recovering" {
		next.RecoveringSince = &now
	}
	return next
}

func (ev *Evaluator) persist(ctx context.Context, rule *store.AlertRule, live *store.AlertEvent, fp []byte, scope string, d Decision, value float64, now time.Time) error {
	return withPoolTx(ev.pool, ctx, func(tx pgx.Tx) error {
		e := live
		if e == nil {
			e = &store.AlertEvent{RuleID: rule.ID, ProjectID: rule.ProjectID, Fingerprint: fp, ScopeLabel: scope}
			e.FirstBreachedAt = &now
		}
		e.State = d.NextState
		e.LastValue = &value
		e.LastEvaluatedAt = now
		if d.Notify == "fire" {
			e.FiredAt = &now
		}
		// Stamp recovery start on entry to recovering — the state machine times
		// keep_firing_for from this, so set it exactly once on firing->recovering
		// (not re-stamped on later recovering ticks).
		if d.NextState == "recovering" && (live == nil || live.State != "recovering") {
			e.RecoveringSince = &now
		}
		if d.NextState == "resolved" {
			e.ResolvedAt = &now
		}
		if err := ev.s.AlertEvents.Upsert(ctx, tx, e); err != nil {
			return err
		}
		if d.Notify != "" {
			payload := buildPayload(rule, scope, value, d.Notify, now)
			if err := ev.s.NotificationJobs.Enqueue(ctx, tx, &store.NotificationJob{
				AlertEventID: e.ID, Channel: rule.Channel, Payload: payload, RunAfter: now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// queryMetric resolves the rule's observed value + scope over its window. Returns
// the observed value, its scope_label, and the denominator count in the window
// (for the min_requests noise guard).
//
// Scope follows the spec §4.2 two-lifecycle split, keeping the fingerprint stable:
// a scoped rule (AgentName != nil) filters to that agent and labels the stream
// with the agent name; an unscoped rule (AgentName == nil) aggregates the whole
// project (all agents together) under the empty scope. A shared agent-name
// predicate handles both cases in one query: an empty agentFilter matches every
// agent, a set one restricts to it. Each query is a pure aggregate (no GROUP BY),
// so it always returns exactly one row.
//
// Signal sources: cost/completion/termination read the agent_runs_5m continuous
// aggregate (real-time, so the un-materialized tail counts); duration_p95 is a
// query-time ordered-set aggregate on raw agent_runs (caggs can't materialize
// percentiles); tool_failure reads agent_steps and eval_regression reads
// agent_evaluations, each joined to agent_runs for agent scoping.
func (ev *Evaluator) queryMetric(ctx context.Context, rule *store.AlertRule) (float64, string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, store.QueryTimeoutDuration)
	defer cancel()

	window := 300
	if rule.WindowSeconds != nil {
		window = *rule.WindowSeconds
	}
	scope := ""
	if rule.AgentName != nil {
		scope = *rule.AgentName
	}
	agentFilter := scope // "" => project-wide (matches every agent)

	var q string
	switch rule.Signal {
	case "cost_burn":
		q = `
			SELECT coalesce(sum(cost_usd), 0)::double precision,
			       coalesce(sum(total_runs), 0)::int
			FROM agent_runs_5m
			WHERE project_id = $1
			  AND ($2 = '' OR agent_name = $2)
			  AND bucket >= now() - ($3 * interval '1 second')`
	case "run_failure":
		// Completion rate over the window. Coalesce to 1 (fully healthy) when the
		// window is empty so a `<` rule doesn't fire on no data.
		q = `
			SELECT coalesce(sum(completed_runs)::float8 / nullif(sum(total_runs), 0), 1),
			       coalesce(sum(total_runs), 0)::int
			FROM agent_runs_5m
			WHERE project_id = $1
			  AND ($2 = '' OR agent_name = $2)
			  AND bucket >= now() - ($3 * interval '1 second')`
	case "termination_shift":
		// Bad-termination share (error/timeout/max_steps_reached over total).
		q = `
			SELECT coalesce(sum(bad_termination_runs)::float8 / nullif(sum(total_runs), 0), 0),
			       coalesce(sum(total_runs), 0)::int
			FROM agent_runs_5m
			WHERE project_id = $1
			  AND ($2 = '' OR agent_name = $2)
			  AND bucket >= now() - ($3 * interval '1 second')`
	case "duration_p95":
		// Query-time percentile on the raw hypertable — TimescaleDB caggs can't
		// materialize ordered-set aggregates, so this is by design.
		q = `
			SELECT coalesce(percentile_disc(0.95) WITHIN GROUP (ORDER BY duration_ms), 0)::float8,
			       count(*)::int
			FROM agent_runs
			WHERE project_id = $1
			  AND ($2 = '' OR agent_name = $2)
			  AND timestamp >= now() - ($3 * interval '1 second')`
	case "tool_failure":
		// Tool failure rate from agent_steps (joined to agent_runs for agent
		// scoping, since steps carry no agent_name). The denominator — steps that
		// reported a tool_success — is the count for the min_requests guard.
		q = `
			SELECT coalesce(
			         count(*) FILTER (WHERE s.tool_success IS FALSE)::float8
			         / nullif(count(*) FILTER (WHERE s.tool_success IS NOT NULL), 0), 0),
			       count(*) FILTER (WHERE s.tool_success IS NOT NULL)::int
			FROM agent_steps s
			JOIN agent_runs r ON r.id = s.agent_run_id AND r.project_id = s.project_id
			WHERE s.project_id = $1
			  AND ($2 = '' OR r.agent_name = $2)
			  AND s.timestamp >= now() - ($3 * interval '1 second')`
	case "eval_regression":
		// Average correctness over the window (joined to agent_runs for agent
		// scoping). Coalesce to 1 on empty so a `<` rule doesn't fire on no data.
		q = `
			SELECT coalesce(avg(e.correctness)::float8, 1),
			       count(e.correctness)::int
			FROM agent_evaluations e
			JOIN agent_runs r ON r.id = e.agent_run_id AND r.project_id = e.project_id
			WHERE e.project_id = $1
			  AND ($2 = '' OR r.agent_name = $2)
			  AND e.evaluated_at >= now() - ($3 * interval '1 second')`
	default:
		return 0, "", 0, fmt.Errorf("alerting: signal %q not implemented", rule.Signal)
	}

	var (
		value float64
		count int
	)
	err := ev.pool.QueryRow(ctx, q, rule.ProjectID, agentFilter, window).Scan(&value, &count)
	if err == nil {
		return value, scope, count, nil
	}
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return 0, scope, 0, nil // no data in window: not breached
	default:
		return 0, "", 0, err
	}
}

func compare(value float64, comparator string, threshold float64) bool {
	switch comparator {
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	default:
		return false
	}
}

// buildPayload assembles the channel-agnostic alert body stored on the
// notification_jobs row. Keys match the mailer template; the rule's
// channel_config is merged in so the channel sender finds its target (email's
// "to", webhook's "url") in the payload.
func buildPayload(rule *store.AlertRule, scope string, value float64, kind string, now time.Time) []byte {
	m := map[string]any{
		"rule_name":   rule.Name,
		"signal":      rule.Signal,
		"severity":    rule.Severity,
		"project_id":  rule.ProjectID.String(),
		"scope":       scope,
		"transition":  kind, // "fire" | "resolve"
		"value":       value,
		"comparator":  rule.Comparator,
		"threshold":   rule.Threshold,
		"fired_at":    now.Format(time.RFC3339),
		"fingerprint": hex.EncodeToString(Fingerprint(rule.ID, scope)),
	}
	if rule.WindowSeconds != nil {
		m["window"] = *rule.WindowSeconds
	}
	// Merge channel_config (carries the recipient: "to" for email, "url" for
	// webhook) so the channel sender finds its target in the payload. Alert
	// fields win on key clash.
	if len(rule.ChannelConfig) > 0 {
		var cc map[string]any
		if err := json.Unmarshal(rule.ChannelConfig, &cc); err == nil {
			for k, v := range cc {
				if _, exists := m[k]; !exists {
					m[k] = v
				}
			}
		}
	}
	b, _ := json.Marshal(m)
	return b
}

// withPoolTx runs fn inside a transaction, mirroring store.withTx but local to the
// alerting package so the state write + job enqueue commit atomically.
func withPoolTx(pool *pgxpool.Pool, ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}
