package minio

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/internal/plugin"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
	"go.uber.org/zap"
)

const (
	defaultImage        = "minio/minio:latest"
	defaultPort         = "9000"
	defaultConsolePort  = "9001"
	defaultAccessKey    = "minioadmin"
	defaultSecretKey    = "minioadmin"
	containerNamePrefix = "gtool-minio"
)

// MinIOPlugin implements the ServicePlugin interface for MinIO (S3-compatible storage)
type MinIOPlugin struct {
	docker      *docker.Client
	logger      *zap.Logger
	containerID string
	config      *MinIOConfig
}

// MinIOConfig holds MinIO-specific configuration
type MinIOConfig struct {
	Image         string `json:"image"`
	Port          string `json:"port"`
	ConsolePort   string `json:"console-port"`
	AccessKey     string `json:"access-key"`
	SecretKey     string `json:"secret-key"`
	ContainerName string `json:"container-name"`
}

// NewMinIOPlugin creates a new MinIO service plugin
func NewMinIOPlugin(dockerClient *docker.Client, logger *zap.Logger) *MinIOPlugin {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &MinIOPlugin{
		docker: dockerClient,
		logger: logger,
	}
}

// Name returns the service identifier
func (p *MinIOPlugin) Name() string {
	return "minio"
}

// Launch starts the MinIO service with given configuration
func (p *MinIOPlugin) Launch(ctx context.Context, config map[string]interface{}) error {
	p.logger.Info("launching MinIO service")

	// Parse configuration
	cfg, err := p.parseConfig(config)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to parse MinIO configuration")
	}
	p.config = cfg

	// Pull image
	p.logger.Info("pulling MinIO image", zap.String("image", cfg.Image))
	if err := p.docker.EnsureImage(ctx, cfg.Image); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to pull MinIO image")
	}

	// Create container
	containerConfig := &docker.ContainerConfig{
		Image: cfg.Image,
		Name:  cfg.ContainerName,
		Env: []string{
			fmt.Sprintf("MINIO_ROOT_USER=%s", cfg.AccessKey),
			fmt.Sprintf("MINIO_ROOT_PASSWORD=%s", cfg.SecretKey),
		},
		Cmd: []string{"server", "/data", "--console-address", ":9001"},
		PortBindings: map[string]string{
			"9000": cfg.Port,
			"9001": cfg.ConsolePort,
		},
		Labels: map[string]string{
			"managed-by": "gtool",
			"service":    "minio",
		},
	}

	p.logger.Info("creating MinIO container",
		zap.String("name", cfg.ContainerName),
		zap.String("port", cfg.Port),
		zap.String("consolePort", cfg.ConsolePort))

	containerID, err := p.docker.CreateContainer(ctx, containerConfig)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to create MinIO container")
	}
	p.containerID = containerID

	// Start container
	p.logger.Info("starting MinIO container", zap.String("containerID", containerID))
	if err := p.docker.StartContainer(ctx, containerID); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to start MinIO container")
	}

	// Wait for MinIO to be ready
	p.logger.Info("waiting for MinIO to be ready")
	if err := p.waitForReady(ctx); err != nil {
		// Cleanup on failure
		_ = p.Stop(ctx)
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "MinIO did not become ready")
	}

	p.logger.Info("MinIO service launched successfully",
		zap.String("containerID", containerID),
		zap.String("port", cfg.Port))

	return nil
}

// IsReady checks if the MinIO service is ready to accept connections
func (p *MinIOPlugin) IsReady(ctx context.Context) (bool, error) {
	if p.containerID == "" {
		return false, gtErrors.New(gtErrors.ErrServiceNotRunning, "MinIO container not started")
	}

	// The minio/minio image does not reliably bundle curl or the mc client, so
	// we cannot probe the health endpoint via ExecInContainer. Readiness is
	// therefore limited to checking that the container is running (weak
	// readiness, improvable in the future).
	running, err := p.docker.IsContainerRunning(ctx, p.containerID)
	if err != nil {
		return false, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to check container status")
	}

	return running, nil
}

// Stop terminates the MinIO service
func (p *MinIOPlugin) Stop(ctx context.Context) error {
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
			"service":    "minio",
		})

		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list MinIO containers")
		}

		if len(containers) == 0 {
			p.logger.Warn("no MinIO containers found to stop")
			return nil
		}

		// Stop all matching containers
		for _, container := range containers {
			p.containerID = container.ID
			p.logger.Info("found MinIO container",
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
func (p *MinIOPlugin) stopContainer(ctx context.Context) error {
	p.logger.Info("stopping MinIO service", zap.String("containerID", p.containerID))

	// Stop container
	timeout := 10
	if err := p.docker.StopContainer(ctx, p.containerID, &timeout); err != nil {
		p.logger.Error("failed to stop container", zap.Error(err))
		// Continue to remove anyway
	}

	// Remove container
	if err := p.docker.RemoveContainer(ctx, p.containerID, true); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to remove MinIO container")
	}

	p.logger.Info("MinIO service stopped successfully")
	p.containerID = ""
	return nil
}

// GetConnectionInfo returns connection details
func (p *MinIOPlugin) GetConnectionInfo() (*plugin.ConnectionInfo, error) {
	if p.config == nil {
		return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "MinIO service not launched")
	}

	return &plugin.ConnectionInfo{
		Host:     "localhost",
		Port:     mustParsePort(p.config.Port),
		Protocol: "s3",
		Metadata: map[string]string{
			"accessKey":   p.config.AccessKey,
			"secretKey":   p.config.SecretKey,
			"endpoint":    fmt.Sprintf("http://localhost:%s", p.config.Port),
			"consolePort": p.config.ConsolePort,
		},
	}, nil
}

// GetLogs retrieves service logs
func (p *MinIOPlugin) GetLogs(ctx context.Context, opts *plugin.LogOptions) ([]string, error) {
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
			"service":    "minio",
		})

		if err != nil {
			return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list MinIO containers")
		}

		if len(containers) == 0 {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "MinIO container not found")
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
		p.logger.Info("found MinIO container for logs",
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

// parseConfig parses the configuration map into MinIOConfig
func (p *MinIOPlugin) parseConfig(config map[string]interface{}) (*MinIOConfig, error) {
	cfg := &MinIOConfig{
		Image:         defaultImage,
		Port:          defaultPort,
		ConsolePort:   defaultConsolePort,
		AccessKey:     defaultAccessKey,
		SecretKey:     defaultSecretKey,
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
	if consolePort, ok := config["console-port"].(string); ok && consolePort != "" {
		cfg.ConsolePort = consolePort
	} else if consolePort, ok := config["console-port"].(float64); ok {
		cfg.ConsolePort = fmt.Sprintf("%.0f", consolePort)
	}
	if accessKey, ok := config["access-key"].(string); ok && accessKey != "" {
		cfg.AccessKey = accessKey
	}
	if secretKey, ok := config["secret-key"].(string); ok && secretKey != "" {
		cfg.SecretKey = secretKey
	}
	if containerName, ok := config["container-name"].(string); ok && containerName != "" {
		cfg.ContainerName = containerName
	}

	return cfg, nil
}

// waitForReady waits for MinIO to be ready
func (p *MinIOPlugin) waitForReady(ctx context.Context) error {
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
			p.logger.Info("MinIO is ready", zap.Int("attempts", i+1))
			return nil
		}

		select {
		case <-ctx.Done():
			return gtErrors.New(gtErrors.ErrServiceFailed, "context cancelled while waiting for MinIO")
		case <-time.After(interval):
			// Continue to next attempt
		}
	}

	return gtErrors.New(gtErrors.ErrServiceFailed,
		fmt.Sprintf("MinIO did not become ready after %d attempts", maxRetries))
}

// mustParsePort parses port string to int, panics on error
func mustParsePort(port string) int {
	var p int
	fmt.Sscanf(port, "%d", &p)
	return p
}
