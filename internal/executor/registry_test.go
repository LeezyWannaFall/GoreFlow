package executor

import (
	"context"
	"encoding/json"
	"testing"
)

type registryTestExecutor struct {
	id int
}

func (e *registryTestExecutor) Execute(context.Context, json.RawMessage) (json.RawMessage, error) {
	return nil, nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	registry := NewRegistry()
	echoExecutor := NewEchoExecutor()

	if err := registry.Register("echo", echoExecutor); err != nil {
		t.Fatalf("Register() unexpected error = %v", err)
	}

	registeredExecutor, exists := registry.Get("echo")
	if !exists {
		t.Fatal("Get() exists = false, want true")
	}
	if registeredExecutor != echoExecutor {
		t.Errorf("Get() executor = %v, want %v", registeredExecutor, echoExecutor)
	}
}

func TestRegistry_GetUnknownExecutor(t *testing.T) {
	registry := NewRegistry()

	registeredExecutor, exists := registry.Get("unknown")
	if exists {
		t.Fatal("Get() exists = true, want false")
	}
	if registeredExecutor != nil {
		t.Errorf("Get() executor = %v, want nil", registeredExecutor)
	}
}

func TestRegistry_Register(t *testing.T) {
	testCases := []struct {
		name         string
		registryName string
		existing     Executor
		toRegister   Executor
		wantExists   bool
	}{
		{
			name:         "Nil executor",
			registryName: "echo",
			toRegister:   nil,
			wantExists:   false,
		},
		{
			name:         "Duplicate executor",
			registryName: "echo",
			existing:     &registryTestExecutor{id: 1},
			toRegister:   &registryTestExecutor{id: 2},
			wantExists:   true,
		},
		{
			name:         "Empty executor name",
			registryName: "",
			toRegister:   &registryTestExecutor{id: 1},
			wantExists:   false,
		},
		{
			name:         "Whitespace-only executor name",
			registryName: "   ",
			toRegister:   &registryTestExecutor{id: 1},
			wantExists:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			registry := NewRegistry()
			if tc.existing != nil {
				if err := registry.Register(tc.registryName, tc.existing); err != nil {
					t.Fatalf("setup Register() unexpected error = %v", err)
				}
			}

			err := registry.Register(tc.registryName, tc.toRegister)
			if err == nil {
				t.Fatal("Register() error = nil, want error")
			}

			registeredExecutor, exists := registry.Get(tc.registryName)
			if exists != tc.wantExists {
				t.Fatalf("Get() exists = %t after failed registration, want %t", exists, tc.wantExists)
			}
			if registeredExecutor != tc.existing {
				t.Errorf("Get() executor = %v after failed registration, want %v", registeredExecutor, tc.existing)
			}
		})
	}
}
