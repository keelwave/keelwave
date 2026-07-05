CREATE INDEX IF NOT EXISTS ai_traces_agent_run_idx
    ON ai_traces (agent_run_id)
    WHERE agent_run_id IS NOT NULL;
