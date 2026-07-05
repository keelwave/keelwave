package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AlertRule struct {
	ID                   uuid.UUID `json:"id"`
	ProjectID            uuid.UUID `json:"project_id"`
	AgentName            *string   `json:"agent_name,omitempty"`
	Name                 string    `json:"name"`
	Class                string    `json:"class"`
	Signal               string    `json:"signal"`
	Comparator           string    `json:"comparator"`
	Threshold            float64   `json:"threshold"`
	WindowSeconds        *int      `json:"window_seconds,omitempty"`
	Severity             string    `json:"severity"`
	ForSeconds           int       `json:"for_seconds"`
	KeepFiringForSeconds int       `json:"keep_firing_for_seconds"`
	CooldownSeconds      int       `json:"cooldown_seconds"`
	MinRequests          int       `json:"min_requests"`
	Channel              string    `json:"channel"`
	ChannelConfig        []byte    `json:"channel_config"`
	Enabled              bool      `json:"enabled"`
	CreatedAt            time.Time `json:"created_at"`
}

type AlertRuleStore struct {
	pool *pgxpool.Pool
}

const alertRuleCols = `id, project_id, agent_name, name, class, signal, comparator,
	threshold, window_seconds, severity, for_seconds, keep_firing_for_seconds,
	cooldown_seconds, min_requests, channel, channel_config, enabled, created_at`

func scanAlertRule(row pgx.Row) (*AlertRule, error) {
	r := &AlertRule{}
	err := row.Scan(&r.ID, &r.ProjectID, &r.AgentName, &r.Name, &r.Class, &r.Signal,
		&r.Comparator, &r.Threshold, &r.WindowSeconds, &r.Severity, &r.ForSeconds,
		&r.KeepFiringForSeconds, &r.CooldownSeconds, &r.MinRequests, &r.Channel,
		&r.ChannelConfig, &r.Enabled, &r.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return r, nil
}

func (s *AlertRuleStore) Create(ctx context.Context, r *AlertRule) error {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()
	const q = `
		INSERT INTO alert_rules (project_id, agent_name, name, class, signal,
			comparator, threshold, window_seconds, severity, for_seconds,
			keep_firing_for_seconds, cooldown_seconds, min_requests, channel,
			channel_config, enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		RETURNING id, created_at`
	return s.pool.QueryRow(ctx, q, r.ProjectID, r.AgentName, r.Name, r.Class, r.Signal,
		r.Comparator, r.Threshold, r.WindowSeconds, r.Severity, r.ForSeconds,
		r.KeepFiringForSeconds, r.CooldownSeconds, r.MinRequests, r.Channel,
		r.ChannelConfig, r.Enabled).Scan(&r.ID, &r.CreatedAt)
}

func (s *AlertRuleStore) GetByID(ctx context.Context, id, projectID uuid.UUID) (*AlertRule, error) {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()
	q := `SELECT ` + alertRuleCols + ` FROM alert_rules WHERE id = $1 AND project_id = $2`
	return scanAlertRule(s.pool.QueryRow(ctx, q, id, projectID))
}

func (s *AlertRuleStore) ListByProject(ctx context.Context, projectID uuid.UUID) ([]*AlertRule, error) {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()
	q := `SELECT ` + alertRuleCols + ` FROM alert_rules WHERE project_id = $1 ORDER BY created_at DESC`
	return s.queryRules(ctx, q, projectID)
}

func (s *AlertRuleStore) ListEnabled(ctx context.Context) ([]*AlertRule, error) {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()
	q := `SELECT ` + alertRuleCols + ` FROM alert_rules WHERE enabled ORDER BY project_id`
	return s.queryRules(ctx, q)
}

func (s *AlertRuleStore) queryRules(ctx context.Context, q string, args ...any) ([]*AlertRule, error) {
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*AlertRule, 0)
	for rows.Next() {
		r, err := scanAlertRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *AlertRuleStore) Update(ctx context.Context, r *AlertRule) error {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()
	const q = `
		UPDATE alert_rules SET agent_name=$3, name=$4, comparator=$5, threshold=$6,
			window_seconds=$7, severity=$8, for_seconds=$9, keep_firing_for_seconds=$10,
			cooldown_seconds=$11, min_requests=$12, channel=$13, channel_config=$14, enabled=$15
		WHERE id=$1 AND project_id=$2`
	tag, err := s.pool.Exec(ctx, q, r.ID, r.ProjectID, r.AgentName, r.Name, r.Comparator,
		r.Threshold, r.WindowSeconds, r.Severity, r.ForSeconds, r.KeepFiringForSeconds,
		r.CooldownSeconds, r.MinRequests, r.Channel, r.ChannelConfig, r.Enabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *AlertRuleStore) Delete(ctx context.Context, id, projectID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()
	tag, err := s.pool.Exec(ctx, `DELETE FROM alert_rules WHERE id=$1 AND project_id=$2`, id, projectID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
