package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/LeezyWannaFall/GoreFlow/internal/executor"
	"github.com/LeezyWannaFall/GoreFlow/internal/job"
	"github.com/google/uuid"
)

type fakeWorkerJobRepository struct {
	claimCalls         int
	claimCtx           context.Context
	claimedBy          string
	claimTime          time.Time
	claimLeaseDuration time.Duration
	claimResult        job.Job
	claimErr           error

	updateCalls int
	updateCtx   context.Context
	updatedJob  *job.Job
	updateErr   error
}

func (f *fakeWorkerJobRepository) ClaimJob(
	ctx context.Context,
	workerID string,
	now time.Time,
	leaseDuration time.Duration,
) (job.Job, error) {
	f.claimCalls++
	f.claimCtx = ctx
	f.claimedBy = workerID
	f.claimTime = now
	f.claimLeaseDuration = leaseDuration
	return f.claimResult, f.claimErr
}

func (f *fakeWorkerJobRepository) UpdateJob(ctx context.Context, model *job.Job) error {
	f.updateCalls++
	f.updateCtx = ctx
	if model != nil {
		modelCopy := *model
		f.updatedJob = &modelCopy
	}
	return f.updateErr
}

type fakeExecutorResolver struct {
	getCalls      int
	requestedName string
	executor      executor.Executor
	exists        bool
}

func (f *fakeExecutorResolver) Get(name string) (executor.Executor, bool) {
	f.getCalls++
	f.requestedName = name
	return f.executor, f.exists
}

type fakeExecutor struct {
	executeCalls int
	executeCtx   context.Context
	payload      json.RawMessage
	result       json.RawMessage
	err          error
}

func (f *fakeExecutor) Execute(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	f.executeCalls++
	f.executeCtx = ctx
	f.payload = payload
	return f.result, f.err
}

func TestJobProcessor_ProcessNextJob(t *testing.T) {
	repositoryErr := errors.New("repository unavailable")
	updateErr := errors.New("update unavailable")
	executorErr := errors.New("executor failed")
	result := json.RawMessage(`{"message":"hello"}`)

	testCases := []struct {
		name             string
		jobType          string
		jobStatus        job.Status
		claimErr         error
		resolverExists   bool
		executeResult    json.RawMessage
		executeErr       error
		updateErr        error
		wantErr          bool
		wantErrorIs      error
		wantResolverCall int
		wantExecuteCalls int
		wantUpdateCalls  int
		wantStatus       job.Status
		wantResult       json.RawMessage
		wantJobError     string
	}{
		{
			name:             "completes a successfully executed job",
			jobType:          "echo",
			jobStatus:        job.StatusRunning,
			resolverExists:   true,
			executeResult:    result,
			wantResolverCall: 1,
			wantExecuteCalls: 1,
			wantUpdateCalls:  1,
			wantStatus:       job.StatusSucceeded,
			wantResult:       result,
		},
		{
			name:             "persists an executor failure",
			jobType:          "echo",
			jobStatus:        job.StatusRunning,
			resolverExists:   true,
			executeErr:       executorErr,
			wantResolverCall: 1,
			wantExecuteCalls: 1,
			wantUpdateCalls:  1,
			wantStatus:       job.StatusFailed,
			wantJobError:     "failed to execute job: executor failed",
		},
		{
			name:             "persists an unknown executor failure",
			jobType:          "unknown",
			jobStatus:        job.StatusRunning,
			resolverExists:   false,
			wantResolverCall: 1,
			wantExecuteCalls: 0,
			wantUpdateCalls:  1,
			wantStatus:       job.StatusFailed,
			wantJobError:     "executor not found for job type: unknown",
		},
		{
			name:             "preserves the no job available error",
			jobType:          "echo",
			jobStatus:        job.StatusRunning,
			claimErr:         ErrNoJobAvailable,
			wantErr:          true,
			wantErrorIs:      ErrNoJobAvailable,
			wantResolverCall: 0,
			wantExecuteCalls: 0,
			wantUpdateCalls:  0,
		},
		{
			name:             "preserves a claim repository error",
			jobType:          "echo",
			jobStatus:        job.StatusRunning,
			claimErr:         repositoryErr,
			wantErr:          true,
			wantErrorIs:      repositoryErr,
			wantResolverCall: 0,
			wantExecuteCalls: 0,
			wantUpdateCalls:  0,
		},
		{
			name:             "preserves an update error after successful execution",
			jobType:          "echo",
			jobStatus:        job.StatusRunning,
			resolverExists:   true,
			executeResult:    result,
			updateErr:        updateErr,
			wantErr:          true,
			wantErrorIs:      updateErr,
			wantResolverCall: 1,
			wantExecuteCalls: 1,
			wantUpdateCalls:  1,
			wantStatus:       job.StatusSucceeded,
			wantResult:       result,
		},
		{
			name:             "preserves an update error after executor failure",
			jobType:          "echo",
			jobStatus:        job.StatusRunning,
			resolverExists:   true,
			executeErr:       executorErr,
			updateErr:        updateErr,
			wantErr:          true,
			wantErrorIs:      updateErr,
			wantResolverCall: 1,
			wantExecuteCalls: 1,
			wantUpdateCalls:  1,
			wantStatus:       job.StatusFailed,
			wantJobError:     "failed to execute job: executor failed",
		},
		{
			name:             "rejects completion of a non-running job",
			jobType:          "echo",
			jobStatus:        job.StatusQueued,
			resolverExists:   true,
			executeResult:    result,
			wantErr:          true,
			wantResolverCall: 1,
			wantExecuteCalls: 1,
			wantUpdateCalls:  0,
		},
		{
			name:             "rejects failure of a non-running job",
			jobType:          "unknown",
			jobStatus:        job.StatusQueued,
			resolverExists:   false,
			wantErr:          true,
			wantResolverCall: 1,
			wantExecuteCalls: 0,
			wantUpdateCalls:  0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			initialUpdatedAt := time.Now().UTC().Add(-time.Minute)
			payload := json.RawMessage(`{"message":"hello"}`)
			claimedJob := job.Job{
				ID:          uuid.New(),
				Type:        tc.jobType,
				Payload:     payload,
				Status:      tc.jobStatus,
				Attempt:     1,
				MaxAttempts: 1,
				RunAfter:    initialUpdatedAt.Add(-time.Minute),
				LockedBy:    "worker-1",
				LeaseUntil:  initialUpdatedAt.Add(time.Minute),
				CreatedAt:   initialUpdatedAt.Add(-time.Minute),
				UpdatedAt:   initialUpdatedAt,
			}

			repository := &fakeWorkerJobRepository{
				claimResult: claimedJob,
				claimErr:    tc.claimErr,
				updateErr:   tc.updateErr,
			}
			executor := &fakeExecutor{
				result: tc.executeResult,
				err:    tc.executeErr,
			}
			resolver := &fakeExecutorResolver{
				executor: executor,
				exists:   tc.resolverExists,
			}
			processor := NewJobProcessor(repository, resolver)

			ctx := context.Background()
			workerID := "worker-1"
			leaseDuration := 30 * time.Second

			err := processor.ProcessNextJob(ctx, workerID, leaseDuration)

			if (err != nil) != tc.wantErr {
				t.Fatalf("ProcessNextJob() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErrorIs != nil && !errors.Is(err, tc.wantErrorIs) {
				t.Fatalf("ProcessNextJob() error = %v, want errors.Is(_, %v)", err, tc.wantErrorIs)
			}

			if repository.claimCalls != 1 {
				t.Errorf("ClaimJob() calls = %d, want 1", repository.claimCalls)
			}
			if repository.claimCtx != ctx {
				t.Error("ProcessNextJob() did not pass context to ClaimJob")
			}
			if repository.claimedBy != workerID {
				t.Errorf("ClaimJob() workerID = %q, want %q", repository.claimedBy, workerID)
			}
			if repository.claimLeaseDuration != leaseDuration {
				t.Errorf("ClaimJob() lease duration = %s, want %s", repository.claimLeaseDuration, leaseDuration)
			}
			if repository.claimTime.IsZero() {
				t.Error("ClaimJob() time is zero")
			}
			if repository.claimTime.Location() != time.UTC {
				t.Errorf("ClaimJob() time location = %v, want UTC", repository.claimTime.Location())
			}

			if resolver.getCalls != tc.wantResolverCall {
				t.Errorf("resolver Get() calls = %d, want %d", resolver.getCalls, tc.wantResolverCall)
			}
			if tc.wantResolverCall > 0 && resolver.requestedName != tc.jobType {
				t.Errorf("resolver Get() name = %q, want %q", resolver.requestedName, tc.jobType)
			}

			if executor.executeCalls != tc.wantExecuteCalls {
				t.Errorf("Executor.Execute() calls = %d, want %d", executor.executeCalls, tc.wantExecuteCalls)
			}
			if tc.wantExecuteCalls > 0 {
				if executor.executeCtx != ctx {
					t.Error("ProcessNextJob() did not pass context to executor")
				}
				if !bytes.Equal(executor.payload, payload) {
					t.Errorf("Executor.Execute() payload = %s, want %s", executor.payload, payload)
				}
			}

			if repository.updateCalls != tc.wantUpdateCalls {
				t.Fatalf("UpdateJob() calls = %d, want %d", repository.updateCalls, tc.wantUpdateCalls)
			}
			if tc.wantUpdateCalls == 0 {
				return
			}

			if repository.updateCtx != ctx {
				t.Error("ProcessNextJob() did not pass context to UpdateJob")
			}
			if repository.updatedJob == nil {
				t.Fatal("UpdateJob() received nil job")
			}
			if repository.updatedJob.ID != claimedJob.ID {
				t.Errorf("UpdateJob() job ID = %s, want %s", repository.updatedJob.ID, claimedJob.ID)
			}
			if repository.updatedJob.Status != tc.wantStatus {
				t.Errorf("UpdateJob() status = %q, want %q", repository.updatedJob.Status, tc.wantStatus)
			}
			if !bytes.Equal(repository.updatedJob.Result, tc.wantResult) {
				t.Errorf("UpdateJob() result = %s, want %s", repository.updatedJob.Result, tc.wantResult)
			}
			if repository.updatedJob.Error != tc.wantJobError {
				t.Errorf("UpdateJob() error = %q, want %q", repository.updatedJob.Error, tc.wantJobError)
			}
			if repository.updatedJob.LockedBy != "" {
				t.Errorf("UpdateJob() LockedBy = %q, want empty", repository.updatedJob.LockedBy)
			}
			if !repository.updatedJob.LeaseUntil.IsZero() {
				t.Errorf("UpdateJob() LeaseUntil = %s, want zero", repository.updatedJob.LeaseUntil)
			}
			if !repository.updatedJob.UpdatedAt.After(initialUpdatedAt) {
				t.Errorf("UpdateJob() UpdatedAt = %s, want after %s", repository.updatedJob.UpdatedAt, initialUpdatedAt)
			}
		})
	}
}
