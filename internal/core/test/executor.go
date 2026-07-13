package test

import (
	"context"

	"github.com/oswaldom-code/gtool/internal/plugin"
)

type Manager struct {
	executor plugin.TestExecutor
}

func NewManager(executor plugin.TestExecutor) *Manager {
	return &Manager{
		executor: executor,
	}
}

func (m *Manager) Execute(ctx context.Context, config *plugin.TestConfig) (*plugin.TestResult, error) {
	return nil, nil
}
