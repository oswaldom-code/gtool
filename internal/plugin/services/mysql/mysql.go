package mysql

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
	defaultImage        = "mysql:8.4"
	defaultPort         = "3306"
	defaultRootPassword = "root"
	defaultDatabase     = "test"
	containerNamePrefix = "gtool-mysql"
)

// MySQLPlugin implements the ServicePlugin interface for MySQL
type MySQLPlugin struct {
	docker      *docker.Client
	logger      *zap.Logger
	containerID string
	config      *MySQLConfig
}

// MySQLConfig holds MySQL-specific configuration
type MySQLConfig struct {
	Image         string `json:"image"`
	Port          string `json:"port"`
	RootPassword  string `json:"root-password"`
	Database      string `json:"database"`
	User          string `json:"user"`
	Password      string `json:"password"`
	ContainerName string `json:"container-name"`
}

// NewMySQLPlugin creates a new MySQL service plugin
func NewMySQLPlugin(dockerClient *docker.Client, logger *zap.Logger) *MySQLPlugin {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &MySQLPlugin{
		docker: dockerClient,
		logger: logger,
	}
}

// Name returns the service identifier
func (p *MySQLPlugin) Name() string {
	return "mysql"
}

// Launch starts the MySQL service with given configuration
func (p *MySQLPlugin) Launch(ctx context.Context, config map[string]interface{}) error {
	p.logger.Info("launching MySQL service")

	// Parse configuration
	cfg, err := p.parseConfig(config)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to parse MySQL configuration")
	}
	p.config = cfg

	// Pull image
	p.logger.Info("pulling MySQL image", zap.String("image", cfg.Image))
	if err := p.docker.EnsureImage(ctx, cfg.Image); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to pull MySQL image")
	}

	// Build environment
	env := []string{
		fmt.Sprintf("MYSQL_ROOT_PASSWORD=%s", cfg.RootPassword),
		fmt.Sprintf("MYSQL_DATABASE=%s", cfg.Database),
	}
	if cfg.User != "" {
		env = append(env,
			fmt.Sprintf("MYSQL_USER=%s", cfg.User),
			fmt.Sprintf("MYSQL_PASSWORD=%s", cfg.Password),
		)
	}

	// Create container
	containerConfig := &docker.ContainerConfig{
		Image: cfg.Image,
		Name:  cfg.ContainerName,
		Env:   env,
		PortBindings: map[string]string{
			"3306": cfg.Port,
		},
		Labels: map[string]string{
			"managed-by": "gtool",
			"service":    "mysql",
		},
	}

	p.logger.Info("creating MySQL container",
		zap.String("name", cfg.ContainerName),
		zap.String("port", cfg.Port))

	containerID, err := p.docker.CreateContainer(ctx, containerConfig)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to create MySQL container")
	}
	p.containerID = containerID

	// Start container
	p.logger.Info("starting MySQL container", zap.String("containerID", containerID))
	if err := p.docker.StartContainer(ctx, containerID); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to start MySQL container")
	}

	// Wait for MySQL to be ready
	p.logger.Info("waiting for MySQL to be ready")
	if err := p.waitForReady(ctx); err != nil {
		// Cleanup on failure
		_ = p.Stop(ctx)
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "MySQL did not become ready")
	}

	p.logger.Info("MySQL service launched successfully",
		zap.String("containerID", containerID),
		zap.String("port", cfg.Port))

	return nil
}

// IsReady checks if the MySQL service is ready to accept connections
func (p *MySQLPlugin) IsReady(ctx context.Context) (bool, error) {
	if p.containerID == "" {
		return false, gtErrors.New(gtErrors.ErrServiceNotRunning, "MySQL container not started")
	}

	// Check if container is running
	running, err := p.docker.IsContainerRunning(ctx, p.containerID)
	if err != nil {
		return false, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to check container status")
	}

	if !running {
		return false, nil
	}

	// Check if MySQL is ready by executing mysqladmin ping
	output, err := p.docker.ExecInContainer(ctx, p.containerID, &docker.ExecConfig{
		Cmd:          []string{"mysqladmin", "ping", "-uroot", fmt.Sprintf("--password=%s", p.config.RootPassword), "--silent"},
		AttachStdout: true,
		AttachStderr: true,
	})

	if err != nil {
		p.logger.Debug("MySQL not ready yet", zap.String("output", output))
		return false, nil
	}

	return true, nil
}

// Stop terminates the MySQL service
func (p *MySQLPlugin) Stop(ctx context.Context) error {
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
			"service":    "mysql",
		})

		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list MySQL containers")
		}

		if len(containers) == 0 {
			p.logger.Warn("no MySQL containers found to stop")
			return nil
		}

		// Stop all matching containers
		for _, container := range containers {
			p.containerID = container.ID
			p.logger.Info("found MySQL container",
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
func (p *MySQLPlugin) stopContainer(ctx context.Context) error {
	p.logger.Info("stopping MySQL service", zap.String("containerID", p.containerID))

	// Stop container
	timeout := 10
	if err := p.docker.StopContainer(ctx, p.containerID, &timeout); err != nil {
		p.logger.Error("failed to stop container", zap.Error(err))
		// Continue to remove anyway
	}

	// Remove container
	if err := p.docker.RemoveContainer(ctx, p.containerID, true); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to remove MySQL container")
	}

	p.logger.Info("MySQL service stopped successfully")
	p.containerID = ""
	return nil
}

// GetConnectionInfo returns connection details
func (p *MySQLPlugin) GetConnectionInfo() (*plugin.ConnectionInfo, error) {
	if p.config == nil {
		return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "MySQL service not launched")
	}

	user := "root"
	password := p.config.RootPassword
	if p.config.User != "" {
		user = p.config.User
		password = p.config.Password
	}

	return &plugin.ConnectionInfo{
		Host:     "localhost",
		Port:     mustParsePort(p.config.Port),
		Protocol: "mysql",
		Metadata: map[string]string{
			"user":     user,
			"password": password,
			"database": p.config.Database,
		},
	}, nil
}

// GetLogs retrieves service logs
func (p *MySQLPlugin) GetLogs(ctx context.Context, opts *plugin.LogOptions) ([]string, error) {
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
			"service":    "mysql",
		})

		if err != nil {
			return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list MySQL containers")
		}

		if len(containers) == 0 {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "MySQL container not found")
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
		p.logger.Info("found MySQL container for logs",
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

// parseConfig parses the configuration map into MySQLConfig
func (p *MySQLPlugin) parseConfig(config map[string]interface{}) (*MySQLConfig, error) {
	cfg := &MySQLConfig{
		Image:         defaultImage,
		Port:          defaultPort,
		RootPassword:  defaultRootPassword,
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
	if rootPassword, ok := config["root-password"].(string); ok && rootPassword != "" {
		cfg.RootPassword = rootPassword
	}
	if database, ok := config["database"].(string); ok && database != "" {
		cfg.Database = database
	}
	if user, ok := config["user"].(string); ok && user != "" {
		cfg.User = user
	}
	if password, ok := config["password"].(string); ok && password != "" {
		cfg.Password = password
	}
	if containerName, ok := config["container-name"].(string); ok && containerName != "" {
		cfg.ContainerName = containerName
	}

	return cfg, nil
}

// waitForReady waits for MySQL to be ready
func (p *MySQLPlugin) waitForReady(ctx context.Context) error {
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
			p.logger.Info("MySQL is ready", zap.Int("attempts", i+1))
			return nil
		}

		select {
		case <-ctx.Done():
			return gtErrors.New(gtErrors.ErrServiceFailed, "context cancelled while waiting for MySQL")
		case <-time.After(interval):
			// Continue to next attempt
		}
	}

	return gtErrors.New(gtErrors.ErrServiceFailed,
		fmt.Sprintf("MySQL did not become ready after %d attempts", maxRetries))
}

// mustParsePort parses port string to int, panics on error
func mustParsePort(port string) int {
	var p int
	fmt.Sscanf(port, "%d", &p)
	return p
}
