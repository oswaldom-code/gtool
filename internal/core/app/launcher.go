package app

import (
	"context"

	"github.com/oswaldom-code/gtool/internal/plugin"
)

// Manager manages application lifecycle
type Manager struct {
	launcher plugin.AppLauncher
}

// NewManager creates a new app manager
func NewManager(launcher plugin.AppLauncher) *Manager {
	return &Manager{
		launcher: launcher,
	}
}

// Start starts the application
func (m *Manager) Start(ctx context.Context, config *plugin.AppConfig) error {
	// TODO: Implement in Phase 3
	return nil
}

// Stop stops the application
func (m *Manager) Stop(ctx context.Context) error {
	// TODO: Implement in Phase 3
	return nil
}

// Restart restarts the application
func (m *Manager) Restart(ctx context.Context) error {
	// TODO: Implement in Phase 3
	return nil
}
