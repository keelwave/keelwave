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
// the breaching scope's value, its scope_label (agent_name), and the run count in
// the window (for the min_requests noise guard).
//
// A2 implements cost_burn only; the remaining signals (run_failure,
// termination_shift, tool_failure, duration_p95, eval_regression) land in Task 12.
func (ev *Evaluator) queryMetric(ctx context.Context, rule *store.AlertRule) (float64, string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, store.QueryTimeoutDuration)
	defer cancel()

	window := 300
	if rule.WindowSeconds != nil {
		window = *rule.WindowSeconds
	}
	agentFilter := ""
	if rule.AgentName != nil {
		agentFilter = *rule.AgentName
	}

	switch rule.Signal {
	case "cost_burn":
		// Sum cost over the window per agent, pick the highest-cost scope. The
		// continuous aggregate is real-time (materialized_only=false), so runs in
		// the un-materialized tail — including one at now() — are counted.
		// Per-scope fan-out for multi-agent rules is a scheduler concern (Task 12);
		// here we evaluate the single scope: the named agent, or the top agent when
		// the rule is unscoped.
		const q = `
			SELECT agent_name,
			       coalesce(sum(cost_usd), 0)::double precision AS value,
			       coalesce(sum(total_runs), 0)::int            AS cnt
			FROM agent_runs_5m
			WHERE project_id = $1
			  AND ($2 = '' OR agent_name = $2)
			  AND bucket >= now() - ($3 * interval '1 second')
			GROUP BY agent_name
			ORDER BY value DESC
			LIMIT 1`
		var (
			scope string
			value float64
			count int
		)
		err := ev.pool.QueryRow(ctx, q, rule.ProjectID, agentFilter, window).Scan(&scope, &value, &count)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, agentFilter, 0, nil // no runs in window: not breached
		}
		if err != nil {
			return 0, "", 0, err
		}
		return value, scope, count, nil
	default:
		return 0, "", 0, fmt.Errorf("alerting: signal %q not implemented", rule.Signal)
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

// alertPayload is the channel-agnostic alert body stored on the notification_jobs
// row (spec §5); channel senders render it into their wire shape.
type alertPayload struct {
	Rule        string    `json:"rule"`
	Signal      string    `json:"signal"`
	Severity    string    `json:"severity"`
	ProjectID   string    `json:"project_id"`
	Scope       string    `json:"scope"`
	Kind        string    `json:"kind"` // "fire" | "resolve"
	Value       float64   `json:"value"`
	Comparator  string    `json:"comparator"`
	Threshold   float64   `json:"threshold"`
	Window      int       `json:"window_seconds,omitempty"`
	FiredAt     time.Time `json:"fired_at"`
	Fingerprint string    `json:"fingerprint"`
}

func buildPayload(rule *store.AlertRule, scope string, value float64, kind string, now time.Time) []byte {
	p := alertPayload{
		Rule:        rule.Name,
		Signal:      rule.Signal,
		Severity:    rule.Severity,
		ProjectID:   rule.ProjectID.String(),
		Scope:       scope,
		Kind:        kind,
		Value:       value,
		Comparator:  rule.Comparator,
		Threshold:   rule.Threshold,
		FiredAt:     now,
		Fingerprint: hex.EncodeToString(Fingerprint(rule.ID, scope)),
	}
	if rule.WindowSeconds != nil {
		p.Window = *rule.WindowSeconds
	}
	b, _ := json.Marshal(p) // fixed shape of primitives: marshal cannot fail
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
