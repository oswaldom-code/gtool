package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/oswaldo-montano/gtool/internal/infra/docker"
	"github.com/oswaldo-montano/gtool/internal/plugin"
	gtErrors "github.com/oswaldo-montano/gtool/pkg/errors"
	"go.uber.org/zap"
)

const (
	defaultImage        = "redis:7-alpine"
	defaultPort         = "6379"
	containerNamePrefix = "gtool-redis"
)

// RedisPlugin implements the ServicePlugin interface for Redis
type RedisPlugin struct {
	docker      *docker.Client
	logger      *zap.Logger
	containerID string
	config      *RedisConfig
}

// RedisConfig holds Redis-specific configuration
type RedisConfig struct {
	Image         string `json:"image"`
	Port          string `json:"port"`
	Password      string `json:"password"`
	ContainerName string `json:"container-name"`
}

// NewRedisPlugin creates a new Redis service plugin
func NewRedisPlugin(dockerClient *docker.Client, logger *zap.Logger) *RedisPlugin {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &RedisPlugin{
		docker: dockerClient,
		logger: logger,
	}
}

// Name returns the service identifier
func (p *RedisPlugin) Name() string {
	return "redis"
}

// Launch starts the Redis service with given configuration
func (p *RedisPlugin) Launch(ctx context.Context, config map[string]interface{}) error {
	p.logger.Info("launching Redis service")

	// Parse configuration
	cfg, err := p.parseConfig(config)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to parse Redis configuration")
	}
	p.config = cfg

	// Pull image
	p.logger.Info("pulling Redis image", zap.String("image", cfg.Image))
	if err := p.docker.EnsureImage(ctx, cfg.Image); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to pull Redis image")
	}

	// Create container
	containerConfig := &docker.ContainerConfig{
		Image: cfg.Image,
		Name:  cfg.ContainerName,
		PortBindings: map[string]string{
			"6379": cfg.Port,
		},
		Labels: map[string]string{
			"managed-by": "gtool",
			"service":    "redis",
		},
	}

	if cfg.Password != "" {
		containerConfig.Cmd = []string{"redis-server", "--requirepass", cfg.Password}
	}

	p.logger.Info("creating Redis container",
		zap.String("name", cfg.ContainerName),
		zap.String("port", cfg.Port))

	containerID, err := p.docker.CreateContainer(ctx, containerConfig)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to create Redis container")
	}
	p.containerID = containerID

	// Start container
	p.logger.Info("starting Redis container", zap.String("containerID", containerID))
	if err := p.docker.StartContainer(ctx, containerID); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to start Redis container")
	}

	// Wait for Redis to be ready
	p.logger.Info("waiting for Redis to be ready")
	if err := p.waitForReady(ctx); err != nil {
		// Cleanup on failure
		_ = p.Stop(ctx)
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "Redis did not become ready")
	}

	p.logger.Info("Redis service launched successfully",
		zap.String("containerID", containerID),
		zap.String("port", cfg.Port))

	return nil
}

// IsReady checks if the Redis service is ready to accept connections
func (p *RedisPlugin) IsReady(ctx context.Context) (bool, error) {
	if p.containerID == "" {
		return false, gtErrors.New(gtErrors.ErrServiceNotRunning, "Redis container not started")
	}

	// Check if container is running
	running, err := p.docker.IsContainerRunning(ctx, p.containerID)
	if err != nil {
		return false, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to check container status")
	}

	if !running {
		return false, nil
	}

	// Check if Redis is ready by executing redis-cli ping
	cmd := []string{"redis-cli"}
	if p.config.Password != "" {
		cmd = append(cmd, "-a", p.config.Password)
	}
	cmd = append(cmd, "ping")

	output, err := p.docker.ExecInContainer(ctx, p.containerID, &docker.ExecConfig{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})

	if err != nil {
		p.logger.Debug("Redis not ready yet", zap.String("output", output))
		return false, nil
	}

	return true, nil
}

// Stop terminates the Redis service
func (p *RedisPlugin) Stop(ctx context.Context) error {
	// If no containerID, try to find container by labels
	if p.containerID == "" {
		// If no Docker client, nothing to stop
		if p.docker == nil {
			p.logger.Debug("no container ID and no Docker client")
			return nil
		}

		p.logger.Info("no container ID, searching by labels")

		containers, err := p.docker.ListContainersByLabels(ctx, map[string]string{
			"managed-by": "gtool",
			"service":    "redis",
		})

		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list Redis containers")
		}

		if len(containers) == 0 {
			p.logger.Warn("no Redis containers found to stop")
			return nil
		}

		// Stop all matching containers
		for _, container := range containers {
			p.containerID = container.ID
			p.logger.Info("found Redis container",
				zap.String("containerID", container.ID),
				zap.Strings("names", container.Names))

			if err := p.stopContainer(ctx); err != nil {
				p.logger.Error("failed to stop container", zap.Error(err), zap.String("containerID", container.ID))
			}
		}

		return nil
	}

	return p.stopContainer(ctx)
}

// stopContainer stops and removes a specific container
func (p *RedisPlugin) stopContainer(ctx context.Context) error {
	p.logger.Info("stopping Redis service", zap.String("containerID", p.containerID))

	// Stop container
	timeout := 10
	if err := p.docker.StopContainer(ctx, p.containerID, &timeout); err != nil {
		p.logger.Error("failed to stop container", zap.Error(err))
		// Continue to remove anyway
	}

	// Remove container
	if err := p.docker.RemoveContainer(ctx, p.containerID, true); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to remove Redis container")
	}

	p.logger.Info("Redis service stopped successfully")
	p.containerID = ""
	return nil
}

// GetConnectionInfo returns connection details
func (p *RedisPlugin) GetConnectionInfo() (*plugin.ConnectionInfo, error) {
	if p.config == nil {
		return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "Redis service not launched")
	}

	return &plugin.ConnectionInfo{
		Host:     "localhost",
		Port:     mustParsePort(p.config.Port),
		Protocol: "redis",
		Metadata: map[string]string{
			"password": p.config.Password,
		},
	}, nil
}

// GetLogs retrieves service logs
func (p *RedisPlugin) GetLogs(ctx context.Context, opts *plugin.LogOptions) ([]string, error) {
	containerID := p.containerID

	// If no containerID, try to find container by labels
	if containerID == "" {
		// If no Docker client, cannot get logs
		if p.docker == nil {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "no container ID and no Docker client")
		}

		p.logger.Info("no container ID for logs, searching by labels")

		containers, err := p.docker.ListContainersByLabels(ctx, map[string]string{
			"managed-by": "gtool",
			"service":    "redis",
		})

		if err != nil {
			return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list Redis containers")
		}

		if len(containers) == 0 {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "Redis container not found")
		}

		// Find first running container
		var foundContainer *types.Container
		for i := range containers {
			if containers[i].State == "running" {
				foundContainer = &containers[i]
				break
			}
		}

		if foundContainer == nil {
			// Fallback to first container if none are running
			foundContainer = &containers[0]
		}

		containerID = foundContainer.ID
		p.logger.Info("found Redis container for logs",
			zap.String("containerID", containerID),
			zap.String("state", foundContainer.State),
			zap.Strings("names", foundContainer.Names))
	}

	tail := 100
	if opts != nil && opts.Tail > 0 {
		tail = opts.Tail
	}

	logs, err := p.docker.GetContainerLogs(ctx, containerID, tail)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to get container logs")
	}

	// Split logs into lines
	lines := strings.Split(strings.TrimSpace(logs), "\n")
	return lines, nil
}

// parseConfig parses the configuration map into RedisConfig
func (p *RedisPlugin) parseConfig(config map[string]interface{}) (*RedisConfig, error) {
	cfg := &RedisConfig{
		Image:         defaultImage,
		Port:          defaultPort,
		ContainerName: fmt.Sprintf("%s-%d", containerNamePrefix, time.Now().Unix()),
	}

	// Override with provided values
	if image, ok := config["image"].(string); ok && image != "" {
		cfg.Image = image
	}
	if port, ok := config["port"].(string); ok && port != "" {
		cfg.Port = port
	} else if port, ok := config["port"].(float64); ok {
		cfg.Port = fmt.Sprintf("%.0f", port)
	}
	if password, ok := config["password"].(string); ok && password != "" {
		cfg.Password = password
	}
	if containerName, ok := config["container-name"].(string); ok && containerName != "" {
		cfg.ContainerName = containerName
	}

	return cfg, nil
}

// waitForReady waits for Redis to be ready
func (p *RedisPlugin) waitForReady(ctx context.Context) error {
	maxRetries := 30
	interval := 1 * time.Second

	for i := 0; i < maxRetries; i++ {
		ready, err := p.IsReady(ctx)
		if err != nil {
			p.logger.Debug("error checking readiness",
				zap.Error(err),
				zap.Int("attempt", i+1))
		}

		if ready {
			p.logger.Info("Redis is ready", zap.Int("attempts", i+1))
			return nil
		}

		select {
		case <-ctx.Done():
			return gtErrors.New(gtErrors.ErrServiceFailed, "context cancelled while waiting for Redis")
		case <-time.After(interval):
			// Continue to next attempt
		}
	}

	return gtErrors.New(gtErrors.ErrServiceFailed,
		fmt.Sprintf("Redis did not become ready after %d attempts", maxRetries))
}

// mustParsePort parses port string to int, panics on error
func mustParsePort(port string) int {
	var p int
	fmt.Sscanf(port, "%d", &p)
	return p
}
