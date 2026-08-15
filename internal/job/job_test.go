package job

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewJob(t *testing.T) {
	validPayload := json.RawMessage(`{"message":"hello"}`)

	tests := []struct {
		name        string
		jobType     string
		payload     json.RawMessage
		wantErrIs   error
		wantJobType string
	}{
		{
			name:        "creates job from valid data",
			jobType:     "echo",
			payload:     validPayload,
			wantJobType: "echo",
		},
		{
			name:      "rejects empty job type",
			jobType:   "",
			payload:   validPayload,
			wantErrIs: ErrInvalidJobType,
		},
		{
			name:      "rejects whitespace-only job type",
			jobType:   "   ",
			payload:   validPayload,
			wantErrIs: ErrInvalidJobType,
		},
		{
			name:      "rejects malformed JSON payload",
			jobType:   "echo",
			payload:   json.RawMessage(`{"message":`),
			wantErrIs: ErrInvalidPayload,
		},
		{
			name:      "rejects nil payload",
			jobType:   "echo",
			payload:   nil,
			wantErrIs: ErrInvalidPayload,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now().UTC()
			got, err := NewJob(tt.jobType, tt.payload)
			after := time.Now().UTC()

			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("NewJob() error = %v, want errors.Is(_, %v)", err, tt.wantErrIs)
				}
				if got != nil {
					t.Fatalf("NewJob() job = %#v, want nil", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("NewJob() unexpected error = %v", err)
			}
			if got == nil {
				t.Fatal("NewJob() job = nil, want job")
			}
			if got.ID == uuid.Nil {
				t.Error("NewJob() ID is nil UUID")
			}
			if got.Type != tt.wantJobType {
				t.Errorf("NewJob() Type = %q, want %q", got.Type, tt.wantJobType)
			}
			if !bytes.Equal(got.Payload, tt.payload) {
				t.Errorf("NewJob() Payload = %s, want %s", got.Payload, tt.payload)
			}
			if got.Status != StatusQueued {
				t.Errorf("NewJob() Status = %q, want %q", got.Status, StatusQueued)
			}
			if got.Attempt != 0 {
				t.Errorf("NewJob() Attempt = %d, want 0", got.Attempt)
			}
			if got.MaxAttempts != 1 {
				t.Errorf("NewJob() MaxAttempts = %d, want 1", got.MaxAttempts)
			}
			if got.RunAfter != got.CreatedAt || got.CreatedAt != got.UpdatedAt {
				t.Errorf(
					"NewJob() timestamps differ: RunAfter=%s CreatedAt=%s UpdatedAt=%s",
					got.RunAfter,
					got.CreatedAt,
					got.UpdatedAt,
				)
			}
			if got.CreatedAt.Before(before) || got.CreatedAt.After(after) {
				t.Errorf("NewJob() CreatedAt = %s, want between %s and %s", got.CreatedAt, before, after)
			}
			if got.CreatedAt.Location() != time.UTC {
				t.Errorf("NewJob() CreatedAt location = %v, want UTC", got.CreatedAt.Location())
			}
			if got.LockedBy != "" || !got.LeaseUntil.IsZero() {
				t.Errorf("NewJob() ownership = (%q, %s), want empty", got.LockedBy, got.LeaseUntil)
			}
			if got.Result != nil || got.Error != "" {
				t.Errorf("NewJob() outcome = (%s, %q), want empty", got.Result, got.Error)
			}
		})
	}
}

func TestJob_Start(t *testing.T) {
	readyAt := time.Date(2026, time.August, 15, 10, 0, 0, 0, time.UTC)
	nonUTCLocation := time.FixedZone("test-zone", 3*60*60)
	startTime := readyAt.Add(time.Minute).In(nonUTCLocation)
	leaseDuration := 30 * time.Second

	tests := []struct {
		name          string
		mutate        func(*Job)
		workerID      string
		now           time.Time
		leaseDuration time.Duration
		wantErr       bool
	}{
		{
			name:          "starts ready queued job",
			workerID:      "worker-1",
			now:           startTime,
			leaseDuration: leaseDuration,
		},
		{
			name:          "starts exactly at run after",
			workerID:      "worker-1",
			now:           readyAt,
			leaseDuration: leaseDuration,
		},
		{
			name: "rejects non-queued job",
			mutate: func(model *Job) {
				model.Status = StatusRunning
			},
			workerID:      "worker-1",
			now:           startTime,
			leaseDuration: leaseDuration,
			wantErr:       true,
		},
		{
			name:          "rejects empty worker ID",
			workerID:      "",
			now:           startTime,
			leaseDuration: leaseDuration,
			wantErr:       true,
		},
		{
			name:          "rejects whitespace-only worker ID",
			workerID:      "   ",
			now:           startTime,
			leaseDuration: leaseDuration,
			wantErr:       true,
		},
		{
			name:          "rejects zero lease duration",
			workerID:      "worker-1",
			now:           startTime,
			leaseDuration: 0,
			wantErr:       true,
		},
		{
			name:          "rejects negative lease duration",
			workerID:      "worker-1",
			now:           startTime,
			leaseDuration: -time.Second,
			wantErr:       true,
		},
		{
			name:          "rejects job before run after",
			workerID:      "worker-1",
			now:           readyAt.Add(-time.Nanosecond),
			leaseDuration: leaseDuration,
			wantErr:       true,
		},
		{
			name: "rejects exhausted attempts",
			mutate: func(model *Job) {
				model.Attempt = model.MaxAttempts
			},
			workerID:      "worker-1",
			now:           startTime,
			leaseDuration: leaseDuration,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := newQueuedJobForTest(readyAt)
			if tt.mutate != nil {
				tt.mutate(&model)
			}
			before := cloneJob(model)

			err := model.Start(tt.workerID, tt.now, tt.leaseDuration)

			if tt.wantErr {
				if err == nil {
					t.Fatal("Start() error = nil, want error")
				}
				assertJobUnchanged(t, model, before)
				return
			}

			if err != nil {
				t.Fatalf("Start() unexpected error = %v", err)
			}
			wantNow := tt.now.UTC()
			if model.Status != StatusRunning {
				t.Errorf("Start() Status = %q, want %q", model.Status, StatusRunning)
			}
			if model.Attempt != before.Attempt+1 {
				t.Errorf("Start() Attempt = %d, want %d", model.Attempt, before.Attempt+1)
			}
			if model.LockedBy != tt.workerID {
				t.Errorf("Start() LockedBy = %q, want %q", model.LockedBy, tt.workerID)
			}
			if !model.LeaseUntil.Equal(wantNow.Add(tt.leaseDuration)) {
				t.Errorf("Start() LeaseUntil = %s, want %s", model.LeaseUntil, wantNow.Add(tt.leaseDuration))
			}
			if !model.UpdatedAt.Equal(wantNow) {
				t.Errorf("Start() UpdatedAt = %s, want %s", model.UpdatedAt, wantNow)
			}
			if model.UpdatedAt.Location() != time.UTC || model.LeaseUntil.Location() != time.UTC {
				t.Errorf(
					"Start() timestamp locations = (%v, %v), want UTC",
					model.UpdatedAt.Location(),
					model.LeaseUntil.Location(),
				)
			}
			if model.Type != before.Type || !bytes.Equal(model.Payload, before.Payload) || model.CreatedAt != before.CreatedAt {
				t.Error("Start() changed immutable job data")
			}
		})
	}
}

func TestJob_Complete(t *testing.T) {
	startedAt := time.Date(2026, time.August, 15, 10, 0, 0, 0, time.UTC)
	endTime := startedAt.Add(time.Minute)
	result := json.RawMessage(`{"message":"done"}`)

	tests := []struct {
		name    string
		mutate  func(*Job)
		result  json.RawMessage
		endTime time.Time
		wantErr bool
	}{
		{
			name:    "completes running job",
			result:  result,
			endTime: endTime,
		},
		{
			name:    "completes exactly at last update time",
			result:  result,
			endTime: startedAt,
		},
		{
			name: "rejects non-running job",
			mutate: func(model *Job) {
				model.Status = StatusQueued
			},
			result:  result,
			endTime: endTime,
			wantErr: true,
		},
		{
			name:    "rejects end time before last update",
			result:  result,
			endTime: startedAt.Add(-time.Nanosecond),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := newRunningJobForTest(startedAt)
			model.Error = "old error"
			if tt.mutate != nil {
				tt.mutate(&model)
			}
			before := cloneJob(model)

			err := model.Complete(tt.result, tt.endTime)

			if tt.wantErr {
				if err == nil {
					t.Fatal("Complete() error = nil, want error")
				}
				assertJobUnchanged(t, model, before)
				return
			}

			if err != nil {
				t.Fatalf("Complete() unexpected error = %v", err)
			}
			if model.Status != StatusSucceeded {
				t.Errorf("Complete() Status = %q, want %q", model.Status, StatusSucceeded)
			}
			if !bytes.Equal(model.Result, tt.result) {
				t.Errorf("Complete() Result = %s, want %s", model.Result, tt.result)
			}
			if model.Error != "" {
				t.Errorf("Complete() Error = %q, want empty", model.Error)
			}
			assertFinishedJob(t, model, tt.endTime.UTC())
			assertExecutionInputUnchanged(t, model, before)
		})
	}
}

func TestJob_Fail(t *testing.T) {
	startedAt := time.Date(2026, time.August, 15, 10, 0, 0, 0, time.UTC)
	endTime := startedAt.Add(time.Minute)
	errorText := "executor unavailable"

	tests := []struct {
		name      string
		mutate    func(*Job)
		errorText string
		endTime   time.Time
		wantErr   bool
	}{
		{
			name:      "fails running job",
			errorText: errorText,
			endTime:   endTime,
		},
		{
			name:      "fails exactly at last update time",
			errorText: errorText,
			endTime:   startedAt,
		},
		{
			name: "rejects non-running job",
			mutate: func(model *Job) {
				model.Status = StatusQueued
			},
			errorText: errorText,
			endTime:   endTime,
			wantErr:   true,
		},
		{
			name:      "rejects empty error text",
			errorText: "",
			endTime:   endTime,
			wantErr:   true,
		},
		{
			name:      "rejects whitespace-only error text",
			errorText: "   ",
			endTime:   endTime,
			wantErr:   true,
		},
		{
			name:      "rejects end time before last update",
			errorText: errorText,
			endTime:   startedAt.Add(-time.Nanosecond),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := newRunningJobForTest(startedAt)
			model.Result = json.RawMessage(`{"old":"result"}`)
			if tt.mutate != nil {
				tt.mutate(&model)
			}
			before := cloneJob(model)

			err := model.Fail(tt.errorText, tt.endTime)

			if tt.wantErr {
				if err == nil {
					t.Fatal("Fail() error = nil, want error")
				}
				assertJobUnchanged(t, model, before)
				return
			}

			if err != nil {
				t.Fatalf("Fail() unexpected error = %v", err)
			}
			if model.Status != StatusFailed {
				t.Errorf("Fail() Status = %q, want %q", model.Status, StatusFailed)
			}
			if model.Result != nil {
				t.Errorf("Fail() Result = %s, want nil", model.Result)
			}
			if model.Error != tt.errorText {
				t.Errorf("Fail() Error = %q, want %q", model.Error, tt.errorText)
			}
			assertFinishedJob(t, model, tt.endTime.UTC())
			assertExecutionInputUnchanged(t, model, before)
		})
	}
}

func newQueuedJobForTest(now time.Time) Job {
	return Job{
		ID:          uuid.MustParse("a61a9ae1-cfe1-4667-886f-4f32b804ef2f"),
		Type:        "echo",
		Payload:     json.RawMessage(`{"message":"hello"}`),
		Status:      StatusQueued,
		Attempt:     0,
		MaxAttempts: 1,
		RunAfter:    now,
		CreatedAt:   now.Add(-time.Minute),
		UpdatedAt:   now.Add(-time.Minute),
	}
}

func newRunningJobForTest(startedAt time.Time) Job {
	model := newQueuedJobForTest(startedAt.Add(-time.Minute))
	model.Status = StatusRunning
	model.Attempt = 1
	model.LockedBy = "worker-1"
	model.LeaseUntil = startedAt.Add(30 * time.Second)
	model.UpdatedAt = startedAt
	return model
}

func cloneJob(model Job) Job {
	model.Payload = bytes.Clone(model.Payload)
	model.Result = bytes.Clone(model.Result)
	return model
}

func assertJobUnchanged(t *testing.T, got, want Job) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("job changed after rejected transition:\n got: %#v\nwant: %#v", got, want)
	}
}

func assertFinishedJob(t *testing.T, model Job, wantUpdatedAt time.Time) {
	t.Helper()
	if model.LockedBy != "" {
		t.Errorf("finished job LockedBy = %q, want empty", model.LockedBy)
	}
	if !model.LeaseUntil.IsZero() {
		t.Errorf("finished job LeaseUntil = %s, want zero", model.LeaseUntil)
	}
	if !model.UpdatedAt.Equal(wantUpdatedAt) {
		t.Errorf("finished job UpdatedAt = %s, want %s", model.UpdatedAt, wantUpdatedAt)
	}
}

func assertExecutionInputUnchanged(t *testing.T, got, before Job) {
	t.Helper()
	if got.ID != before.ID ||
		got.Type != before.Type ||
		!bytes.Equal(got.Payload, before.Payload) ||
		got.Attempt != before.Attempt ||
		got.MaxAttempts != before.MaxAttempts ||
		got.RunAfter != before.RunAfter ||
		got.CreatedAt != before.CreatedAt {
		t.Error("terminal transition changed execution input or immutable job data")
	}
}
