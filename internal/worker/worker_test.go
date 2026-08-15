package worker

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/LeezyWannaFall/GoreFlow/internal/application"
)

type processorStep struct {
	err error
}

type processorCall struct {
	ctx           context.Context
	workerID      string
	leaseDuration time.Duration
}

type fakeProcessor struct {
	steps  []processorStep
	calls  []processorCall
	onCall func(callNumber int)
}

func (f *fakeProcessor) ProcessNextJob(
	ctx context.Context,
	workerID string,
	leaseDuration time.Duration,
) error {
	f.calls = append(f.calls, processorCall{
		ctx:           ctx,
		workerID:      workerID,
		leaseDuration: leaseDuration,
	})

	callNumber := len(f.calls)
	if f.onCall != nil {
		f.onCall(callNumber)
	}

	if callNumber > len(f.steps) {
		return fmt.Errorf("unexpected ProcessNextJob call number %d", callNumber)
	}

	return f.steps[callNumber-1].err
}

func TestWorker_Run(t *testing.T) {
	processErr := errors.New("processor unavailable")
	wrappedNoJobError := fmt.Errorf("claim job: %w", application.ErrNoJobAvailable)

	testCases := []struct {
		name         string
		steps        []processorStep
		pollInterval time.Duration
		cancelOnCall int
		wantErrIs    error
		wantCalls    int
		minRunTime   time.Duration
		maxRunTime   time.Duration
	}{
		{
			name:         "returns processor error",
			steps:        []processorStep{{err: processErr}},
			pollInterval: time.Hour,
			wantErrIs:    processErr,
			wantCalls:    1,
		},
		{
			name: "processes next job immediately after success",
			steps: []processorStep{
				{err: nil},
				{err: processErr},
			},
			pollInterval: 200 * time.Millisecond,
			wantErrIs:    processErr,
			wantCalls:    2,
			maxRunTime:   100 * time.Millisecond,
		},
		{
			name: "retries after poll interval when queue is empty",
			steps: []processorStep{
				{err: wrappedNoJobError},
				{err: processErr},
			},
			pollInterval: 20 * time.Millisecond,
			wantErrIs:    processErr,
			wantCalls:    2,
			minRunTime:   20 * time.Millisecond,
		},
		{
			name:         "stops when context is canceled while idle",
			steps:        []processorStep{{err: wrappedNoJobError}},
			pollInterval: 200 * time.Millisecond,
			cancelOnCall: 1,
			wantErrIs:    context.Canceled,
			wantCalls:    1,
			maxRunTime:   100 * time.Millisecond,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			processor := &fakeProcessor{steps: tc.steps}
			if tc.cancelOnCall > 0 {
				processor.onCall = func(callNumber int) {
					if callNumber == tc.cancelOnCall {
						cancel()
					}
				}
			}

			workerID := "worker-1"
			leaseDuration := 30 * time.Second
			worker := NewWorker(workerID, processor, tc.pollInterval, leaseDuration)

			startedAt := time.Now()
			err := worker.Run(ctx)
			runTime := time.Since(startedAt)

			if !errors.Is(err, tc.wantErrIs) {
				t.Fatalf("Run() error = %v, want errors.Is(_, %v)", err, tc.wantErrIs)
			}
			if len(processor.calls) != tc.wantCalls {
				t.Fatalf("ProcessNextJob() calls = %d, want %d", len(processor.calls), tc.wantCalls)
			}
			if tc.minRunTime > 0 && runTime < tc.minRunTime {
				t.Errorf("Run() duration = %s, want at least %s", runTime, tc.minRunTime)
			}
			if tc.maxRunTime > 0 && runTime >= tc.maxRunTime {
				t.Errorf("Run() duration = %s, want less than %s", runTime, tc.maxRunTime)
			}

			for callNumber, call := range processor.calls {
				if call.ctx != ctx {
					t.Errorf("ProcessNextJob() call %d received a different context", callNumber+1)
				}
				if call.workerID != workerID {
					t.Errorf(
						"ProcessNextJob() call %d workerID = %q, want %q",
						callNumber+1,
						call.workerID,
						workerID,
					)
				}
				if call.leaseDuration != leaseDuration {
					t.Errorf(
						"ProcessNextJob() call %d lease duration = %s, want %s",
						callNumber+1,
						call.leaseDuration,
						leaseDuration,
					)
				}
			}
		})
	}
}
