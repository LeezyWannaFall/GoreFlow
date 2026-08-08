package job

import (
	"github.com/google/uuid"
	"time"
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
    Payload     []byte
    Status      Status
    Attempt     int
    MaxAttempts int
    RunAfter 	time.Time
	LockedBy	string
	LeaseUntil	time.Time
	Result		int
	Error 		error
	CreatedAt	time.Time
	UpdatedAt	time.Time
}

func (j *Job) Start(...) error {
    // Проверка перехода queued → running
}

func (j *Job) Complete(...) error {
    // running → succeeded
}

func (j *Job) Fail(...) error {
    // running → retrying/failed
}