package executor

import (
	"context"
	"encoding/json"
	"errors"
)

type EchoExecutor struct{}

func NewEchoExecutor() *EchoExecutor {
	return &EchoExecutor{}
}

func (e *EchoExecutor) Execute(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	if !json.Valid(payload) {
		return nil, errors.New("invalid JSON payload")
	}

	return payload, nil
}
