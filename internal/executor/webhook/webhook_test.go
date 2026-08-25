package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestNewWebhookExecutor(t *testing.T) {
	testCases := []struct {
		name    string
		client  *http.Client
		wantErr bool
	}{
		{
			name:    "Valid client",
			client:  &http.Client{},
			wantErr: false,
		},
		{
			name:    "Nil client",
			client:  nil,
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			executor, err := NewWebhookExecutor(tc.client)
			if tc.wantErr {
				if err == nil {
					t.Fatal("NewWebhookExecutor() error = nil, want error")
				}
				if executor != nil {
					t.Errorf("NewWebhookExecutor() executor = %v, want nil", executor)
				}
				return
			}

			if err != nil {
				t.Fatalf("NewWebhookExecutor() unexpected error = %v", err)
			}
			if executor == nil {
				t.Fatal("NewWebhookExecutor() executor = nil, want non-nil")
			}
			if executor.client != tc.client {
				t.Errorf("NewWebhookExecutor() client = %p, want %p", executor.client, tc.client)
			}
		})
	}
}

func TestWebhookExecutor_Execute(t *testing.T) {
	type receivedRequest struct {
		method      string
		contentType string
		body        string
	}

	received := make(chan receivedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body json.RawMessage
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http.Error(writer, "invalid request body", http.StatusBadRequest)
			return
		}

		received <- receivedRequest{method: request.Method, contentType: request.Header.Get("Content-Type"), body: string(body)}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"accepted":true}`))
	}))
	defer server.Close()

	executor, err := NewWebhookExecutor(server.Client())
	if err != nil {
		t.Fatalf("NewWebhookExecutor() unexpected error = %v", err)
	}

	payload := json.RawMessage(`{"url":"` + server.URL + `","body":{"message":"hello"}}`)
	resultJSON, err := executor.Execute(context.Background(), payload)
	if err != nil {
		t.Fatalf("Execute() unexpected error = %v", err)
	}

	request := <-received
	if request.method != http.MethodPost {
		t.Errorf("request method = %q, want %q", request.method, http.MethodPost)
	}
	if request.contentType != "application/json" {
		t.Errorf("request Content-Type = %q, want %q", request.contentType, "application/json")
	}
	if request.body != `{"message":"hello"}` {
		t.Errorf("request body = %s, want %s", request.body, `{"message":"hello"}`)
	}

	var result Result
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatalf("unmarshal Execute() result: %v", err)
	}
	if result.StatusCode != http.StatusCreated {
		t.Errorf("Execute() result status code = %d, want %d", result.StatusCode, http.StatusCreated)
	}
	if result.Body != `{"accepted":true}` {
		t.Errorf("Execute() result body = %q, want %q", result.Body, `{"accepted":true}`)
	}
}

func TestWebhookExecutor_ExecutePayloadValidation(t *testing.T) {
	testCases := []struct {
		name        string
		payload     json.RawMessage
		wantErrorIs error
		wantError   string
	}{
		{
			name:        "Invalid JSON payload",
			payload:     json.RawMessage(`{"url":`),
			wantErrorIs: ErrInvalidPayload,
		},
		{
			name:      "Empty URL",
			payload:   json.RawMessage(`{"url":"","body":{}}`),
			wantError: "webhook URL must not be empty",
		},
		{
			name:      "Whitespace-only URL",
			payload:   json.RawMessage(`{"url":"   ","body":{}}`),
			wantError: "webhook URL must not be empty",
		},
		{
			name:      "Invalid URL",
			payload:   json.RawMessage(`{"url":"http://[::1","body":{}}`),
			wantError: "invalid webhook URL",
		},
		{
			name:      "URL without host",
			payload:   json.RawMessage(`{"url":"http:///callback","body":{}}`),
			wantError: "webhook URL host must not be empty",
		},
		{
			name:      "Unsupported URL scheme",
			payload:   json.RawMessage(`{"url":"ftp://example.com/callback","body":{}}`),
			wantError: "webhook URL scheme must be http or https",
		},
	}

	executor, err := NewWebhookExecutor(&http.Client{})
	if err != nil {
		t.Fatalf("NewWebhookExecutor() unexpected error = %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := executor.Execute(context.Background(), tc.payload)
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if result != nil {
				t.Errorf("Execute() result = %s, want nil", result)
			}
			if tc.wantErrorIs != nil && !errors.Is(err, tc.wantErrorIs) {
				t.Errorf("Execute() error = %v, want errors.Is(_, %v)", err, tc.wantErrorIs)
			}
			if tc.wantError != "" && !strings.Contains(err.Error(), tc.wantError) {
				t.Errorf("Execute() error = %q, want it to contain %q", err, tc.wantError)
			}
		})
	}
}

func TestCheckPayloadRejectsInvalidBody(t *testing.T) {
	payload := Payload{URL: "https://example.com/callback", Body: json.RawMessage(`{"invalid"`)}

	err := checkPayload(payload)
	if err == nil {
		t.Fatal("checkPayload() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "webhook body must contain valid JSON") {
		t.Errorf("checkPayload() error = %q, want invalid body error", err)
	}
}

func TestWebhookExecutor_ExecuteHTTPStatus(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
	}{
		{
			name:       "Client error",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "Server error",
			statusCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(tc.statusCode)
			}))
			defer server.Close()

			executor, err := NewWebhookExecutor(server.Client())
			if err != nil {
				t.Fatalf("NewWebhookExecutor() unexpected error = %v", err)
			}

			payload := json.RawMessage(`{"url":"` + server.URL + `","body":{}}`)
			result, err := executor.Execute(context.Background(), payload)
			if !errors.Is(err, ErrBadHTTPStatusCode) {
				t.Fatalf("Execute() error = %v, want errors.Is(_, %v)", err, ErrBadHTTPStatusCode)
			}
			if result != nil {
				t.Errorf("Execute() result = %s, want nil", result)
			}
		})
	}
}

func TestWebhookExecutor_ExecuteTransportError(t *testing.T) {
	wantErr := errors.New("service unavailable")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, wantErr
	})}
	executor, err := NewWebhookExecutor(client)
	if err != nil {
		t.Fatalf("NewWebhookExecutor() unexpected error = %v", err)
	}

	result, err := executor.Execute(context.Background(), json.RawMessage(`{"url":"https://example.com/callback","body":{}}`))
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want errors.Is(_, %v)", err, wantErr)
	}
	if result != nil {
		t.Errorf("Execute() result = %s, want nil", result)
	}
}

func TestWebhookExecutor_ExecuteResponseBodyTooLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(strings.Repeat("a", 1024*1024+1)))
	}))
	defer server.Close()

	executor, err := NewWebhookExecutor(server.Client())
	if err != nil {
		t.Fatalf("NewWebhookExecutor() unexpected error = %v", err)
	}

	payload := json.RawMessage(`{"url":"` + server.URL + `","body":{}}`)
	result, err := executor.Execute(context.Background(), payload)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "response body exceeds limit") {
		t.Errorf("Execute() error = %q, want response size error", err)
	}
	if result != nil {
		t.Errorf("Execute() result = %s, want nil", result)
	}
}

func TestWebhookExecutor_ExecuteCanceledContext(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	executor, err := NewWebhookExecutor(client)
	if err != nil {
		t.Fatalf("NewWebhookExecutor() unexpected error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := executor.Execute(ctx, json.RawMessage(`{"url":"https://example.com/callback","body":{}}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want errors.Is(_, %v)", err, context.Canceled)
	}
	if result != nil {
		t.Errorf("Execute() result = %s, want nil", result)
	}
}
