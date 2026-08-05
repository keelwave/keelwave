package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound          = errors.New("resource not found")
	ErrConflict          = errors.New("resource already exists")
	QueryTimeoutDuration = 5 * time.Second
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

type Storage struct {
	Projects interface {
		Create(ctx context.Context, p *Project, orgID uuid.UUID) error
		GetByID(ctx context.Context, id uuid.UUID) (*Project, error)
		GetByIDForUser(ctx context.Context, id, userID uuid.UUID) (*Project, error)
		ListByOrg(ctx context.Context, orgID uuid.UUID) ([]*Project, error)
		ListByUser(ctx context.Context, userID uuid.UUID) ([]*Project, error)
		Delete(ctx context.Context, id uuid.UUID) error
	}
	APIKeys interface {
		Create(ctx context.Context, k *APIKey) error
		GetByHash(ctx context.Context, hash []byte) (*APIKey, error)
		TouchLastUsed(ctx context.Context, id uuid.UUID) error
		ListByProject(ctx context.Context, projectID uuid.UUID) ([]*APIKey, error)
		Delete(ctx context.Context, id, projectID uuid.UUID) error
	}
	Users interface {
		Create(ctx context.Context, u *User, tx pgx.Tx) error
		CreateWithVerification(ctx context.Context, u *User, v *UserVerification) error
		CreateWithIdentity(ctx context.Context, u *User, identity *UserIdentity) error
		GetByID(ctx context.Context, id uuid.UUID) (*User, error)
		GetByEmail(ctx context.Context, email string) (*User, error)
		Verify(ctx context.Context, tokenHash []byte) error
	}
	UserVerifications interface {
		Create(ctx context.Context, v *UserVerification) error
		GetByToken(ctx context.Context, hash []byte) (*UserVerification, error)
		DeleteByUser(ctx context.Context, userID uuid.UUID) error
	}
	UserIdentities interface {
		Create(ctx context.Context, i *UserIdentity) error
		GetByProvider(ctx context.Context, provider, providerUserID string) (*UserIdentity, error)
	}
	Organizations interface {
		CreateWithOwner(ctx context.Context, a *Organization, ownerUserID uuid.UUID) error
		GetByID(ctx context.Context, id uuid.UUID) (*Organization, error)
		Update(ctx context.Context, id uuid.UUID, name string) (*Organization, error)
		ListByUser(ctx context.Context, userID uuid.UUID) ([]*Organization, error)
		Delete(ctx context.Context, id uuid.UUID) error
	}
	OrganizationInvites interface {
		Create(ctx context.Context, inv *OrganizationInvite) error
		GetByToken(ctx context.Context, hash []byte) (*OrganizationInvite, error)
		ListByOrg(ctx context.Context, orgID uuid.UUID) ([]*OrganizationInvite, error)
		Accept(ctx context.Context, inviteID, userID uuid.UUID, role string) error
		Delete(ctx context.Context, id uuid.UUID) error
	}
	OrganizationMembers interface {
		Get(ctx context.Context, organizationID, userID uuid.UUID) (*OrganizationMember, error)
		ListByOrganization(ctx context.Context, organizationID uuid.UUID) ([]*MemberWithUser, error)
		UpdateRole(ctx context.Context, organizationID, userID uuid.UUID, role string) error
		Remove(ctx context.Context, organizationID, userID uuid.UUID) error
	}
	Sessions interface {
		Create(ctx context.Context, sess *Session) error
		GetByHash(ctx context.Context, hash []byte) (*Session, error)
		TouchLastUsed(ctx context.Context, id uuid.UUID) error
		Delete(ctx context.Context, id uuid.UUID) error
		DeleteByUser(ctx context.Context, userID uuid.UUID) error
		DeleteExpired(ctx context.Context) (int64, error)
	}
	AITraces interface {
		Insert(ctx context.Context, t *AITrace) error
	}
	APIEvents interface {
		Insert(ctx context.Context, e *APIEvent) error
	}
	InfraMetrics interface {
		Insert(ctx context.Context, m *InfraMetric) error
	}
	AgentRuns interface {
		Insert(ctx context.Context, r *AgentRun) error
		Finish(ctx context.Context, id uuid.UUID, ts time.Time, f AgentRunFinish) error
		GetByID(ctx context.Context, projectID, runID uuid.UUID, from, to time.Time) (*AgentRun, error)
		ListByProject(ctx context.Context, projectID uuid.UUID, from, to time.Time, limit, offset int) ([]*AgentRun, error)
		RunHealth(ctx context.Context, projectID uuid.UUID, from, to time.Time) ([]*RunHealthRow, error)
		RunsTimeseries(ctx context.Context, projectID uuid.UUID, from, to time.Time, interval string) ([]*RunBucket, error)
		SummaryWithPrev(ctx context.Context, projectID uuid.UUID, from, to time.Time) (cur, prev *RunSummary, err error)
		TerminationCounts(ctx context.Context, projectID uuid.UUID, from, to time.Time) ([]*TerminationCount, error)
	}
	AgentSteps interface {
		Insert(ctx context.Context, st *AgentStep) error
		CountFingerprint(ctx context.Context, runID uuid.UUID, fingerprint []byte) (int, error)
		ListByRun(ctx context.Context, projectID, runID uuid.UUID, from, to time.Time, limit int) ([]*AgentStep, error)
		ListLoops(ctx context.Context, projectID, runID uuid.UUID, from, to time.Time) ([]*LoopHit, error)
		ToolStats(ctx context.Context, projectID uuid.UUID, from, to time.Time) ([]*ToolStat, error)
		StepTypeDistribution(ctx context.Context, projectID uuid.UUID, from, to time.Time) ([]*StepTypeCount, error)
		StepCountsWithPrev(ctx context.Context, projectID uuid.UUID, from, to time.Time) (cur, prev *StepCounts, err error)
	}
	AgentTools interface {
		UpsertSeen(ctx context.Context, projectID uuid.UUID, toolName string) error
		ListByProject(ctx context.Context, projectID uuid.UUID) ([]*AgentTool, error)
	}
	AgentEvaluations interface {
		Insert(ctx context.Context, e *AgentEvaluation) error
		ListByRun(ctx context.Context, runID uuid.UUID) ([]*AgentEvaluation, error)
	}
	AlertRules interface {
		Create(ctx context.Context, r *AlertRule) error
		GetByID(ctx context.Context, id, projectID uuid.UUID) (*AlertRule, error)
		ListByProject(ctx context.Context, projectID uuid.UUID) ([]*AlertRule, error)
		ListEnabled(ctx context.Context) ([]*AlertRule, error)
		Update(ctx context.Context, r *AlertRule) error
		Delete(ctx context.Context, id, projectID uuid.UUID) error
	}
	AlertEvents interface {
		GetLive(ctx context.Context, ruleID uuid.UUID, fingerprint []byte) (*AlertEvent, error)
		Upsert(ctx context.Context, tx pgx.Tx, e *AlertEvent) error
		FireEvent(ctx context.Context, tx pgx.Tx, e *AlertEvent, cooldownSeconds int) (uuid.UUID, bool, error)
		ListByProject(ctx context.Context, projectID uuid.UUID, limit int) ([]*AlertEvent, error)
		ListByProjectFiltered(ctx context.Context, projectID uuid.UUID, state string, limit int) ([]*AlertEventWithDelivery, error)
	}
	NotificationJobs interface {
		Enqueue(ctx context.Context, tx pgx.Tx, j *NotificationJob) error
		Claim(ctx context.Context, limit int) ([]*NotificationJob, error)
		MarkDone(ctx context.Context, id uuid.UUID) error
		MarkRetry(ctx context.Context, id uuid.UUID, errMsg string, runAfter time.Time, dead bool) error
	}
}

func withTx(pool *pgxpool.Pool, ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

func NewStorage(pool *pgxpool.Pool) Storage {
	members := &OrganizationMemberStore{pool: pool}
	identities := &UserIdentityStore{pool: pool}
	verifications := &UserVerificationStore{pool: pool}

	return Storage{
		Projects:            &ProjectStore{pool: pool},
		APIKeys:             &APIKeyStore{pool: pool},
		Users:               &UserStore{pool: pool, identities: identities, verifications: verifications},
		UserVerifications:   verifications,
		UserIdentities:      identities,
		Organizations:       &OrganizationStore{pool: pool, members: members},
		OrganizationInvites: &OrganizationInviteStore{pool: pool, members: members},
		OrganizationMembers: members,
		Sessions:            &SessionStore{pool: pool},
		AITraces:            &AITraceStore{pool: pool},
		APIEvents:           &APIEventStore{pool: pool},
		InfraMetrics:        &InfraMetricStore{pool: pool},
		AgentRuns:           &AgentRunStore{pool: pool},
		AgentSteps:          &AgentStepStore{pool: pool},
		AgentTools:          &AgentToolStore{pool: pool},
		AgentEvaluations:    &AgentEvaluationStore{pool: pool},
		AlertRules:          &AlertRuleStore{pool: pool},
		AlertEvents:         &AlertEventStore{pool: pool},
		NotificationJobs:    &NotificationJobStore{pool: pool},
	}
}
