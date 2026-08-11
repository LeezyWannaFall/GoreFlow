package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/LeezyWannaFall/GoreFlow/internal/job"
	"github.com/google/uuid"
)

type fakeJobRepository struct {
	createCalls int
	createCtx   context.Context
	createdJob  *job.Job
	createErr   error

	getCalls  int
	getCtx    context.Context
	requested uuid.UUID
	getResult job.Job
	getErr    error
}

func (f *fakeJobRepository) CreateJob(ctx context.Context, model *job.Job) error {
	f.createCalls++
	f.createCtx = ctx
	f.createdJob = model
	return f.createErr
}

func (f *fakeJobRepository) GetJobByID(ctx context.Context, id uuid.UUID) (job.Job, error) {
	f.getCalls++
	f.getCtx = ctx
	f.requested = id
	return f.getResult, f.getErr
}

func TestApplication_CreateJob(t *testing.T) {
	repositoryErr := errors.New("repository unavailable")
	validPayload := json.RawMessage(`{"message":"hello"}`)

	tests := []struct {
		name            string
		jobType         string
		payload         json.RawMessage
		repositoryErr   error
		wantErr         bool
		wantErrorIs     error
		wantCreateCalls int
	}{
		{
			name:            "creates and saves valid job",
			jobType:         "echo",
			payload:         validPayload,
			wantCreateCalls: 1,
		},
		{
			name:            "rejects empty job type before repository call",
			jobType:         "   ",
			payload:         validPayload,
			wantErr:         true,
			wantCreateCalls: 0,
		},
		{
			name:            "rejects invalid JSON before repository call",
			jobType:         "echo",
			payload:         json.RawMessage(`{"message":`),
			wantErr:         true,
			wantCreateCalls: 0,
		},
		{
			name:            "preserves repository error",
			jobType:         "echo",
			payload:         validPayload,
			repositoryErr:   repositoryErr,
			wantErr:         true,
			wantErrorIs:     repositoryErr,
			wantCreateCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeJobRepository{createErr: tt.repositoryErr}
			app := NewApplication(repo)
			ctx := context.Background()

			got, err := app.CreateJob(ctx, tt.jobType, tt.payload)

			if (err != nil) != tt.wantErr {
				t.Fatalf("CreateJob() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrorIs != nil && !errors.Is(err, tt.wantErrorIs) {
				t.Fatalf("CreateJob() error = %v, want errors.Is(_, %v)", err, tt.wantErrorIs)
			}
			if repo.createCalls != tt.wantCreateCalls {
				t.Fatalf("repository CreateJob() calls = %d, want %d", repo.createCalls, tt.wantCreateCalls)
			}

			if tt.wantErr {
				if got.ID != uuid.Nil {
					t.Errorf("CreateJob() returned job ID %s on error, want zero job", got.ID)
				}
				return
			}

			if repo.createCtx != ctx {
				t.Error("CreateJob() did not pass context to repository")
			}
			if repo.createdJob == nil {
				t.Fatal("CreateJob() passed nil job to repository")
			}
			if got.ID == uuid.Nil {
				t.Error("CreateJob() returned job with nil ID")
			}
			if repo.createdJob.ID != got.ID {
				t.Errorf("saved job ID = %s, returned job ID = %s", repo.createdJob.ID, got.ID)
			}
			if got.Type != tt.jobType {
				t.Errorf("CreateJob() type = %q, want %q", got.Type, tt.jobType)
			}
			if !bytes.Equal(got.Payload, tt.payload) {
				t.Errorf("CreateJob() payload = %s, want %s", got.Payload, tt.payload)
			}
			if got.Status != job.StatusQueued {
				t.Errorf("CreateJob() status = %q, want %q", got.Status, job.StatusQueued)
			}
		})
	}
}

func TestApplication_GetJobByID(t *testing.T) {
	repositoryErr := errors.New("repository unavailable")
	jobID := uuid.New()
	existingJob := job.Job{
		ID:      jobID,
		Type:    "echo",
		Payload: json.RawMessage(`{"message":"hello"}`),
		Status:  job.StatusQueued,
	}

	tests := []struct {
		name          string
		id            uuid.UUID
		repositoryJob job.Job
		repositoryErr error
		wantJob       job.Job
		wantErr       bool
		wantErrorIs   error
		wantGetCalls  int
	}{
		{
			name:          "returns existing job",
			id:            jobID,
			repositoryJob: existingJob,
			wantJob:       existingJob,
			wantGetCalls:  1,
		},
		{
			name:         "rejects nil ID before repository call",
			id:           uuid.Nil,
			wantErr:      true,
			wantGetCalls: 0,
		},
		{
			name:          "preserves job not found error",
			id:            uuid.New(),
			repositoryErr: ErrJobNotFound,
			wantErr:       true,
			wantErrorIs:   ErrJobNotFound,
			wantGetCalls:  1,
		},
		{
			name:          "preserves repository error",
			id:            uuid.New(),
			repositoryErr: repositoryErr,
			wantErr:       true,
			wantErrorIs:   repositoryErr,
			wantGetCalls:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeJobRepository{
				getResult: tt.repositoryJob,
				getErr:    tt.repositoryErr,
			}
			app := NewApplication(repo)
			ctx := context.Background()

			got, err := app.GetJobByID(ctx, tt.id)

			if (err != nil) != tt.wantErr {
				t.Fatalf("GetJobByID() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrorIs != nil && !errors.Is(err, tt.wantErrorIs) {
				t.Fatalf("GetJobByID() error = %v, want errors.Is(_, %v)", err, tt.wantErrorIs)
			}
			if repo.getCalls != tt.wantGetCalls {
				t.Fatalf("repository GetJobByID() calls = %d, want %d", repo.getCalls, tt.wantGetCalls)
			}

			if tt.wantErr {
				if got.ID != uuid.Nil {
					t.Errorf("GetJobByID() returned job ID %s on error, want zero job", got.ID)
				}
				return
			}

			if repo.getCtx != ctx {
				t.Error("GetJobByID() did not pass context to repository")
			}
			if repo.requested != tt.id {
				t.Errorf("repository received ID %s, want %s", repo.requested, tt.id)
			}
			if !reflect.DeepEqual(got, tt.wantJob) {
				t.Errorf("GetJobByID() job = %#v, want %#v", got, tt.wantJob)
			}
		})
	}
}
