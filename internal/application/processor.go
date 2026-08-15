package application

import (
	"context"
	"fmt"
	"time"
)

type JobProcessor struct {
	repo     WorkerJobRepository
	resolver ExecutorResolver
}

func NewJobProcessor(repo WorkerJobRepository, resolver ExecutorResolver) *JobProcessor {
	return &JobProcessor{repo: repo, resolver: resolver}
}

func (j *JobProcessor) ProcessNextJob(ctx context.Context, workerID string, leaseDuration time.Duration) error {
	claimTime := time.Now().UTC()

	claimedJob, err := j.repo.ClaimJob(ctx, workerID, claimTime, leaseDuration)
	if err != nil {
		return fmt.Errorf("claim job: %w", err)
	}

	if exec, ok := j.resolver.Get(claimedJob.Type); ok {
		result, executeErr := exec.Execute(ctx, claimedJob.Payload)
		endTime := time.Now().UTC()
		if executeErr != nil {
			reason := "failed to execute job: " + executeErr.Error()
			if err := j.failJob(ctx, &claimedJob, reason); err != nil {
				return fmt.Errorf("fail job after execution error: %w", err)
			}

			return nil
		}

		err = claimedJob.Complete(result, endTime)
		if err != nil {
			return fmt.Errorf("complete job: %w", err)
		}

		err = j.repo.UpdateJob(ctx, &claimedJob)
		if err != nil {
			return fmt.Errorf("update completed job: %w", err)
		}
	} else {
		reason := "executor not found for job type: " + claimedJob.Type
		if err := j.failJob(ctx, &claimedJob, reason); err != nil {
			return fmt.Errorf("fail job without executor: %w", err)
		}
	}

	return nil
}
