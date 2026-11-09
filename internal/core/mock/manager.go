package mock

import (
	"context"
	"errors"
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
	starting  bool
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

// reserve records serviceName as starting, so concurrent Starts of the same
// service and StopAll see it. It returns nil when the service already runs.
func (m *Manager) reserve(serviceName string) (plugin.ServicePlugin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.services[serviceName]; exists && !state.stopped {
		m.logger.Warn("service already running", zap.String("service", serviceName))
		return nil, nil
	}

	servicePlugin, err := m.registry.GetService(serviceName)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
			fmt.Sprintf("service %s not found in registry", serviceName))
	}

	m.services[serviceName] = &serviceState{plugin: servicePlugin, startTime: time.Now(), starting: true}
	return servicePlugin, nil
}

func (m *Manager) forget(serviceName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.services, serviceName)
}

func (m *Manager) markReady(serviceName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state, ok := m.services[serviceName]; ok {
		state.starting = false
		state.startTime = time.Now()
	}
}

func (m *Manager) markFailed(serviceName string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state, ok := m.services[serviceName]; ok {
		state.starting = false
		state.error = err
	}
}

// waitReady polls the plugin until it is ready, the retries run out or ctx is
// cancelled.
func (m *Manager) waitReady(ctx context.Context, serviceName string, servicePlugin plugin.ServicePlugin) error {
	for i := 0; i < m.orchestration.HealthCheckRetries; i++ {
		isReady, err := servicePlugin.IsReady(ctx)
		if gtErrors.Is(err, gtErrors.ErrContainerExited) {
			return err
		}
		if err != nil {
			m.logger.Debug("health check failed",
				zap.String("service", serviceName),
				zap.Error(err),
				zap.Int("attempt", i+1))
		}
		if isReady {
			return nil
		}

		select {
		case <-ctx.Done():
			return gtErrors.Wrap(ctx.Err(), gtErrors.ErrServiceFailed, "context cancelled while waiting for service")
		case <-time.After(m.orchestration.HealthCheckInterval):
		}
	}
	return gtErrors.New(gtErrors.ErrServiceNotReady,
		fmt.Sprintf("service %s did not become ready within timeout", serviceName))
}

// Start launches a service and waits until it is ready. The manager lock is
// held only to update its state, so services start concurrently. A service
// that launched but did not become ready is stopped when the context was
// cancelled or cleanup-on-failure is set; otherwise it stays recorded (state
// "error") for inspection and a later StopAll.
func (m *Manager) Start(ctx context.Context, serviceName string, config map[string]interface{}) error {
	servicePlugin, err := m.reserve(serviceName)
	if err != nil || servicePlugin == nil {
		return err
	}

	m.logger.Info("starting service", zap.String("service", serviceName))
	if err := servicePlugin.Launch(ctx, config); err != nil {
		m.forget(serviceName)
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
			fmt.Sprintf("failed to launch service %s", serviceName))
	}

	m.logger.Info("waiting for service to be ready", zap.String("service", serviceName))
	if err := m.waitReady(ctx, serviceName, servicePlugin); err != nil {
		if ctx.Err() != nil || m.orchestration.CleanupOnFailure {
			if stopErr := servicePlugin.Stop(context.Background()); stopErr != nil {
				m.logger.Warn("failed to stop service after a failed start",
					zap.String("service", serviceName), zap.Error(stopErr))
			}
			m.forget(serviceName)
		} else {
			m.markFailed(serviceName, err)
		}
		return err
	}

	m.markReady(serviceName)
	m.logger.Info("service started successfully", zap.String("service", serviceName))
	return nil
}

// Stop stops a specific service
func (m *Manager) Stop(ctx context.Context, serviceName string) error {
	m.mu.RLock()
	state, exists := m.services[serviceName]
	m.mu.RUnlock()

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
		m.markFailed(serviceName, err)
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
			fmt.Sprintf("failed to stop service %s", serviceName))
	}

	m.forget(serviceName)

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

	var errs []error
	for _, name := range serviceNames {
		if err := m.Stop(ctx, name); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return gtErrors.Wrap(errors.Join(errs...), gtErrors.ErrServiceFailed, "failed to stop some services")
	}

	return nil
}

// GetStatus returns the status of a specific service
func (m *Manager) GetStatus(ctx context.Context, serviceName string) (*ServiceStatus, error) {
	m.mu.RLock()
	var state serviceState
	current, exists := m.services[serviceName]
	if exists {
		state = *current
	}
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

	if state.starting {
		status.Status = "starting"
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
	servicesMap := make(map[string]bool)
	m.mu.RLock()
	for name, state := range m.services {
		if !state.stopped {
			servicesMap[name] = true
		}
	}
	m.mu.RUnlock()

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
				_ = m.StopAll(context.Background())
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
				errChan <- err
			}
		}(serviceName, config)
	}

	wg.Wait()
	close(errChan)

	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		if m.orchestration.CleanupOnFailure {
			_ = m.StopAll(context.Background())
		}
		return gtErrors.Wrap(errors.Join(errs...), gtErrors.ErrServiceFailed, "failed to start services")
	}

	return nil
}
