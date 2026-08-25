package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func parsePayload(payload json.RawMessage) (Payload, error) {
	var parsed Payload

	if err := json.Unmarshal(payload, &parsed); err != nil {
		return Payload{}, ErrInvalidPayload
	}

	return parsed, nil
}

func checkPayload(payload Payload) error {
	if strings.TrimSpace(payload.URL) == "" {
		return errors.New("webhook URL must not be empty")
	}

	parsedURL, err := url.ParseRequestURI(payload.URL)
	if err != nil {
		return errors.New("invalid webhook URL")
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return errors.New("webhook URL scheme must be http or https")
	}

	if strings.TrimSpace(parsedURL.Host) == "" {
		return errors.New("webhook URL host must not be empty")
	}

	if len(payload.Body) > 0 {
		if !json.Valid(payload.Body) {
			return errors.New("webhook body must contain valid JSON")
		}
	}

	return nil
}

func readResponse(response *http.Response) ([]byte, error) {
	const maxBodySize = 1024 * 1024
	limitedReader := io.LimitReader(response.Body, maxBodySize+1)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if len(bodyBytes) > maxBodySize {
		return nil, fmt.Errorf("response body exceeds limit of %d bytes", maxBodySize)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected webhook status %d: %w", response.StatusCode, ErrBadHTTPStatusCode)
	}

	return bodyBytes, nil
}
