-- alert_rules: user-defined alert conditions on agent signals.
CREATE TABLE IF NOT EXISTS alert_rules (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    project_id      uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    agent_name      text,
    name            text NOT NULL,
    class           text NOT NULL CHECK (class IN ('event', 'aggregate')),
    signal          text NOT NULL CHECK (signal IN (
                        'run_failure','loop','termination_shift','cost_burn',
                        'tool_failure','duration_p95','eval_regression')),
    comparator      text NOT NULL DEFAULT '>' CHECK (comparator IN ('>','>=','<','<=')),
    threshold       double precision NOT NULL DEFAULT 0,
    window_seconds  int,
    severity        text NOT NULL DEFAULT 'page' CHECK (severity IN ('page','warn','digest')),
    for_seconds     int NOT NULL DEFAULT 0,
    keep_firing_for_seconds int NOT NULL DEFAULT 0,
    cooldown_seconds int NOT NULL DEFAULT 900,
    min_requests    int NOT NULL DEFAULT 0,
    channel         text NOT NULL DEFAULT 'email' CHECK (channel IN ('email','slack','webhook','pagerduty')),
    channel_config  jsonb NOT NULL DEFAULT '{}'::jsonb,
    enabled         bool NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS alert_rules_project_idx ON alert_rules (project_id) WHERE enabled;

-- alert_events: one live row per (rule, fingerprint); history retained.
CREATE TABLE IF NOT EXISTS alert_events (
    id                uuid PRIMARY KEY DEFAULT uuidv7(),
    rule_id           uuid NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    project_id        uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    fingerprint       bytea NOT NULL,
    scope_label       text NOT NULL DEFAULT '',
    state             text NOT NULL CHECK (state IN ('pending','firing','recovering','resolved','fired')),
    first_breached_at timestamptz,
    fired_at          timestamptz,
    resolved_at       timestamptz,
    last_fired_at     timestamptz,
    last_value        double precision,
    last_evaluated_at timestamptz NOT NULL DEFAULT now()
);
-- one live (non-resolved) row per rule+fingerprint
CREATE UNIQUE INDEX IF NOT EXISTS alert_events_live_idx
    ON alert_events (rule_id, fingerprint)
    WHERE state <> 'resolved';
CREATE INDEX IF NOT EXISTS alert_events_project_idx ON alert_events (project_id, id DESC);

-- notification_jobs: durable delivery queue (transactional outbox).
CREATE TABLE IF NOT EXISTS notification_jobs (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    alert_event_id  uuid NOT NULL REFERENCES alert_events(id) ON DELETE CASCADE,
    channel         text NOT NULL,
    payload         jsonb NOT NULL,
    status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','done','dead')),
    attempts        int NOT NULL DEFAULT 0,
    run_after       timestamptz NOT NULL DEFAULT now(),
    locked_at       timestamptz,
    last_error      text,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS notification_jobs_claim_idx
    ON notification_jobs (run_after)
    WHERE status = 'pending';
