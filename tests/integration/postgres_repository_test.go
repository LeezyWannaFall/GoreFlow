//go:build integration

package integration_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/LeezyWannaFall/GoreFlow/internal/application"
	"github.com/LeezyWannaFall/GoreFlow/internal/job"
	"github.com/LeezyWannaFall/GoreFlow/internal/storage/postgres"
	"github.com/google/uuid"
)

func TestRepository_CreateAndGetJob(t *testing.T) {
	db := openTestDatabase(t)
	repository := postgres.NewRepository(db)
	payload := json.RawMessage(`{"message":"integration","nested":{"value":1}}`)
	created, err := job.NewJob("echo", payload)
	if err != nil {
		t.Fatalf("NewJob() unexpected error = %v", err)
	}

	if err := repository.CreateJob(t.Context(), created); err != nil {
		t.Fatalf("CreateJob() unexpected error = %v", err)
	}

	stored, err := repository.GetJobByID(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("GetJobByID() unexpected error = %v", err)
	}

	if stored.ID != created.ID {
		t.Errorf("GetJobByID() ID = %s, want %s", stored.ID, created.ID)
	}
	if stored.Type != created.Type {
		t.Errorf("GetJobByID() type = %q, want %q", stored.Type, created.Type)
	}
	if !jsonEqual(stored.Payload, created.Payload) {
		t.Errorf("GetJobByID() payload = %s, want %s", stored.Payload, created.Payload)
	}
	if stored.Status != job.StatusQueued {
		t.Errorf("GetJobByID() status = %q, want %q", stored.Status, job.StatusQueued)
	}
	if stored.Attempt != 0 || stored.MaxAttempts != 1 {
		t.Errorf("GetJobByID() attempts = %d/%d, want 0/1", stored.Attempt, stored.MaxAttempts)
	}
	if !stored.RunAfter.Equal(created.RunAfter.Truncate(time.Microsecond)) {
		t.Errorf("GetJobByID() run after = %s, want %s", stored.RunAfter, created.RunAfter.Truncate(time.Microsecond))
	}
	if stored.LockedBy != "" || !stored.LeaseUntil.IsZero() || stored.Result != nil || stored.Error != "" {
		t.Errorf("GetJobByID() nullable execution fields were not empty: %#v", stored)
	}
}

func TestRepository_ClaimAndUpdateJob(t *testing.T) {
	db := openTestDatabase(t)
	repository := postgres.NewRepository(db)
	created, err := job.NewJob("echo", json.RawMessage(`{"message":"claim me"}`))
	if err != nil {
		t.Fatalf("NewJob() unexpected error = %v", err)
	}
	if err := repository.CreateJob(t.Context(), created); err != nil {
		t.Fatalf("CreateJob() unexpected error = %v", err)
	}

	claimTime := created.RunAfter.Add(time.Second)
	leaseDuration := 30 * time.Second
	claimed, err := repository.ClaimJob(t.Context(), "integration-worker", claimTime, leaseDuration)
	if err != nil {
		t.Fatalf("ClaimJob() unexpected error = %v", err)
	}
	if claimed.ID != created.ID {
		t.Errorf("ClaimJob() ID = %s, want %s", claimed.ID, created.ID)
	}
	if claimed.Status != job.StatusRunning || claimed.Attempt != 1 {
		t.Errorf("ClaimJob() status/attempt = %q/%d, want %q/1", claimed.Status, claimed.Attempt, job.StatusRunning)
	}
	if claimed.LockedBy != "integration-worker" {
		t.Errorf("ClaimJob() locked by = %q, want integration-worker", claimed.LockedBy)
	}
	if !claimed.LeaseUntil.Equal(claimTime.Add(leaseDuration)) {
		t.Errorf("ClaimJob() lease until = %s, want %s", claimed.LeaseUntil, claimTime.Add(leaseDuration))
	}

	result := json.RawMessage(`{"delivered":true}`)
	if err := claimed.Complete(result, claimTime.Add(2*time.Second)); err != nil {
		t.Fatalf("Complete() unexpected error = %v", err)
	}
	if err := repository.UpdateJob(t.Context(), &claimed); err != nil {
		t.Fatalf("UpdateJob() unexpected error = %v", err)
	}

	stored, err := repository.GetJobByID(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("GetJobByID() unexpected error = %v", err)
	}
	if stored.Status != job.StatusSucceeded || stored.Attempt != 1 {
		t.Errorf("updated job status/attempt = %q/%d, want %q/1", stored.Status, stored.Attempt, job.StatusSucceeded)
	}
	if !jsonEqual(stored.Result, result) {
		t.Errorf("updated job result = %s, want %s", stored.Result, result)
	}
	if stored.LockedBy != "" || !stored.LeaseUntil.IsZero() || stored.Error != "" {
		t.Errorf("updated job execution fields were not cleared: %#v", stored)
	}
}

func TestRepository_NotFoundAndNoJobAvailable(t *testing.T) {
	db := openTestDatabase(t)
	repository := postgres.NewRepository(db)

	_, err := repository.GetJobByID(t.Context(), uuid.New())
	if !errors.Is(err, application.ErrJobNotFound) {
		t.Errorf("GetJobByID() error = %v, want errors.Is(_, %v)", err, application.ErrJobNotFound)
	}

	_, err = repository.ClaimJob(t.Context(), "integration-worker", time.Now().UTC(), 30*time.Second)
	if !errors.Is(err, application.ErrNoJobAvailable) {
		t.Errorf("ClaimJob() error = %v, want errors.Is(_, %v)", err, application.ErrNoJobAvailable)
	}

	missing := &job.Job{ID: uuid.New(), Status: job.StatusSucceeded, Attempt: 1, UpdatedAt: time.Now().UTC()}
	err = repository.UpdateJob(t.Context(), missing)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("UpdateJob() error = %v, want errors.Is(_, %v)", err, sql.ErrNoRows)
	}
}

func TestRepository_ClaimJobSkipsFutureJob(t *testing.T) {
	db := openTestDatabase(t)
	repository := postgres.NewRepository(db)
	futureJob, err := job.NewJob("echo", json.RawMessage(`{"message":"not yet"}`))
	if err != nil {
		t.Fatalf("NewJob() unexpected error = %v", err)
	}
	futureJob.RunAfter = time.Now().UTC().Add(time.Hour)
	if err := repository.CreateJob(t.Context(), futureJob); err != nil {
		t.Fatalf("CreateJob() unexpected error = %v", err)
	}

	_, err = repository.ClaimJob(t.Context(), "integration-worker", time.Now().UTC(), 30*time.Second)
	if !errors.Is(err, application.ErrNoJobAvailable) {
		t.Errorf("ClaimJob() error = %v, want errors.Is(_, %v)", err, application.ErrNoJobAvailable)
	}
}
