//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LeezyWannaFall/GoreFlow/internal/application"
	"github.com/LeezyWannaFall/GoreFlow/internal/job"
	"github.com/LeezyWannaFall/GoreFlow/internal/storage/postgres"
	httptransport "github.com/LeezyWannaFall/GoreFlow/internal/transport/http"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestHTTP_CreateAndGetJob(t *testing.T) {
	db := openTestDatabase(t)
	repository := postgres.NewRepository(db)
	app := application.NewApplication(repository)
	handler := httptransport.NewHandler(app)
	router := chi.NewRouter()
	router.Post("/jobs", handler.CreateJob)
	router.Get("/jobs/{id}", handler.GetJobByID)
	server := httptest.NewServer(router)
	defer server.Close()

	payload := json.RawMessage(`{"message":"HTTP integration"}`)
	requestBody := json.RawMessage(`{"type":"echo","payload":{"message":"HTTP integration"}}`)
	response := sendRequest(t, server.Client(), http.MethodPost, server.URL+"/jobs", requestBody)
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("POST /jobs status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, readBody(t, response))
	}

	var created httptransport.JobResponse
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("decode POST /jobs response: %v", err)
	}
	if created.ID == uuid.Nil || created.Type != "echo" || created.Status != job.StatusQueued {
		t.Errorf("POST /jobs response contains unexpected job: %#v", created)
	}
	if !jsonEqual(created.Payload, payload) {
		t.Errorf("POST /jobs payload = %s, want %s", created.Payload, payload)
	}

	response = sendRequest(t, server.Client(), http.MethodGet, server.URL+"/jobs/"+created.ID.String(), nil)
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /jobs/{id} status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, readBody(t, response))
	}

	var stored httptransport.JobResponse
	if err := json.NewDecoder(response.Body).Decode(&stored); err != nil {
		t.Fatalf("decode GET /jobs/{id} response: %v", err)
	}
	if stored.ID != created.ID || stored.Type != created.Type || stored.Status != created.Status {
		t.Errorf("GET /jobs/{id} response = %#v, want created job %#v", stored, created)
	}
	if !jsonEqual(stored.Payload, payload) {
		t.Errorf("GET /jobs/{id} payload = %s, want %s", stored.Payload, payload)
	}
}

func TestHTTP_JobErrors(t *testing.T) {
	db := openTestDatabase(t)
	repository := postgres.NewRepository(db)
	app := application.NewApplication(repository)
	handler := httptransport.NewHandler(app)
	router := chi.NewRouter()
	router.Post("/jobs", handler.CreateJob)
	router.Get("/jobs/{id}", handler.GetJobByID)
	server := httptest.NewServer(router)
	defer server.Close()

	testCases := []struct {
		name       string
		method     string
		path       string
		body       json.RawMessage
		wantStatus int
		wantError  string
	}{
		{
			name:       "Invalid create body",
			method:     http.MethodPost,
			path:       "/jobs",
			body:       json.RawMessage(`{"type":`),
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request body",
		},
		{
			name:       "Invalid job data",
			method:     http.MethodPost,
			path:       "/jobs",
			body:       json.RawMessage(`{"type":" ","payload":{}}`),
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid job data",
		},
		{
			name:       "Invalid job ID",
			method:     http.MethodGet,
			path:       "/jobs/not-a-uuid",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid job ID",
		},
		{
			name:       "Job not found",
			method:     http.MethodGet,
			path:       "/jobs/" + uuid.NewString(),
			wantStatus: http.StatusNotFound,
			wantError:  "job not found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			response := sendRequest(t, server.Client(), tc.method, server.URL+tc.path, tc.body)
			defer response.Body.Close()
			if response.StatusCode != tc.wantStatus {
				t.Fatalf("response status = %d, want %d; body = %s", response.StatusCode, tc.wantStatus, readBody(t, response))
			}

			var errorBody struct {
				Error string `json:"error"`
			}
			if err := json.NewDecoder(response.Body).Decode(&errorBody); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if errorBody.Error != tc.wantError {
				t.Errorf("response error = %q, want %q", errorBody.Error, tc.wantError)
			}
		})
	}
}

func sendRequest(t *testing.T, client *http.Client, method string, targetURL string, body json.RawMessage) *http.Response {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), method, targetURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create HTTP request: %v", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("send HTTP request: %v", err)
	}

	return response
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<10))
	if err != nil {
		t.Fatalf("read HTTP response body: %v", err)
	}

	return string(body)
}
