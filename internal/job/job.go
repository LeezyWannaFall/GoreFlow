package job

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidJobType = errors.New("job type must not be empty")
	ErrInvalidPayload = errors.New("invalid JSON payload")
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

type Job struct {
	ID          uuid.UUID
	Type        string
	Payload     json.RawMessage
	Status      Status
	Attempt     int
	MaxAttempts int
	RunAfter    time.Time
	LockedBy    string
	LeaseUntil  time.Time
	Result      json.RawMessage
	Error       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func NewJob(jobType string, payload json.RawMessage) (*Job, error) {
	if strings.TrimSpace(jobType) == "" {
		return nil, ErrInvalidJobType
	}

	if !json.Valid(payload) {
		return nil, ErrInvalidPayload
	}

	now := time.Now().UTC()

	return &Job{
		ID:          uuid.New(),
		Type:        jobType,
		Payload:     payload,
		Status:      StatusQueued,
		Attempt:     0,
		MaxAttempts: 1,
		RunAfter:    now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// queued -> running
func (j *Job) Start(workerID string, now time.Time, leaseDuration time.Duration) error {
	if j.Status != StatusQueued {
		return errors.New("only queued job can be started")
	}

	if strings.TrimSpace(workerID) == "" {
		return errors.New("worker ID must not be empty")
	}

	if leaseDuration <= 0 {
		return errors.New("lease duration must be positive")
	}

	if now.Before(j.RunAfter) {
		return errors.New("job is not ready to run")
	}

	if j.Attempt >= j.MaxAttempts {
		return errors.New("maximum attempts reached")
	}

	now = now.UTC()

	j.Status = StatusRunning
	j.Attempt++
	j.LockedBy = workerID
	j.LeaseUntil = now.Add(leaseDuration)
	j.UpdatedAt = now

	return nil
}

// running -> succeeded
func (j *Job) Complete(result json.RawMessage, endTime time.Time) error {
	if j.Status != StatusRunning {
		return errors.New("job is not running now")
	}

	endTime = endTime.UTC()

	if endTime.Before(j.UpdatedAt) {
		return errors.New("end time can't be earlier than job update")
	}

	j.Status = StatusSucceeded
	j.LeaseUntil = time.Time{}
	j.LockedBy = ""
	j.Result = result
	j.Error = ""
	j.UpdatedAt = endTime

	return nil
}

// running -> failed
func (j *Job) Fail(errorText string, endTime time.Time) error {
	if j.Status != StatusRunning {
		return errors.New("job is not running now")
	}

	endTime = endTime.UTC()

	if endTime.Before(j.UpdatedAt) {
		return errors.New("end time can't be earlier than job update")
	}

	if strings.TrimSpace(errorText) == "" {
		return errors.New("error doesnt have text")
	}

	j.Status = StatusFailed
	j.LeaseUntil = time.Time{}
	j.LockedBy = ""
	j.Result = nil
	j.Error = errorText
	j.UpdatedAt = endTime

	return nil
}
