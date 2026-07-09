package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NotificationJob struct {
	ID           uuid.UUID  `json:"id"`
	AlertEventID uuid.UUID  `json:"alert_event_id"`
	Channel      string     `json:"channel"`
	Payload      []byte     `json:"payload"`
	Status       string     `json:"status"`
	Attempts     int        `json:"attempts"`
	RunAfter     time.Time  `json:"run_after"`
	LockedAt     *time.Time `json:"locked_at,omitempty"`
	LastError    *string    `json:"last_error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type NotificationJobStore struct {
	pool *pgxpool.Pool
}

// Enqueue participates in the caller's tx so the job is written in the same
// transaction as the alert_events row (transactional outbox) — the job only
// becomes visible if the alert state change commits.
func (s *NotificationJobStore) Enqueue(ctx context.Context, tx pgx.Tx, j *NotificationJob) error {
	const q = `
		INSERT INTO notification_jobs (alert_event_id, channel, payload, run_after)
		VALUES ($1,$2,$3,$4) RETURNING id, status, attempts, created_at`
	return tx.QueryRow(ctx, q, j.AlertEventID, j.Channel, j.Payload, j.RunAfter).
		Scan(&j.ID, &j.Status, &j.Attempts, &j.CreatedAt)
}

// Claim atomically leases up to `limit` due pending jobs using FOR UPDATE SKIP
// LOCKED, so concurrent workers never grab the same row within a transaction,
// and flips them to 'processing' so a later poll (or another replica) can't
// re-claim the same rows once this claim commits — SKIP LOCKED only guards
// concurrent open transactions, not committed 'pending' rows. MarkDone/MarkRetry
// move the job out of 'processing' (done/dead) or back to 'pending' for retry.
func (s *NotificationJobStore) Claim(ctx context.Context, limit int) ([]*NotificationJob, error) {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()
	const q = `
		WITH claimed AS (
			SELECT id FROM notification_jobs
			WHERE status = 'pending' AND run_after <= now()
			ORDER BY run_after
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE notification_jobs j SET status = 'processing', locked_at = now()
		FROM claimed WHERE j.id = claimed.id
		RETURNING j.id, j.alert_event_id, j.channel, j.payload, j.status, j.attempts,
			j.run_after, j.locked_at, j.last_error, j.created_at`
	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*NotificationJob, 0)
	for rows.Next() {
		j := &NotificationJob{}
		if err := rows.Scan(&j.ID, &j.AlertEventID, &j.Channel, &j.Payload, &j.Status,
			&j.Attempts, &j.RunAfter, &j.LockedAt, &j.LastError, &j.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *NotificationJobStore) MarkDone(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()
	_, err := s.pool.Exec(ctx, `UPDATE notification_jobs SET status='done', locked_at=NULL WHERE id=$1`, id)
	return err
}

// MarkRetry records a failed attempt: reschedule (dead=false) or give up (dead=true).
func (s *NotificationJobStore) MarkRetry(ctx context.Context, id uuid.UUID, errMsg string, runAfter time.Time, dead bool) error {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()
	status := "pending"
	if dead {
		status = "dead"
	}
	const q = `UPDATE notification_jobs
		SET status=$2, attempts=attempts+1, run_after=$3, last_error=$4, locked_at=NULL
		WHERE id=$1`
	_, err := s.pool.Exec(ctx, q, id, status, runAfter, errMsg)
	return err
}
