ALTER TABLE alert_events ADD COLUMN IF NOT EXISTS recovering_since timestamptz;
