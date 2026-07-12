package jobs

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type PostgresStore struct {
	database *sql.DB
	newID    func() string
}

func NewPostgresStore(database *sql.DB, newID func() string) *PostgresStore {
	return &PostgresStore{database: database, newID: newID}
}

func (store *PostgresStore) Enqueue(
	ctx context.Context,
	sessionID string,
	at time.Time,
) (*Job, error) {
	return scanJob(store.database.QueryRowContext(
		ctx,
		`INSERT INTO generation_jobs (
		  id, session_id, status, attempts, max_attempts, created_at, updated_at
		) VALUES ($1, $2, 'queued', 0, 3, $3, $3)
		ON CONFLICT (session_id) DO UPDATE SET session_id = EXCLUDED.session_id
		RETURNING id, session_id, status, attempts, max_attempts,
		          lease_owner, lease_until, created_at, updated_at`,
		store.newID(),
		sessionID,
		at.UTC(),
	), ErrNotFound)
}

func (store *PostgresStore) Claim(
	ctx context.Context,
	worker string,
	now time.Time,
	lease time.Duration,
) (*Job, error) {
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = transaction.Rollback() }()
	now = now.UTC()
	for {
		job, err := scanJob(transaction.QueryRowContext(
			ctx,
			claimJobQuery,
			now,
			worker,
			now.Add(lease),
		), ErrNoJob)
		if errors.Is(err, ErrNoJob) {
			return nil, ErrNoJob
		}
		if err != nil {
			return nil, err
		}
		if job.Status == StatusFailed {
			continue
		}
		if err := transaction.Commit(); err != nil {
			return nil, err
		}
		return job, nil
	}
}

func (store *PostgresStore) Get(ctx context.Context, jobID string) (*Job, error) {
	return scanJob(store.database.QueryRowContext(
		ctx,
		`SELECT id, session_id, status, attempts, max_attempts,
		        lease_owner, lease_until, created_at, updated_at
		 FROM generation_jobs WHERE id = $1`,
		jobID,
	), ErrNotFound)
}

func (store *PostgresStore) Complete(
	ctx context.Context,
	jobID string,
	worker string,
	at time.Time,
) error {
	return store.finish(ctx, jobID, worker, StatusCompleted, at)
}

func (store *PostgresStore) Fail(
	ctx context.Context,
	jobID string,
	worker string,
	at time.Time,
) error {
	return store.finish(ctx, jobID, worker, StatusFailed, at)
}

func (store *PostgresStore) finish(
	ctx context.Context,
	jobID string,
	worker string,
	status Status,
	at time.Time,
) error {
	result, err := store.database.ExecContext(
		ctx,
		`UPDATE generation_jobs
		 SET status = $1, lease_owner = NULL, lease_until = NULL, updated_at = $2
		 WHERE id = $3 AND status = 'running' AND lease_owner = $4`,
		status,
		at.UTC(),
		jobID,
		worker,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 1 {
		return nil
	}
	job, err := store.Get(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Status == status {
		return nil
	}
	return ErrConflict
}

func (store *PostgresStore) CancelBySession(
	ctx context.Context,
	sessionID string,
	at time.Time,
) error {
	_, err := store.database.ExecContext(
		ctx,
		`UPDATE generation_jobs
		 SET status = 'cancelled', lease_owner = NULL, lease_until = NULL,
		     updated_at = $1
		 WHERE session_id = $2 AND status IN ('queued', 'running')`,
		at.UTC(),
		sessionID,
	)
	return err
}

type scanner interface {
	Scan(...any) error
}

func scanJob(row scanner, notFound error) (*Job, error) {
	var job Job
	var leaseOwner sql.NullString
	var leaseUntil sql.NullTime
	if err := row.Scan(
		&job.ID,
		&job.SessionID,
		&job.Status,
		&job.Attempts,
		&job.MaxAttempts,
		&leaseOwner,
		&leaseUntil,
		&job.CreatedAt,
		&job.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, notFound
		}
		return nil, err
	}
	job.LeaseOwner = leaseOwner.String
	if leaseUntil.Valid {
		job.LeaseUntil = leaseUntil.Time.UTC()
	}
	return &job, nil
}

const claimJobQuery = `
WITH candidate AS (
  SELECT id
  FROM generation_jobs
  WHERE status = 'queued' OR (status = 'running' AND lease_until <= $1)
  ORDER BY created_at, id
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE generation_jobs AS job
SET
  status = CASE WHEN job.attempts >= job.max_attempts THEN 'failed' ELSE 'running' END,
  attempts = CASE WHEN job.attempts >= job.max_attempts THEN job.attempts ELSE job.attempts + 1 END,
  lease_owner = CASE WHEN job.attempts >= job.max_attempts THEN NULL ELSE $2::text END,
  lease_until = CASE WHEN job.attempts >= job.max_attempts THEN NULL ELSE $3::timestamptz END,
  updated_at = $1
FROM candidate
WHERE job.id = candidate.id
RETURNING job.id, job.session_id, job.status, job.attempts, job.max_attempts,
          job.lease_owner, job.lease_until, job.created_at, job.updated_at`
