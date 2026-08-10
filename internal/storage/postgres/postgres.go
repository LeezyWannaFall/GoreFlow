package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/LeezyWannaFall/GoreFlow/internal/job"
	"github.com/google/uuid"
)

type rowScanner interface {
	Scan(dest ...any) error
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateJob(ctx context.Context, model *job.Job) error {
	if model == nil {
		return errors.New("create job: job must not be nil")
	}

	query := `
		INSERT INTO jobs (
			id,
			type,
			payload,
			status,
			attempt,
			max_attempts,
			run_after,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := r.db.ExecContext(
		ctx,
		query,
		model.ID,
		model.Type,
		model.Payload,
		model.Status,
		model.Attempt,
		model.MaxAttempts,
		model.RunAfter,
		model.CreatedAt,
		model.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("create job: %w", err)
	}

	return nil
}

func (r *Repository) ClaimJob(ctx context.Context, workerID string, now time.Time, leaseDuration time.Duration) (job.Job, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return job.Job{}, fmt.Errorf("begin claim job transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	selectQuery := `
		SELECT id, type, payload, status, attempt, max_attempts, run_after, locked_by, lease_until, result, error, created_at, updated_at
		FROM jobs
		WHERE status = $1
		  AND run_after <= $2
		  AND attempt < max_attempts
		ORDER BY run_after ASC, created_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`

	model, err := scanJob(tx.QueryRowContext(ctx, selectQuery, job.StatusQueued, now))
	if err != nil {
		return job.Job{}, fmt.Errorf("select job for claim: %w", err)
	}

	if err := model.Start(workerID, now, leaseDuration); err != nil {
		return job.Job{}, fmt.Errorf("start claimed job: %w", err)
	}

	updateQuery := `
		UPDATE jobs
		SET status = $1,
			attempt = $2,
			locked_by = $3,
			lease_until = $4,
			updated_at = $5
		WHERE id = $6
	`

	updateResult, err := tx.ExecContext(
		ctx,
		updateQuery,
		model.Status,
		model.Attempt,
		model.LockedBy,
		model.LeaseUntil,
		model.UpdatedAt,
		model.ID,
	)
	if err != nil {
		return job.Job{}, fmt.Errorf("save claimed job: %w", err)
	}
	if err := requireOneAffectedRow(updateResult, "save claimed job"); err != nil {
		return job.Job{}, err
	}

	if err := tx.Commit(); err != nil {
		return job.Job{}, fmt.Errorf("commit claim job transaction: %w", err)
	}

	return model, nil
}

func (r *Repository) GetJobByID(ctx context.Context, id uuid.UUID) (job.Job, error) {
	query := `
		SELECT id, type, payload, status, attempt, max_attempts, run_after, locked_by, lease_until, result, error, created_at, updated_at
		FROM jobs
		WHERE id = $1
	`

	model, err := scanJob(r.db.QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return job.Job{}, fmt.Errorf("job not found: %w", err)
		}
		return job.Job{}, fmt.Errorf("get job by ID: %w", err)
	}

	return model, nil
}

func (r *Repository) UpdateJob(ctx context.Context, model *job.Job) error {
	if model == nil {
		return errors.New("update job: job must not be nil")
	}

	query := `
		UPDATE jobs
		SET status = $1,
			attempt = $2,
			locked_by = $3,
			lease_until = $4,
			result = $5,
			error = $6,
			updated_at = $7
		WHERE id = $8
	`

	lockedBy := sql.NullString{String: model.LockedBy, Valid: model.LockedBy != ""}
	leaseUntil := sql.NullTime{Time: model.LeaseUntil, Valid: !model.LeaseUntil.IsZero()}
	errorText := sql.NullString{String: model.Error, Valid: model.Error != ""}

	var result any
	if model.Result != nil {
		result = model.Result
	}

	updateResult, err := r.db.ExecContext(
		ctx,
		query,
		model.Status,
		model.Attempt,
		lockedBy,
		leaseUntil,
		result,
		errorText,
		model.UpdatedAt,
		model.ID,
	)

	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}

	return requireOneAffectedRow(updateResult, "update job")
}

func scanJob(scanner rowScanner) (job.Job, error) {
	var (
		model      job.Job
		payload    []byte
		status     string
		lockedBy   sql.NullString
		leaseUntil sql.NullTime
		result     []byte
		errorText  sql.NullString
	)

	err := scanner.Scan(
		&model.ID,
		&model.Type,
		&payload,
		&status,
		&model.Attempt,
		&model.MaxAttempts,
		&model.RunAfter,
		&lockedBy,
		&leaseUntil,
		&result,
		&errorText,
		&model.CreatedAt,
		&model.UpdatedAt,
	)
	if err != nil {
		return job.Job{}, err
	}

	model.Payload = payload
	model.Status = job.Status(status)
	model.Result = result
	model.RunAfter = model.RunAfter.UTC()
	model.CreatedAt = model.CreatedAt.UTC()
	model.UpdatedAt = model.UpdatedAt.UTC()

	if lockedBy.Valid {
		model.LockedBy = lockedBy.String
	}
	if leaseUntil.Valid {
		model.LeaseUntil = leaseUntil.Time.UTC()
	}
	if errorText.Valid {
		model.Error = errorText.String
	}

	return model, nil
}

func requireOneAffectedRow(result sql.Result, operation string) error {
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: get affected rows: %w", operation, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("%s: %w", operation, sql.ErrNoRows)
	}
	if rowsAffected != 1 {
		return fmt.Errorf("%s: expected 1 affected row, got %d", operation, rowsAffected)
	}

	return nil
}
