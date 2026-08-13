package httptransport

import (
	"context"
	"encoding/json"

	"github.com/LeezyWannaFall/GoreFlow/internal/job"
	"github.com/google/uuid"
)

type JobService interface {
	CreateJob(ctx context.Context, jobType string, payload json.RawMessage) (job.Job, error)
	GetJobByID(ctx context.Context, id uuid.UUID) (job.Job, error)
}
