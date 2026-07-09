-- Dropping the continuous aggregate removes its refresh policy automatically.
DROP MATERIALIZED VIEW IF EXISTS agent_runs_5m;
