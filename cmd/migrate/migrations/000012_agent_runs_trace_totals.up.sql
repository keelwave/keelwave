-- Keep agent_runs.total_tokens / total_cost_usd in sync with a run's linked
-- ai_traces. The agent_runs_5m continuous aggregate sums these raw columns and
-- feeds cost/token alerting, and a cagg cannot join ai_traces -- so the columns
-- themselves must be correct. LLM tokens/cost usually arrive as linked ai_traces
-- (e.g. via wrapModel) after the run finishes, so a finish-time rollup misses
-- them; this trigger backfills as each trace lands (prefer-traces semantic,
-- matching how the query layer derives these values at read time).

CREATE OR REPLACE FUNCTION agent_runs_sync_trace_totals() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.agent_run_id IS NOT NULL THEN
        UPDATE agent_runs r SET
            total_tokens = coalesce((
                SELECT sum(coalesce(t.total_tokens,
                                    coalesce(t.input_tokens, 0) + coalesce(t.output_tokens, 0)))
                FROM ai_traces t
                WHERE t.agent_run_id = r.id AND t.project_id = r.project_id
            ), r.total_tokens),
            total_cost_usd = coalesce((
                SELECT sum(t.cost_usd)
                FROM ai_traces t
                WHERE t.agent_run_id = r.id AND t.project_id = r.project_id
            ), r.total_cost_usd)
        WHERE r.id = NEW.agent_run_id AND r.project_id = NEW.project_id;
    END IF;
    RETURN NULL;
END;
$$;

CREATE TRIGGER ai_traces_sync_run_totals
    AFTER INSERT ON ai_traces
    FOR EACH ROW
    EXECUTE FUNCTION agent_runs_sync_trace_totals();

-- One-time backfill of existing runs that have linked traces.
UPDATE agent_runs r SET
    total_tokens = coalesce((
        SELECT sum(coalesce(t.total_tokens,
                            coalesce(t.input_tokens, 0) + coalesce(t.output_tokens, 0)))
        FROM ai_traces t
        WHERE t.agent_run_id = r.id AND t.project_id = r.project_id
    ), r.total_tokens),
    total_cost_usd = coalesce((
        SELECT sum(t.cost_usd)
        FROM ai_traces t
        WHERE t.agent_run_id = r.id AND t.project_id = r.project_id
    ), r.total_cost_usd)
WHERE EXISTS (
    SELECT 1 FROM ai_traces t WHERE t.agent_run_id = r.id AND t.project_id = r.project_id
);
