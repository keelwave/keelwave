-- Add a 'processing' status so Claim can transition a leased job out of the
-- 'pending' set in the same UPDATE. FOR UPDATE SKIP LOCKED only prevents
-- concurrent locks within one open transaction; once Claim commits, the row is
-- unlocked and still 'pending', so a later poll (or a second worker replica)
-- re-selects it and re-sends the notification. Moving to 'processing' on claim
-- removes it from the WHERE status='pending' set (and the partial claim index).
ALTER TABLE notification_jobs DROP CONSTRAINT notification_jobs_status_check;
ALTER TABLE notification_jobs ADD CONSTRAINT notification_jobs_status_check
    CHECK (status IN ('pending','processing','done','dead'));
