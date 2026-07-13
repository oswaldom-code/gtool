package mongodb

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
	defaultImage        = "mongo:7"
	defaultPort         = "27017"
	defaultDatabase     = "test"
	containerNamePrefix = "gtool-mongodb"
)

// MongoDBPlugin implements the ServicePlugin interface for MongoDB
type MongoDBPlugin struct {
	docker      *docker.Client
	logger      *zap.Logger
	containerID string
	config      *MongoDBConfig
}

// MongoDBConfig holds MongoDB-specific configuration
type MongoDBConfig struct {
	Image         string `json:"image"`
	Port          string `json:"port"`
	Database      string `json:"database"`
	ContainerName string `json:"container-name"`
}

// NewMongoDBPlugin creates a new MongoDB service plugin
func NewMongoDBPlugin(dockerClient *docker.Client, logger *zap.Logger) *MongoDBPlugin {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &MongoDBPlugin{
		docker: dockerClient,
		logger: logger,
	}
}

// Name returns the service identifier
func (p *MongoDBPlugin) Name() string {
	return "mongodb"
}

// Launch starts the MongoDB service with given configuration
func (p *MongoDBPlugin) Launch(ctx context.Context, config map[string]interface{}) error {
	p.logger.Info("launching MongoDB service")

	// Parse configuration
	cfg, err := p.parseConfig(config)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to parse MongoDB configuration")
	}
	p.config = cfg

	// Pull image
	p.logger.Info("pulling MongoDB image", zap.String("image", cfg.Image))
	if err := p.docker.EnsureImage(ctx, cfg.Image); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to pull MongoDB image")
	}

	// Create container
	containerConfig := &docker.ContainerConfig{
		Image: cfg.Image,
		Name:  cfg.ContainerName,
		PortBindings: map[string]string{
			"27017": cfg.Port,
		},
		Labels: map[string]string{
			"managed-by": "gtool",
			"service":    "mongodb",
		},
	}

	p.logger.Info("creating MongoDB container",
		zap.String("name", cfg.ContainerName),
		zap.String("port", cfg.Port))

	containerID, err := p.docker.CreateContainer(ctx, containerConfig)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to create MongoDB container")
	}
	p.containerID = containerID

	// Start container
	p.logger.Info("starting MongoDB container", zap.String("containerID", containerID))
	if err := p.docker.StartContainer(ctx, containerID); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to start MongoDB container")
	}

	// Wait for MongoDB to be ready
	p.logger.Info("waiting for MongoDB to be ready")
	if err := p.waitForReady(ctx); err != nil {
		// Cleanup on failure
		_ = p.Stop(ctx)
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "MongoDB did not become ready")
	}

	p.logger.Info("MongoDB service launched successfully",
		zap.String("containerID", containerID),
		zap.String("port", cfg.Port))

	return nil
}

// IsReady checks if the MongoDB service is ready to accept connections
func (p *MongoDBPlugin) IsReady(ctx context.Context) (bool, error) {
	if p.containerID == "" {
		return false, gtErrors.New(gtErrors.ErrServiceNotRunning, "MongoDB container not started")
	}

	// Check if container is running
	running, err := p.docker.IsContainerRunning(ctx, p.containerID)
	if err != nil {
		return false, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to check container status")
	}

	if !running {
		return false, nil
	}

	// Check if MongoDB is ready by pinging via mongosh
	output, err := p.docker.ExecInContainer(ctx, p.containerID, &docker.ExecConfig{
		Cmd:          []string{"mongosh", "--quiet", "--eval", "db.runCommand({ ping: 1 })"},
		AttachStdout: true,
		AttachStderr: true,
	})

	if err != nil {
		p.logger.Debug("MongoDB not ready yet", zap.String("output", output))
		return false, nil
	}

	return true, nil
}

// Stop terminates the MongoDB service
func (p *MongoDBPlugin) Stop(ctx context.Context) error {
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
			"service":    "mongodb",
		})

		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list MongoDB containers")
		}

		if len(containers) == 0 {
			p.logger.Warn("no MongoDB containers found to stop")
			return nil
		}

		// Stop all matching containers
		for _, container := range containers {
			p.containerID = container.ID
			p.logger.Info("found MongoDB container",
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
func (p *MongoDBPlugin) stopContainer(ctx context.Context) error {
	p.logger.Info("stopping MongoDB service", zap.String("containerID", p.containerID))

	// Stop container
	timeout := 10
	if err := p.docker.StopContainer(ctx, p.containerID, &timeout); err != nil {
		p.logger.Error("failed to stop container", zap.Error(err))
		// Continue to remove anyway
	}

	// Remove container
	if err := p.docker.RemoveContainer(ctx, p.containerID, true); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to remove MongoDB container")
	}

	p.logger.Info("MongoDB service stopped successfully")
	p.containerID = ""
	return nil
}

// GetConnectionInfo returns connection details
func (p *MongoDBPlugin) GetConnectionInfo() (*plugin.ConnectionInfo, error) {
	if p.config == nil {
		return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "MongoDB service not launched")
	}

	return &plugin.ConnectionInfo{
		Host:     "localhost",
		Port:     mustParsePort(p.config.Port),
		Protocol: "mongodb",
		Metadata: map[string]string{
			"database":         p.config.Database,
			"connectionString": fmt.Sprintf("mongodb://localhost:%s", p.config.Port),
		},
	}, nil
}

// GetLogs retrieves service logs
func (p *MongoDBPlugin) GetLogs(ctx context.Context, opts *plugin.LogOptions) ([]string, error) {
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
			"service":    "mongodb",
		})

		if err != nil {
			return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list MongoDB containers")
		}

		if len(containers) == 0 {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "MongoDB container not found")
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
		p.logger.Info("found MongoDB container for logs",
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

// parseConfig parses the configuration map into MongoDBConfig
func (p *MongoDBPlugin) parseConfig(config map[string]interface{}) (*MongoDBConfig, error) {
	cfg := &MongoDBConfig{
		Image:         defaultImage,
		Port:          defaultPort,
		Database:      defaultDatabase,
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
	if database, ok := config["database"].(string); ok && database != "" {
		cfg.Database = database
	}
	if containerName, ok := config["container-name"].(string); ok && containerName != "" {
		cfg.ContainerName = containerName
	}

	return cfg, nil
}

// waitForReady waits for MongoDB to be ready
func (p *MongoDBPlugin) waitForReady(ctx context.Context) error {
	maxRetries := 60
	interval := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		ready, err := p.IsReady(ctx)
		if err != nil {
			p.logger.Debug("error checking readiness",
				zap.Error(err),
				zap.Int("attempt", i+1))
		}

		if ready {
			p.logger.Info("MongoDB is ready", zap.Int("attempts", i+1))
			return nil
		}

		select {
		case <-ctx.Done():
			return gtErrors.New(gtErrors.ErrServiceFailed, "context cancelled while waiting for MongoDB")
		case <-time.After(interval):
			// Continue to next attempt
		}
	}

	return gtErrors.New(gtErrors.ErrServiceFailed,
		fmt.Sprintf("MongoDB did not become ready after %d attempts", maxRetries))
}

// mustParsePort parses port string to int, panics on error
func mustParsePort(port string) int {
	var p int
	fmt.Sscanf(port, "%d", &p)
	return p
}
