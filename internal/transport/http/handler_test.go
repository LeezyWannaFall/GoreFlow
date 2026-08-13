package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LeezyWannaFall/GoreFlow/internal/application"
	"github.com/LeezyWannaFall/GoreFlow/internal/job"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type fakeJobService struct {
	createCalls   int
	createType    string
	createPayload json.RawMessage
	createResult  job.Job
	createErr     error

	getCalls  int
	getID     uuid.UUID
	getResult job.Job
	getErr    error
}

func (f *fakeJobService) CreateJob(_ context.Context, jobType string, payload json.RawMessage) (job.Job, error) {
	f.createCalls++
	f.createType = jobType
	f.createPayload = payload
	return f.createResult, f.createErr
}

func (f *fakeJobService) GetJobByID(_ context.Context, id uuid.UUID) (job.Job, error) {
	f.getCalls++
	f.getID = id
	return f.getResult, f.getErr
}

func TestHandler_CreateJob(t *testing.T) {
	now := time.Date(2026, time.August, 13, 8, 0, 0, 0, time.UTC)
	createdJob := job.Job{
		ID:          uuid.New(),
		Type:        "echo",
		Payload:     json.RawMessage(`{"message":"hello"}`),
		Status:      job.StatusQueued,
		MaxAttempts: 1,
		RunAfter:    now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	tests := []struct {
		name            string
		body            string
		serviceResult   job.Job
		serviceErr      error
		wantStatus      int
		wantCreateCalls int
		wantError       string
	}{
		{
			name:            "creates job",
			body:            `{"type":"echo","payload":{"message":"hello"}}`,
			serviceResult:   createdJob,
			wantStatus:      http.StatusCreated,
			wantCreateCalls: 1,
		},
		{
			name:            "rejects malformed JSON",
			body:            `{"type":"echo"`,
			wantStatus:      http.StatusBadRequest,
			wantCreateCalls: 0,
			wantError:       "invalid request body",
		},
		{
			name:            "rejects unknown field",
			body:            `{"type":"echo","payload":{},"unknown":true}`,
			wantStatus:      http.StatusBadRequest,
			wantCreateCalls: 0,
			wantError:       "invalid request body",
		},
		{
			name:            "maps domain validation error to bad request",
			body:            `{"type":"","payload":{}}`,
			serviceErr:      job.ErrInvalidJobType,
			wantStatus:      http.StatusBadRequest,
			wantCreateCalls: 1,
			wantError:       "invalid job data",
		},
		{
			name:            "maps internal error to server error",
			body:            `{"type":"echo","payload":{}}`,
			serviceErr:      errors.New("database unavailable"),
			wantStatus:      http.StatusInternalServerError,
			wantCreateCalls: 1,
			wantError:       "internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeJobService{
				createResult: tt.serviceResult,
				createErr:    tt.serviceErr,
			}
			handler := NewHandler(service)
			request := httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader(tt.body))
			response := httptest.NewRecorder()

			handler.CreateJob(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.wantStatus, response.Body.String())
			}
			if service.createCalls != tt.wantCreateCalls {
				t.Fatalf("CreateJob() calls = %d, want %d", service.createCalls, tt.wantCreateCalls)
			}
			if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", contentType)
			}

			if tt.wantError != "" {
				assertErrorResponse(t, response, tt.wantError)
				return
			}

			if service.createType != "echo" {
				t.Errorf("job type = %q, want echo", service.createType)
			}
			if string(service.createPayload) != `{"message":"hello"}` {
				t.Errorf("payload = %s, want message payload", service.createPayload)
			}

			var body JobResponse
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.ID != createdJob.ID || body.Status != job.StatusQueued {
				t.Errorf("response job = %#v, want ID %s and queued status", body, createdJob.ID)
			}
			if body.LockedBy != nil || body.LeaseUntil != nil || body.Error != nil || string(body.Result) != "null" {
				t.Errorf("nullable response fields must be null: %#v", body)
			}
		})
	}
}

func TestHandler_GetJobByID(t *testing.T) {
	jobID := uuid.New()
	existingJob := job.Job{
		ID:          jobID,
		Type:        "echo",
		Payload:     json.RawMessage(`{"message":"hello"}`),
		Status:      job.StatusQueued,
		MaxAttempts: 1,
		RunAfter:    time.Now().UTC(),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	tests := []struct {
		name         string
		pathID       string
		serviceJob   job.Job
		serviceErr   error
		wantStatus   int
		wantGetCalls int
		wantError    string
	}{
		{
			name:         "returns job",
			pathID:       jobID.String(),
			serviceJob:   existingJob,
			wantStatus:   http.StatusOK,
			wantGetCalls: 1,
		},
		{
			name:         "rejects invalid UUID",
			pathID:       "not-a-uuid",
			wantStatus:   http.StatusBadRequest,
			wantGetCalls: 0,
			wantError:    "invalid job ID",
		},
		{
			name:         "rejects nil UUID",
			pathID:       uuid.Nil.String(),
			wantStatus:   http.StatusBadRequest,
			wantGetCalls: 0,
			wantError:    "invalid job ID",
		},
		{
			name:         "maps not found error",
			pathID:       uuid.NewString(),
			serviceErr:   application.ErrJobNotFound,
			wantStatus:   http.StatusNotFound,
			wantGetCalls: 1,
			wantError:    "job not found",
		},
		{
			name:         "maps internal error",
			pathID:       uuid.NewString(),
			serviceErr:   errors.New("database unavailable"),
			wantStatus:   http.StatusInternalServerError,
			wantGetCalls: 1,
			wantError:    "internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeJobService{getResult: tt.serviceJob, getErr: tt.serviceErr}
			handler := NewHandler(service)
			router := chi.NewRouter()
			router.Get("/jobs/{id}", handler.GetJobByID)

			request := httptest.NewRequest(http.MethodGet, "/jobs/"+tt.pathID, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.wantStatus, response.Body.String())
			}
			if service.getCalls != tt.wantGetCalls {
				t.Fatalf("GetJobByID() calls = %d, want %d", service.getCalls, tt.wantGetCalls)
			}

			if tt.wantError != "" {
				assertErrorResponse(t, response, tt.wantError)
				return
			}

			if service.getID != jobID {
				t.Errorf("job ID = %s, want %s", service.getID, jobID)
			}

			var body JobResponse
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.ID != jobID || body.Type != existingJob.Type {
				t.Errorf("response job = %#v, want ID %s and type %q", body, jobID, existingJob.Type)
			}
		})
	}
}

func assertErrorResponse(t *testing.T, response *httptest.ResponseRecorder, want string) {
	t.Helper()

	var body errorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error != want {
		t.Errorf("error response = %q, want %q", body.Error, want)
	}
}
