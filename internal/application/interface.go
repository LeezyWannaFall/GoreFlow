package application

import (
	"context"

	"github.com/LeezyWannaFall/GoreFlow/internal/job"
	"github.com/google/uuid"
)

type JobRepository interface {
	CreateJob(ctx context.Context, model *job.Job) error
	GetJobByID(ctx context.Context, id uuid.UUID) (job.Job, error)
}
