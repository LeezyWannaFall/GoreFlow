package application

import (
	"context"
	"fmt"
	"time"

	"github.com/LeezyWannaFall/GoreFlow/internal/job"
)

func (j *JobProcessor) failJob(ctx context.Context, claimedJob *job.Job, reason string) error {
	err := claimedJob.Fail(reason, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("mark job as failed: %w", err)
	}

	err = j.repo.UpdateJob(ctx, claimedJob)
	if err != nil {
		return fmt.Errorf("update failed job: %w", err)
	}

	return nil
}
