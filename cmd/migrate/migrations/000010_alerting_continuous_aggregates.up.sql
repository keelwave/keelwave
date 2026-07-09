-- agent_runs_5m: continuous aggregate rolling up agent_runs by 5-minute bucket,
-- project, and agent. Feeds aggregate alert rules (cost_burn, run_failure,
-- termination_shift, tool_failure). Percentiles (duration_p95) are computed
-- query-time on the raw hypertable, not materialized here.
--
-- materialized_only = false turns real-time aggregation ON: evaluator queries
-- union the materialized buckets with a live aggregate of the un-materialized raw
-- tail, so a run at now() is counted immediately (the refresh policy only
-- materializes up to end_offset = 5m ago). TimescaleDB >= 2.13 defaults this to
-- true, and the session GUC `timescaledb.materialized_only` does NOT override the
-- per-view setting, so it MUST be set on the view here.
CREATE MATERIALIZED VIEW IF NOT EXISTS agent_runs_5m
WITH (timescaledb.continuous, timescaledb.materialized_only = false) AS
SELECT
    project_id,
    agent_name,
    time_bucket('5 minutes', timestamp) AS bucket,
    count(*)                                                        AS total_runs,
    count(*) FILTER (WHERE status = 'completed')                   AS completed_runs,
    count(*) FILTER (WHERE status = 'failed')                      AS failed_runs,
    count(*) FILTER (WHERE termination_reason IN ('error','timeout','max_steps_reached')) AS bad_termination_runs,
    coalesce(sum(total_cost_usd), 0)                               AS cost_usd,
    coalesce(sum(total_tokens), 0)                                 AS tokens
FROM agent_runs
GROUP BY project_id, agent_name, bucket
WITH NO DATA;

-- Refresh policy: materialize buckets from 1 day ago up to 5m ago, every minute.
-- The 5m end_offset leaves the recent tail to real-time aggregation. The 1-day
-- start_offset covers any realistic run duration: a run finishing long after it
-- started invalidates its start bucket, which must still fall inside the refresh
-- window or the finish (cost, completion) is never materialized. Requires
-- TimescaleDB background workers (running in the dev/prod container).
SELECT add_continuous_aggregate_policy('agent_runs_5m',
    start_offset => INTERVAL '1 day',
    end_offset   => INTERVAL '5 minutes',
    schedule_interval => INTERVAL '1 minute',
    if_not_exists => true);
