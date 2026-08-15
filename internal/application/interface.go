package application

import (
	"context"
	"time"

	"github.com/LeezyWannaFall/GoreFlow/internal/executor"
	"github.com/LeezyWannaFall/GoreFlow/internal/job"
	"github.com/google/uuid"
)

type JobRepository interface {
	CreateJob(ctx context.Context, model *job.Job) error
	GetJobByID(ctx context.Context, id uuid.UUID) (job.Job, error)
}

type WorkerJobRepository interface {
	ClaimJob(ctx context.Context, workerID string, now time.Time, leaseDuration time.Duration) (job.Job, error)
	UpdateJob(ctx context.Context, model *job.Job) error
}

type ExecutorResolver interface {
	Get(name string) (executor.Executor, bool)
}
