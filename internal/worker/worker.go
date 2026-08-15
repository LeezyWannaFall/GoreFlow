package worker

import (
	"context"
	"errors"
	"time"

	"github.com/LeezyWannaFall/GoreFlow/internal/application"
)

type Worker struct {
	id            string
	processor     Processor
	pollInterval  time.Duration
	leaseDuration time.Duration
}

func NewWorker(id string, processor Processor, pollInterval, leaseDuration time.Duration) *Worker {
	return &Worker{
		id:            id,
		processor:     processor,
		pollInterval:  pollInterval,
		leaseDuration: leaseDuration,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	for {
		err := w.processor.ProcessNextJob(ctx, w.id, w.leaseDuration)
		if errors.Is(err, application.ErrNoJobAvailable) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(w.pollInterval):
			}
		} else if err != nil {
			return err
		}
	}
}
