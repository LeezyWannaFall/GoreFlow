package worker

import (
	"context"
	"time"
)

type Processor interface {
	ProcessNextJob(ctx context.Context, workerID string, leaseDuration time.Duration) error
}
