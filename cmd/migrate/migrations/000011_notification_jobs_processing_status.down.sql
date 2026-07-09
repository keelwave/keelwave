-- Settle any in-flight leases back to pending before narrowing the constraint,
-- so the CHECK can't reject an existing 'processing' row.
UPDATE notification_jobs SET status = 'pending', locked_at = NULL WHERE status = 'processing';
ALTER TABLE notification_jobs DROP CONSTRAINT notification_jobs_status_check;
ALTER TABLE notification_jobs ADD CONSTRAINT notification_jobs_status_check
    CHECK (status IN ('pending','done','dead'));
