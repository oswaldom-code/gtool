package mock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/oswaldom-code/gtool/internal/plugin"
	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
	"go.uber.org/zap"
)

// ServiceStatus represents the status of a service
type ServiceStatus struct {
	Name   string
	Status string // "running", "stopped", "error", "starting"
	Port   int
	Uptime time.Duration
	Error  string
}

// Manager manages mock services lifecycle
type Manager struct {
	registry      *plugin.Registry
	logger        *zap.Logger
	orchestration config.OrchestrationConfig
	services      map[string]*serviceState
	docker        DockerClient
	mu            sync.RWMutex
}

// DockerClient interface for Docker operations
type DockerClient interface {
	ListContainersByLabels(ctx context.Context, labels map[string]string) ([]types.Container, error)
}

// serviceState tracks the state of a running service
type serviceState struct {
	plugin    plugin.ServicePlugin
	startTime time.Time
	stopped   bool
	error     error
}

// NewManager creates a new mock manager
func NewManager(registry *plugin.Registry, logger *zap.Logger, orchestration config.OrchestrationConfig, dockerClient DockerClient) *Manager {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Manager{
		registry:      registry,
		logger:        logger,
		orchestration: orchestration,
		services:      make(map[string]*serviceState),
		docker:        dockerClient,
	}
}

// Start starts a specific service
func (m *Manager) Start(ctx context.Context, serviceName string, config map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already running
	if state, exists := m.services[serviceName]; exists && !state.stopped {
		m.logger.Warn("service already running", zap.String("service", serviceName))
		return nil
	}

	// Get plugin from registry
	servicePlugin, err := m.registry.GetService(serviceName)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
			fmt.Sprintf("service %s not found in registry", serviceName))
	}

	m.logger.Info("starting service", zap.String("service", serviceName))

	// Launch the service
	if err := servicePlugin.Launch(ctx, config); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
			fmt.Sprintf("failed to launch service %s", serviceName))
	}

	// Wait for service to be ready
	m.logger.Info("waiting for service to be ready", zap.String("service", serviceName))

	ready := false
	for i := 0; i < m.orchestration.HealthCheckRetries; i++ {
		isReady, err := servicePlugin.IsReady(ctx)
		if err != nil {
			m.logger.Debug("health check failed",
				zap.String("service", serviceName),
				zap.Error(err),
				zap.Int("attempt", i+1))
		}

		if isReady {
			ready = true
			break
		}

		select {
		case <-ctx.Done():
			return gtErrors.New(gtErrors.ErrServiceFailed, "context cancelled while waiting for service")
		case <-time.After(m.orchestration.HealthCheckInterval):
			// Continue to next attempt
		}
	}

	if !ready {
		// Cleanup on failure if configured
		if m.orchestration.CleanupOnFailure {
			_ = servicePlugin.Stop(ctx)
		}
		return gtErrors.New(gtErrors.ErrServiceNotReady,
			fmt.Sprintf("service %s did not become ready within timeout", serviceName))
	}

	// Store service state
	m.services[serviceName] = &serviceState{
		plugin:    servicePlugin,
		startTime: time.Now(),
		stopped:   false,
	}

	m.logger.Info("service started successfully", zap.String("service", serviceName))
	return nil
}

// Stop stops a specific service
func (m *Manager) Stop(ctx context.Context, serviceName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// First, check if service is in memory
	state, exists := m.services[serviceName]

	// If not in memory, try to find it in Docker and get plugin
	if !exists {
		m.logger.Info("service not in memory, checking Docker", zap.String("service", serviceName))

		// Try to find containers for this service
		if m.docker != nil {
			containers, err := m.docker.ListContainersByLabels(ctx, map[string]string{
				"managed-by": "gtool",
				"service":    serviceName,
			})

			if err != nil {
				m.logger.Error("failed to list containers", zap.Error(err))
			} else if len(containers) == 0 {
				return gtErrors.New(gtErrors.ErrServiceNotRunning,
					fmt.Sprintf("service %s is not running", serviceName))
			}
		}

		// Get plugin from registry to stop the service
		servicePlugin, err := m.registry.GetService(serviceName)
		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
				fmt.Sprintf("service %s not found in registry", serviceName))
		}

		m.logger.Info("stopping service via plugin", zap.String("service", serviceName))

		if err := servicePlugin.Stop(ctx); err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
				fmt.Sprintf("failed to stop service %s", serviceName))
		}

		m.logger.Info("service stopped", zap.String("service", serviceName))
		return nil
	}

	// Service is in memory, stop normally
	if state.stopped {
		return nil
	}

	m.logger.Info("stopping service", zap.String("service", serviceName))

	if err := state.plugin.Stop(ctx); err != nil {
		state.error = err
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
			fmt.Sprintf("failed to stop service %s", serviceName))
	}

	state.stopped = true
	delete(m.services, serviceName)

	m.logger.Info("service stopped", zap.String("service", serviceName))
	return nil
}

// StopAll stops all running services
func (m *Manager) StopAll(ctx context.Context) error {
	m.mu.RLock()
	serviceNames := make([]string, 0, len(m.services))
	for name := range m.services {
		serviceNames = append(serviceNames, name)
	}
	m.mu.RUnlock()

	var errors []error
	for _, name := range serviceNames {
		if err := m.Stop(ctx, name); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to stop some services: %v", errors)
	}

	return nil
}

// GetStatus returns the status of a specific service
func (m *Manager) GetStatus(ctx context.Context, serviceName string) (*ServiceStatus, error) {
	m.mu.RLock()
	state, exists := m.services[serviceName]
	m.mu.RUnlock()

	// If not in memory, check Docker
	if !exists {
		if m.docker != nil {
			containers, err := m.docker.ListContainersByLabels(ctx, map[string]string{
				"managed-by": "gtool",
				"service":    serviceName,
			})

			if err != nil {
				m.logger.Debug("failed to list containers for status", zap.Error(err))
			} else if len(containers) > 0 {
				// Found container(s) in Docker
				container := containers[0] // Use first container
				status := &ServiceStatus{
					Name:   serviceName,
					Status: container.State,
				}

				// Try to get port from container
				if len(container.Ports) > 0 {
					status.Port = int(container.Ports[0].PublicPort)
				}

				// Calculate uptime
				if container.State == "running" {
					status.Status = "running"
					// Note: container.Created is a Unix timestamp
					status.Uptime = time.Since(time.Unix(container.Created, 0))
				}

				return status, nil
			}
		}

		// Not in memory and not in Docker
		return &ServiceStatus{
			Name:   serviceName,
			Status: "stopped",
		}, nil
	}

	// Service is in memory
	status := &ServiceStatus{
		Name:   serviceName,
		Uptime: time.Since(state.startTime),
	}

	if state.stopped {
		status.Status = "stopped"
		return status, nil
	}

	if state.error != nil {
		status.Status = "error"
		status.Error = state.error.Error()
		return status, nil
	}

	// Check if service is still ready
	ready, err := state.plugin.IsReady(ctx)
	if err != nil || !ready {
		status.Status = "error"
		if err != nil {
			status.Error = err.Error()
		}
		return status, nil
	}

	status.Status = "running"

	// Get connection info for port
	if connInfo, err := state.plugin.GetConnectionInfo(); err == nil {
		status.Port = connInfo.Port
	}

	return status, nil
}

// GetAllStatuses returns the status of all services
func (m *Manager) GetAllStatuses(ctx context.Context) []*ServiceStatus {
	// Get running services from both memory and Docker
	serviceNames := m.ListRunning()

	statuses := make([]*ServiceStatus, 0, len(serviceNames))
	for _, name := range serviceNames {
		status, err := m.GetStatus(ctx, name)
		if err != nil {
			statuses = append(statuses, &ServiceStatus{
				Name:   name,
				Status: "error",
				Error:  err.Error(),
			})
		} else {
			statuses = append(statuses, status)
		}
	}

	return statuses
}

// GetLogs retrieves logs from a service
func (m *Manager) GetLogs(ctx context.Context, serviceName string, opts *plugin.LogOptions) ([]string, error) {
	m.mu.RLock()
	state, exists := m.services[serviceName]
	m.mu.RUnlock()

	// If in memory, use the plugin
	if exists {
		return state.plugin.GetLogs(ctx, opts)
	}

	// Not in memory, try to get plugin and let it find the container
	m.logger.Info("service not in memory for logs, checking Docker", zap.String("service", serviceName))

	// Verify container exists in Docker first
	if m.docker != nil {
		containers, err := m.docker.ListContainersByLabels(ctx, map[string]string{
			"managed-by": "gtool",
			"service":    serviceName,
		})

		if err != nil {
			return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to check for containers")
		}

		if len(containers) == 0 {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning,
				fmt.Sprintf("service %s is not running", serviceName))
		}
	}

	// Get plugin from registry
	servicePlugin, err := m.registry.GetService(serviceName)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
			fmt.Sprintf("service %s not found in registry", serviceName))
	}

	// Let the plugin get logs (it will find the container by labels)
	return servicePlugin.GetLogs(ctx, opts)
}

// ListRunning returns a list of currently running services
func (m *Manager) ListRunning() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Start with services in memory
	servicesMap := make(map[string]bool)
	for name, state := range m.services {
		if !state.stopped {
			servicesMap[name] = true
		}
	}

	// Also check Docker for containers managed by gtool
	if m.docker != nil {
		ctx := context.Background()
		containers, err := m.docker.ListContainersByLabels(ctx, map[string]string{
			"managed-by": "gtool",
		})

		if err != nil {
			m.logger.Error("failed to list running containers", zap.Error(err))
		} else {
			// Extract service names from labels
			for _, container := range containers {
				if container.State == "running" {
					if serviceName, ok := container.Labels["service"]; ok {
						servicesMap[serviceName] = true
					}
				}
			}
		}
	}

	// Convert map to slice
	services := make([]string, 0, len(servicesMap))
	for name := range servicesMap {
		services = append(services, name)
	}

	return services
}

// StartAll starts all configured mock services
func (m *Manager) StartAll(ctx context.Context, serviceConfigs map[string]map[string]interface{}) error {
	if m.orchestration.ParallelMocks {
		return m.startParallel(ctx, serviceConfigs)
	}
	return m.startSequential(ctx, serviceConfigs)
}

// startSequential starts services one by one
func (m *Manager) startSequential(ctx context.Context, serviceConfigs map[string]map[string]interface{}) error {
	for serviceName, config := range serviceConfigs {
		if err := m.Start(ctx, serviceName, config); err != nil {
			if m.orchestration.CleanupOnFailure {
				_ = m.StopAll(ctx)
			}
			return err
		}
	}
	return nil
}

// startParallel starts services in parallel
func (m *Manager) startParallel(ctx context.Context, serviceConfigs map[string]map[string]interface{}) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(serviceConfigs))

	for serviceName, config := range serviceConfigs {
		wg.Add(1)
		go func(name string, cfg map[string]interface{}) {
			defer wg.Done()
			if err := m.Start(ctx, name, cfg); err != nil {
				errChan <- fmt.Errorf("%s: %w", name, err)
			}
		}(serviceName, config)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		if m.orchestration.CleanupOnFailure {
			_ = m.StopAll(ctx)
		}
		return fmt.Errorf("failed to start services: %v", errors)
	}

	return nil
}
