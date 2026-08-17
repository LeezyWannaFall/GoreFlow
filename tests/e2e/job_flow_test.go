//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

const pollInterval = 200 * time.Millisecond

type CreateJobRequest struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type JobResponse struct {
	ID          uuid.UUID       `json:"id"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
	Status      string          `json:"status"`
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

func TestEchoJobFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	const baseURL = "http://localhost:8080"
	expectedPayload := json.RawMessage(`{"message":"hello from GoreFlow e2e"}`)

	if err := waitForHealth(ctx, client, baseURL); err != nil {
		t.Fatalf("wait for API health: %v", err)
	}

	createdJob, err := createJob(ctx, client, baseURL, "echo", expectedPayload)
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	if createdJob.ID == uuid.Nil {
		t.Fatal("created job has an empty ID")
	}
	if createdJob.Type != "echo" {
		t.Fatalf("created job type: expected echo, got %q", createdJob.Type)
	}
	if createdJob.Status != "queued" {
		t.Fatalf("created job status: expected queued, got %q", createdJob.Status)
	}
	if createdJob.Attempt != 0 {
		t.Fatalf("created job attempt: expected 0, got %d", createdJob.Attempt)
	}
	if !jsonEqual(createdJob.Payload, expectedPayload) {
		t.Fatalf("created job payload: expected %s, got %s", expectedPayload, createdJob.Payload)
	}

	completedJob, err := waitForTerminalJob(ctx, client, baseURL, createdJob.ID)
	if err != nil {
		t.Fatalf("wait for terminal job: %v", err)
	}

	if completedJob.ID != createdJob.ID {
		t.Fatalf("completed job ID: expected %s, got %s", createdJob.ID, completedJob.ID)
	}
	if completedJob.Status != "succeeded" {
		t.Fatalf("completed job status: expected succeeded, got %q; error: %v", completedJob.Status, completedJob.Error)
	}
	if completedJob.Attempt != 1 {
		t.Fatalf("completed job attempt: expected 1, got %d", completedJob.Attempt)
	}
	if !jsonEqual(completedJob.Result, expectedPayload) {
		t.Fatalf("completed job result: expected %s, got %s", expectedPayload, completedJob.Result)
	}
	if completedJob.Error != nil {
		t.Fatalf("completed job error: expected null, got %q", *completedJob.Error)
	}
	if completedJob.LockedBy != nil {
		t.Fatalf("completed job locked_by: expected null, got %q", *completedJob.LockedBy)
	}
	if completedJob.LeaseUntil != nil {
		t.Fatalf("completed job lease_until: expected null, got %s", completedJob.LeaseUntil)
	}
	if completedJob.CreatedAt.IsZero() || completedJob.UpdatedAt.IsZero() {
		t.Fatal("completed job contains zero created_at or updated_at")
	}
	if completedJob.UpdatedAt.Before(completedJob.CreatedAt) {
		t.Fatalf("completed job updated_at %s is before created_at %s", completedJob.UpdatedAt, completedJob.CreatedAt)
	}
}

func waitForHealth(ctx context.Context, client *http.Client, baseURL string) error {
	var lastErr error
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		healthy, err := checkHealth(ctx, client, baseURL)
		if err == nil && healthy {
			return nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return fmt.Errorf("health check did not succeed: %w; last error: %v", ctx.Err(), lastErr)
		case <-ticker.C:
		}
	}
}

func checkHealth(ctx context.Context, client *http.Client, baseURL string) (bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return false, fmt.Errorf("create health request: %w", err)
	}

	response, err := client.Do(request)
	if err != nil {
		return false, fmt.Errorf("send health request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected health status: %d", response.StatusCode)
	}

	return true, nil
}

func createJob(ctx context.Context, client *http.Client, baseURL string, jobType string, payload json.RawMessage) (JobResponse, error) {
	requestBody, err := json.Marshal(CreateJobRequest{
		Type:    jobType,
		Payload: payload,
	})
	if err != nil {
		return JobResponse{}, fmt.Errorf("marshal create job request: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		baseURL+"/jobs",
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return JobResponse{}, fmt.Errorf("create job request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return JobResponse{}, fmt.Errorf("send create job request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return JobResponse{}, fmt.Errorf(
			"create job: expected status %d, got %d: %s",
			http.StatusCreated,
			response.StatusCode,
			body,
		)
	}

	var createdJob JobResponse
	if err := json.NewDecoder(response.Body).Decode(&createdJob); err != nil {
		return JobResponse{}, fmt.Errorf("decode create job response: %w", err)
	}

	return createdJob, nil
}

func getJob(ctx context.Context, client *http.Client, baseURL string, id uuid.UUID) (JobResponse, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		baseURL+"/jobs/"+id.String(),
		nil,
	)
	if err != nil {
		return JobResponse{}, fmt.Errorf("create get job request: %w", err)
	}

	response, err := client.Do(request)
	if err != nil {
		return JobResponse{}, fmt.Errorf("send get job request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return JobResponse{}, fmt.Errorf(
			"get job: expected status %d, got %d: %s",
			http.StatusOK,
			response.StatusCode,
			body,
		)
	}

	var model JobResponse
	if err := json.NewDecoder(response.Body).Decode(&model); err != nil {
		return JobResponse{}, fmt.Errorf("decode get job response: %w", err)
	}

	return model, nil
}

func waitForTerminalJob(ctx context.Context, client *http.Client, baseURL string, id uuid.UUID) (JobResponse, error) {
	var (
		lastErr    error
		lastStatus string
	)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		model, err := getJob(ctx, client, baseURL, id)
		if err != nil {
			lastErr = err
		} else {
			lastStatus = model.Status
			switch model.Status {
			case "succeeded", "failed":
				return model, nil
			case "queued", "running":
				lastErr = nil
			default:
				return JobResponse{}, fmt.Errorf("job %s has unexpected status %q", id, model.Status)
			}
		}

		select {
		case <-ctx.Done():
			return JobResponse{}, fmt.Errorf(
				"wait for job %s: %w; last status: %q; last error: %v",
				id,
				ctx.Err(),
				lastStatus,
				lastErr,
			)
		case <-ticker.C:
		}
	}
}

func jsonEqual(left, right json.RawMessage) bool {
	var leftValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}

	var rightValue any
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}

	return reflect.DeepEqual(leftValue, rightValue)
}
