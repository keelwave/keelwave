package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AgentRun struct {
	ID                uuid.UUID  `json:"id"`
	Timestamp         time.Time  `json:"timestamp"`
	ProjectID         uuid.UUID  `json:"project_id"`
	AgentName         string     `json:"agent_name"`
	Status            string     `json:"status"`
	TerminationReason *string    `json:"termination_reason,omitempty"`
	LoopDetected      bool       `json:"loop_detected"`
	LoopStepIndex     *int       `json:"loop_step_index,omitempty"`
	TotalSteps        int        `json:"total_steps"`
	TotalTokens       int        `json:"total_tokens"`
	TotalCostUSD      *float64   `json:"total_cost_usd,omitempty"`
	DurationMs        *int       `json:"duration_ms,omitempty"`
	Input             *string    `json:"input,omitempty"`
	Output            *string    `json:"output,omitempty"`
	Metadata          []byte     `json:"metadata,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
}

type AgentRunFinish struct {
	Status            string
	TerminationReason *string
	LoopDetected      bool
	LoopStepIndex     *int
	TotalSteps        int
	TotalTokens       int
	TotalCostUSD      *float64
	DurationMs        *int
	Output            *string
}

type AgentRunStore struct {
	pool *pgxpool.Pool
}

func (s *AgentRunStore) Insert(ctx context.Context, r *AgentRun) error {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	if r.Timestamp.IsZero() {
		r.Timestamp = time.Now()
	}

	const q = `
		INSERT INTO agent_runs (
			timestamp, project_id, agent_name, status,
			input, metadata
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`
	return s.pool.QueryRow(ctx, q,
		r.Timestamp, r.ProjectID, r.AgentName, r.Status,
		r.Input, r.Metadata,
	).Scan(&r.ID)
}

func (s *AgentRunStore) ListByProject(ctx context.Context, projectID uuid.UUID, from, to time.Time, limit, offset int) ([]*AgentRun, error) {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	// Prefer cost summed from the run's linked ai_traces (auto-emitted + priced
	// server-side per LLM call); fall back to the stored rollup for users who
	// report cost manually without our provider adapters. See
	// docs/notes/cost-pricing.md.
	const q = `
		SELECT id, timestamp, project_id, agent_name, status,
		       termination_reason, loop_detected, loop_step_index,
		       total_steps,
		       coalesce(
		         (SELECT sum(coalesce(t.total_tokens, t.input_tokens + t.output_tokens)) FROM ai_traces t
		          WHERE t.agent_run_id = agent_runs.id AND t.project_id = agent_runs.project_id),
		         total_tokens) AS total_tokens,
		       coalesce(
		         (SELECT sum(t.cost_usd) FROM ai_traces t
		          WHERE t.agent_run_id = agent_runs.id AND t.project_id = agent_runs.project_id),
		         total_cost_usd) AS total_cost_usd,
		       duration_ms,
		       input, output, metadata, finished_at
		FROM agent_runs
		WHERE project_id = $1 AND timestamp >= $2 AND timestamp < $3
		ORDER BY timestamp DESC
		LIMIT $4 OFFSET $5
	`
	rows, err := s.pool.Query(ctx, q, projectID, from, to, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*AgentRun, 0)
	for rows.Next() {
		r := &AgentRun{}
		if err := rows.Scan(
			&r.ID, &r.Timestamp, &r.ProjectID, &r.AgentName, &r.Status,
			&r.TerminationReason, &r.LoopDetected, &r.LoopStepIndex,
			&r.TotalSteps, &r.TotalTokens, &r.TotalCostUSD, &r.DurationMs,
			&r.Input, &r.Output, &r.Metadata, &r.FinishedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *AgentRunStore) GetByID(ctx context.Context, projectID, runID uuid.UUID, from, to time.Time) (*AgentRun, error) {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	// total_cost_usd: linked-trace sum, falling back to the stored rollup — see ListByProject.
	const q = `
		SELECT id, timestamp, project_id, agent_name, status,
		       termination_reason, loop_detected, loop_step_index,
		       total_steps,
		       coalesce(
		         (SELECT sum(coalesce(t.total_tokens, t.input_tokens + t.output_tokens)) FROM ai_traces t
		          WHERE t.agent_run_id = agent_runs.id AND t.project_id = agent_runs.project_id),
		         total_tokens) AS total_tokens,
		       coalesce(
		         (SELECT sum(t.cost_usd) FROM ai_traces t
		          WHERE t.agent_run_id = agent_runs.id AND t.project_id = agent_runs.project_id),
		         total_cost_usd) AS total_cost_usd,
		       duration_ms,
		       input, output, metadata, finished_at
		FROM agent_runs
		WHERE project_id = $1 AND id = $2
		  AND timestamp >= $3 AND timestamp < $4
		LIMIT 1
	`
	r := &AgentRun{}
	err := s.pool.QueryRow(ctx, q, projectID, runID, from, to).Scan(
		&r.ID, &r.Timestamp, &r.ProjectID, &r.AgentName, &r.Status,
		&r.TerminationReason, &r.LoopDetected, &r.LoopStepIndex,
		&r.TotalSteps, &r.TotalTokens, &r.TotalCostUSD, &r.DurationMs,
		&r.Input, &r.Output, &r.Metadata, &r.FinishedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return r, nil
}

// Finish marks a run terminal. Caller passes id + timestamp because hypertable
// PK is (id, timestamp); without timestamp the UPDATE has to scan every chunk.
func (s *AgentRunStore) Finish(ctx context.Context, id uuid.UUID, ts time.Time, f AgentRunFinish) error {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	const q = `
		UPDATE agent_runs SET
			status              = $3,
			termination_reason  = $4,
			loop_detected       = $5,
			loop_step_index     = $6,
			total_steps         = $7,
			total_tokens        = $8,
			total_cost_usd      = $9,
			duration_ms         = $10,
			output              = $11,
			finished_at         = now()
		WHERE id = $1 AND timestamp = $2
	`
	tag, err := s.pool.Exec(ctx, q,
		id, ts, f.Status, f.TerminationReason, f.LoopDetected, f.LoopStepIndex,
		f.TotalSteps, f.TotalTokens, f.TotalCostUSD, f.DurationMs, f.Output,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type RunHealthRow struct {
	AgentName      string   `json:"agent_name"`
	TotalRuns      int      `json:"total_runs"`
	CompletedRuns  int      `json:"completed_runs"`
	LoopRuns       int      `json:"loop_runs"`
	CompletionRate float64  `json:"completion_rate"`
	LoopRate       float64  `json:"loop_rate"`
	AvgCostUSD     *float64 `json:"avg_cost_usd,omitempty"`
	AvgTokens      float64  `json:"avg_tokens"`
	PrevTotalRuns  int      `json:"prev_total_runs"`
}

// RunHealth rolls up run outcomes per agent_name across the project, scoped to a
// time window. prev_total_runs carries each agent's run count over the equal
// window immediately before [from, to) so callers can render deltas. Agents that
// ran only in the prior window are excluded. avg_cost_usd is null when no run in
// the current group reported a cost.
func (s *AgentRunStore) RunHealth(ctx context.Context, projectID uuid.UUID, from, to time.Time) ([]*RunHealthRow, error) {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	prevFrom := from.Add(-to.Sub(from))

	// One scan over [prevFrom, to); FILTER splits current vs prior window.
	const q = `
		SELECT agent_name,
		       count(*) FILTER (WHERE timestamp >= $2)::int                              AS total_runs,
		       count(*) FILTER (WHERE timestamp >= $2 AND status = 'completed')::int     AS completed_runs,
		       count(*) FILTER (WHERE timestamp >= $2 AND loop_detected)::int            AS loop_runs,
		       avg(coalesce(
		             (SELECT sum(t.cost_usd) FROM ai_traces t
		              WHERE t.agent_run_id = agent_runs.id AND t.project_id = agent_runs.project_id),
		             total_cost_usd))
		           FILTER (WHERE timestamp >= $2)::float8                                AS avg_cost_usd,
		       coalesce(avg(coalesce((SELECT sum(coalesce(t.total_tokens, t.input_tokens + t.output_tokens)) FROM ai_traces t WHERE t.agent_run_id = agent_runs.id AND t.project_id = agent_runs.project_id), total_tokens)) FILTER (WHERE timestamp >= $2), 0)::float8     AS avg_tokens,
		       count(*) FILTER (WHERE timestamp < $2)::int                               AS prev_total_runs
		FROM agent_runs
		WHERE project_id = $1 AND timestamp >= $4 AND timestamp < $3
		GROUP BY agent_name
		HAVING count(*) FILTER (WHERE timestamp >= $2) > 0
		ORDER BY total_runs DESC
	`
	rows, err := s.pool.Query(ctx, q, projectID, from, to, prevFrom)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*RunHealthRow, 0)
	for rows.Next() {
		r := &RunHealthRow{}
		if err := rows.Scan(
			&r.AgentName, &r.TotalRuns, &r.CompletedRuns, &r.LoopRuns,
			&r.AvgCostUSD, &r.AvgTokens, &r.PrevTotalRuns,
		); err != nil {
			return nil, err
		}
		r.CompletionRate = float64(r.CompletedRuns) / float64(r.TotalRuns)
		r.LoopRate = float64(r.LoopRuns) / float64(r.TotalRuns)
		out = append(out, r)
	}
	return out, rows.Err()
}

type RunSummary struct {
	TotalRuns      int      `json:"total_runs"`
	CompletedRuns  int      `json:"completed_runs"`
	LoopRuns       int      `json:"loop_runs"`
	CompletionRate float64  `json:"completion_rate"`
	LoopRate       float64  `json:"loop_rate"`
	AvgCostUSD     *float64 `json:"avg_cost_usd,omitempty"`
	AvgTokens      float64  `json:"avg_tokens"`
	TotalSteps     int      `json:"total_steps"`
	DurationP50Ms  int      `json:"duration_p50_ms"`
	DurationP95Ms  int      `json:"duration_p95_ms"`
	DurationP99Ms  int      `json:"duration_p99_ms"`
	TotalToolCalls int      `json:"total_tool_calls"`
	UniqueTools    int      `json:"unique_tools"`
	UniqueAgents   int      `json:"unique_agents"`
}

func (r *RunSummary) computeRates() {
	if r.TotalRuns > 0 {
		r.CompletionRate = float64(r.CompletedRuns) / float64(r.TotalRuns)
		r.LoopRate = float64(r.LoopRuns) / float64(r.TotalRuns)
	}
}

// SummaryWithPrev aggregates run outcomes, duration percentiles, and unique
// agent count for [from, to) and the equal window immediately before it, in a
// single scan over [prevFrom, to). Step-derived fields (tool calls, unique
// tools) live in agent_steps and are filled by the caller. avg_cost_usd is null
// when no run in a window reported a cost.
func (s *AgentRunStore) SummaryWithPrev(ctx context.Context, projectID uuid.UUID, from, to time.Time) (cur, prev *RunSummary, err error) {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	prevFrom := from.Add(-to.Sub(from))

	const q = `
		SELECT
			count(*) FILTER (WHERE timestamp >= $2)::int                                          AS cur_total,
			count(*) FILTER (WHERE timestamp >= $2 AND status = 'completed')::int                 AS cur_completed,
			count(*) FILTER (WHERE timestamp >= $2 AND loop_detected)::int                        AS cur_loops,
			avg(coalesce(
			      (SELECT sum(t.cost_usd) FROM ai_traces t
			       WHERE t.agent_run_id = agent_runs.id AND t.project_id = agent_runs.project_id),
			      total_cost_usd))
			    FILTER (WHERE timestamp >= $2)::float8                                            AS cur_avg_cost,
			coalesce(avg(coalesce((SELECT sum(coalesce(t.total_tokens, t.input_tokens + t.output_tokens)) FROM ai_traces t WHERE t.agent_run_id = agent_runs.id AND t.project_id = agent_runs.project_id), total_tokens)) FILTER (WHERE timestamp >= $2), 0)::float8                 AS cur_avg_tokens,
			coalesce(sum(total_steps) FILTER (WHERE timestamp >= $2), 0)::int                     AS cur_steps,
			coalesce(percentile_disc(0.50) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE timestamp >= $2), 0)::int AS cur_p50,
			coalesce(percentile_disc(0.95) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE timestamp >= $2), 0)::int AS cur_p95,
			coalesce(percentile_disc(0.99) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE timestamp >= $2), 0)::int AS cur_p99,
			count(DISTINCT agent_name) FILTER (WHERE timestamp >= $2)::int                        AS cur_agents,

			count(*) FILTER (WHERE timestamp < $2)::int                                           AS prev_total,
			count(*) FILTER (WHERE timestamp < $2 AND status = 'completed')::int                  AS prev_completed,
			count(*) FILTER (WHERE timestamp < $2 AND loop_detected)::int                         AS prev_loops,
			avg(coalesce(
			      (SELECT sum(t.cost_usd) FROM ai_traces t
			       WHERE t.agent_run_id = agent_runs.id AND t.project_id = agent_runs.project_id),
			      total_cost_usd))
			    FILTER (WHERE timestamp < $2)::float8                                             AS prev_avg_cost,
			coalesce(avg(coalesce((SELECT sum(coalesce(t.total_tokens, t.input_tokens + t.output_tokens)) FROM ai_traces t WHERE t.agent_run_id = agent_runs.id AND t.project_id = agent_runs.project_id), total_tokens)) FILTER (WHERE timestamp < $2), 0)::float8                  AS prev_avg_tokens,
			coalesce(sum(total_steps) FILTER (WHERE timestamp < $2), 0)::int                      AS prev_steps,
			coalesce(percentile_disc(0.50) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE timestamp < $2), 0)::int AS prev_p50,
			coalesce(percentile_disc(0.95) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE timestamp < $2), 0)::int AS prev_p95,
			coalesce(percentile_disc(0.99) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE timestamp < $2), 0)::int AS prev_p99,
			count(DISTINCT agent_name) FILTER (WHERE timestamp < $2)::int                         AS prev_agents
		FROM agent_runs
		WHERE project_id = $1 AND timestamp >= $4 AND timestamp < $3
	`
	cur = &RunSummary{}
	prev = &RunSummary{}
	err = s.pool.QueryRow(ctx, q, projectID, from, to, prevFrom).Scan(
		&cur.TotalRuns, &cur.CompletedRuns, &cur.LoopRuns, &cur.AvgCostUSD, &cur.AvgTokens,
		&cur.TotalSteps, &cur.DurationP50Ms, &cur.DurationP95Ms, &cur.DurationP99Ms, &cur.UniqueAgents,
		&prev.TotalRuns, &prev.CompletedRuns, &prev.LoopRuns, &prev.AvgCostUSD, &prev.AvgTokens,
		&prev.TotalSteps, &prev.DurationP50Ms, &prev.DurationP95Ms, &prev.DurationP99Ms, &prev.UniqueAgents,
	)
	if err != nil {
		return nil, nil, err
	}
	cur.computeRates()
	prev.computeRates()
	return cur, prev, nil
}

type TerminationCount struct {
	TerminationReason string `json:"termination_reason"`
	Count             int    `json:"count"`
}

func (s *AgentRunStore) TerminationCounts(ctx context.Context, projectID uuid.UUID, from, to time.Time) ([]*TerminationCount, error) {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()
	const q = `
		SELECT coalesce(termination_reason, 'unknown') AS reason, count(*)::int
		FROM agent_runs
		WHERE project_id = $1 AND timestamp >= $2 AND timestamp < $3
		GROUP BY reason
		ORDER BY count(*) DESC
	`
	rows, err := s.pool.Query(ctx, q, projectID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*TerminationCount, 0)
	for rows.Next() {
		c := &TerminationCount{}
		if err := rows.Scan(&c.TerminationReason, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type RunBucket struct {
	Bucket    time.Time `json:"bucket"`
	Total     int       `json:"total"`
	Completed int       `json:"completed"`
	Failed    int       `json:"failed"`
	Loop      int       `json:"loop"`
}

// RunsTimeseries buckets runs into fixed intervals via TimescaleDB time_bucket,
// counting outcomes per bucket. interval is a Postgres interval literal
// (e.g. "1 hour"); the caller validates it against an allowlist. Buckets with
// no runs are omitted — the chart renders gaps.
func (s *AgentRunStore) RunsTimeseries(ctx context.Context, projectID uuid.UUID, from, to time.Time, interval string) ([]*RunBucket, error) {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	const q = `
		SELECT time_bucket($4::interval, timestamp)            AS bucket,
		       count(*)::int                                   AS total,
		       count(*) FILTER (WHERE status = 'completed')::int AS completed,
		       count(*) FILTER (WHERE status = 'failed')::int    AS failed,
		       count(*) FILTER (WHERE loop_detected)::int        AS loop
		FROM agent_runs
		WHERE project_id = $1 AND timestamp >= $2 AND timestamp < $3
		GROUP BY bucket
		ORDER BY bucket ASC
	`
	rows, err := s.pool.Query(ctx, q, projectID, from, to, interval)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*RunBucket, 0)
	for rows.Next() {
		b := &RunBucket{}
		if err := rows.Scan(&b.Bucket, &b.Total, &b.Completed, &b.Failed, &b.Loop); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
