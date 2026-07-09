package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AlertEvent struct {
	ID              uuid.UUID  `json:"id"`
	RuleID          uuid.UUID  `json:"rule_id"`
	ProjectID       uuid.UUID  `json:"project_id"`
	Fingerprint     []byte     `json:"-"`
	ScopeLabel      string     `json:"scope_label"`
	State           string     `json:"state"`
	FirstBreachedAt *time.Time `json:"first_breached_at,omitempty"`
	FiredAt         *time.Time `json:"fired_at,omitempty"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	RecoveringSince *time.Time `json:"recovering_since,omitempty"`
	LastFiredAt     *time.Time `json:"last_fired_at,omitempty"`
	LastValue       *float64   `json:"last_value,omitempty"`
	LastEvaluatedAt time.Time  `json:"last_evaluated_at"`
}

type AlertEventStore struct {
	pool *pgxpool.Pool
}

const alertEventCols = `id, rule_id, project_id, fingerprint, scope_label, state,
	first_breached_at, fired_at, resolved_at, recovering_since, last_fired_at, last_value, last_evaluated_at`

func scanAlertEvent(row pgx.Row) (*AlertEvent, error) {
	e := &AlertEvent{}
	err := row.Scan(&e.ID, &e.RuleID, &e.ProjectID, &e.Fingerprint, &e.ScopeLabel,
		&e.State, &e.FirstBreachedAt, &e.FiredAt, &e.ResolvedAt, &e.RecoveringSince,
		&e.LastFiredAt, &e.LastValue, &e.LastEvaluatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return e, nil
}

// GetLive returns the current non-resolved event for a rule+fingerprint.
func (s *AlertEventStore) GetLive(ctx context.Context, ruleID uuid.UUID, fingerprint []byte) (*AlertEvent, error) {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()
	q := `SELECT ` + alertEventCols + ` FROM alert_events
		WHERE rule_id = $1 AND fingerprint = $2 AND state <> 'resolved' LIMIT 1`
	return scanAlertEvent(s.pool.QueryRow(ctx, q, ruleID, fingerprint))
}

// Upsert inserts a new event or updates the existing live row (matched by id when
// set). Participates in the caller's tx so the state write + job enqueue commit
// atomically (transactional outbox).
func (s *AlertEventStore) Upsert(ctx context.Context, tx pgx.Tx, e *AlertEvent) error {
	if e.ID != uuid.Nil {
		const q = `
			UPDATE alert_events SET state=$2, first_breached_at=$3, fired_at=$4,
				resolved_at=$5, recovering_since=$6, last_fired_at=$7, last_value=$8,
				last_evaluated_at=$9
			WHERE id=$1`
		_, err := tx.Exec(ctx, q, e.ID, e.State, e.FirstBreachedAt, e.FiredAt,
			e.ResolvedAt, e.RecoveringSince, e.LastFiredAt, e.LastValue, e.LastEvaluatedAt)
		return err
	}
	const q = `
		INSERT INTO alert_events (rule_id, project_id, fingerprint, scope_label, state,
			first_breached_at, fired_at, resolved_at, recovering_since, last_fired_at, last_value, last_evaluated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id`
	return tx.QueryRow(ctx, q, e.RuleID, e.ProjectID, e.Fingerprint, e.ScopeLabel, e.State,
		e.FirstBreachedAt, e.FiredAt, e.ResolvedAt, e.RecoveringSince, e.LastFiredAt,
		e.LastValue, e.LastEvaluatedAt).Scan(&e.ID)
}

// FireEvent atomically upserts a fired event guarded by cooldownSeconds, using the
// partial unique index alert_events_live_idx (rule_id, fingerprint WHERE state <>
// 'resolved') as the conflict arbiter. Exactly one concurrent caller wins the
// insert-or-cooldown race and returns (id, true); a caller whose conflict finds the
// cooldown unelapsed matches no row and returns (uuid.Nil, false). Runs in the
// caller's tx so the fire + its notification enqueue commit atomically.
func (s *AlertEventStore) FireEvent(ctx context.Context, tx pgx.Tx, e *AlertEvent, cooldownSeconds int) (uuid.UUID, bool, error) {
	const q = `
		INSERT INTO alert_events (rule_id, project_id, fingerprint, scope_label, state,
			first_breached_at, fired_at, last_fired_at, last_evaluated_at)
		VALUES ($1,$2,$3,$4,'fired', now(), now(), now(), now())
		ON CONFLICT (rule_id, fingerprint) WHERE state <> 'resolved'
		DO UPDATE SET fired_at = now(), last_fired_at = now(), last_evaluated_at = now()
			WHERE alert_events.last_fired_at IS NULL
			   OR alert_events.last_fired_at <= now() - make_interval(secs => $5)
		RETURNING id`
	var id uuid.UUID
	err := tx.QueryRow(ctx, q, e.RuleID, e.ProjectID, e.Fingerprint, e.ScopeLabel, cooldownSeconds).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return id, true, nil
}

func (s *AlertEventStore) ListByProject(ctx context.Context, projectID uuid.UUID, limit int) ([]*AlertEvent, error) {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()
	q := `SELECT ` + alertEventCols + ` FROM alert_events WHERE project_id=$1 ORDER BY id DESC LIMIT $2`
	rows, err := s.pool.Query(ctx, q, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*AlertEvent, 0)
	for rows.Next() {
		e, err := scanAlertEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
