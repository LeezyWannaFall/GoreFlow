package executor

import (
	"context"
	"testing"
)

func TestEchoExecutor_Execute(t *testing.T) {
	executor := NewEchoExecutor()

	payload := []byte(`{"message": "hello world"}`)

	testCases := []struct {
		name    string
		payload []byte
		wantErr bool
	}{
		{
			name:    "Valid JSON",
			payload: payload,
			wantErr: false,
		},
		{
			name:    "Empty JSON",
			payload: []byte(`{}`),
			wantErr: false,
		},
		{
			name:    "Nested JSON",
			payload: []byte(`{"outer": {"inner": "value"}}`),
			wantErr: false,
		},
		{
			name:    "Array JSON",
			payload: []byte(`[{"item": 1}, {"item": 2}]`),
			wantErr: false,
		},
		{
			name:    "Invalid JSON",
			payload: []byte(`{"message": "invalid json"`),
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := executor.Execute(context.Background(), tc.payload)
			if tc.wantErr {
				if err == nil {
					t.Fatal("Execute() error = nil, want error")
				}
				if result != nil {
					t.Errorf("Execute() result = %s, want nil", result)
				}
				return
			}

			if err != nil {
				t.Fatalf("Execute() unexpected error = %v", err)
			}

			if string(result) != string(tc.payload) {
				t.Errorf("Execute() = %s, want %s", result, tc.payload)
			}
		})
	}
}
