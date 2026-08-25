package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

var ErrInvalidPayload = errors.New("invalid JSON payload")
var ErrBadHTTPStatusCode = errors.New("bad HTTP status code")

type WebhookExecutor struct {
	client *http.Client
}

type Payload struct {
	URL  string          `json:"url"`
	Body json.RawMessage `json:"body"`
}

type Result struct {
	StatusCode int    `json:"status_code"`
	Body       string `json:"body"`
}

func NewWebhookExecutor(client *http.Client) (*WebhookExecutor, error) {
	if client == nil {
		return nil, errors.New("http client must not be nil")
	}

	return &WebhookExecutor{client: client}, nil
}

func (e *WebhookExecutor) Execute(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	parsedPayload, err := parsePayload(payload)
	if err != nil {
		return nil, fmt.Errorf("parse payload: %w", err)
	}

	if err := checkPayload(parsedPayload); err != nil {
		return nil, fmt.Errorf("payload validation: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, parsedPayload.URL, bytes.NewReader(parsedPayload.Body))
	if err != nil {
		return nil, fmt.Errorf("create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	response, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send webhook request: %w", err)
	}
	defer response.Body.Close()

	bodyBytes, err := readResponse(response)
	if err != nil {
		return nil, fmt.Errorf("read webhook response: %w", err)
	}

	webhookResult := &Result{
		StatusCode: response.StatusCode,
		Body:       string(bodyBytes),
	}

	result, err := json.Marshal(webhookResult)
	if err != nil {
		return nil, fmt.Errorf("serialize webhook result: %w", err)
	}

	return result, nil
}
