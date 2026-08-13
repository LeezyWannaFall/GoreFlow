package executor

import (
	"errors"
	"strings"
)

type Registry struct {
	executors map[string]Executor
}

func NewRegistry() *Registry {
	return &Registry{
		executors: make(map[string]Executor),
	}
}

func (r *Registry) Register(name string, executor Executor) error {
	if _, exists := r.executors[name]; exists {
		return errors.New("executor already registered: " + name)
	}

	if executor == nil {
		return errors.New("executor cannot be nil")
	}

	if strings.TrimSpace(name) == "" {
		return errors.New("executor name cannot be empty")
	}

	r.executors[name] = executor
	return nil
}

func (r *Registry) Get(name string) (Executor, bool) {
	executor, exists := r.executors[name]
	return executor, exists
}
