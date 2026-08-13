package httptransport

import (
	"encoding/json"
	"time"

	"github.com/LeezyWannaFall/GoreFlow/internal/job"
	"github.com/google/uuid"
)

type CreateJobRequest struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type JobResponse struct {
	ID          uuid.UUID       `json:"id"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
	Status      job.Status      `json:"status"`
	Attempt     int             `json:"attempt"`
	MaxAttempts int             `json:"max_attempts"`
	RunAfter    time.Time       `json:"run_after"`
	LockedBy    *string         `json:"locked_by"`
	LeaseUntil  *time.Time      `json:"lease_until"`
	Result      json.RawMessage `json:"result"`
	Error       *string         `json:"error"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func newJobResponse(model job.Job) JobResponse {
	response := JobResponse{
		ID:          model.ID,
		Type:        model.Type,
		Payload:     model.Payload,
		Status:      model.Status,
		Attempt:     model.Attempt,
		MaxAttempts: model.MaxAttempts,
		RunAfter:    model.RunAfter,
		Result:      model.Result,
		CreatedAt:   model.CreatedAt,
		UpdatedAt:   model.UpdatedAt,
	}

	if model.LockedBy != "" {
		lockedBy := model.LockedBy
		response.LockedBy = &lockedBy
	}
	if !model.LeaseUntil.IsZero() {
		leaseUntil := model.LeaseUntil
		response.LeaseUntil = &leaseUntil
	}
	if model.Error != "" {
		errorText := model.Error
		response.Error = &errorText
	}

	return response
}
