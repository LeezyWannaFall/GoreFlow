package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/LeezyWannaFall/GoreFlow/internal/job"
	"github.com/google/uuid"
)

type Application struct {
	repo JobRepository
}

func NewApplication(repo JobRepository) *Application {
	return &Application{repo: repo}
}

func (a *Application) CreateJob(ctx context.Context, jobType string, payload json.RawMessage) (job.Job, error) {
	newJob, err := job.NewJob(jobType, payload)
	if err != nil {
		return job.Job{}, fmt.Errorf("failed to create new job: %w", err)
	}

	err = a.repo.CreateJob(ctx, newJob)
	if err != nil {
		return job.Job{}, fmt.Errorf("save job: %w", err)
	}

	return *newJob, nil
}

func (a *Application) GetJobByID(ctx context.Context, id uuid.UUID) (job.Job, error) {
	if id == uuid.Nil {
		return job.Job{}, errors.New("job id cannot be empty or nil")
	}

	collectedJob, err := a.repo.GetJobByID(ctx, id)
	if err != nil {
		return job.Job{}, fmt.Errorf("get job by ID: %w", err)
	}

	return collectedJob, nil
}
