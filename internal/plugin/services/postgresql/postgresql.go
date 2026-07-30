package postgresql

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/internal/plugin"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
	"go.uber.org/zap"
)

const (
	defaultImage        = "postgres:16-alpine"
	defaultPort         = "5432"
	defaultUser         = "postgres"
	defaultPassword     = "postgres"
	defaultDatabase     = "postgres"
	containerNamePrefix = "gtool-postgresql"
)

// PostgreSQLPlugin implements the ServicePlugin interface for PostgreSQL
type PostgreSQLPlugin struct {
	docker      *docker.Client
	logger      *zap.Logger
	containerID string
	config      *PostgreSQLConfig
}

// PostgreSQLConfig holds PostgreSQL-specific configuration
type PostgreSQLConfig struct {
	Image         string `json:"image"`
	Port          string `json:"port"`
	User          string `json:"user"`
	Password      string `json:"password"`
	Database      string `json:"database"`
	ScriptsPath   string `json:"scripts-path"`
	ContainerName string `json:"container-name"`
}

// NewPostgreSQLPlugin creates a new PostgreSQL service plugin
func NewPostgreSQLPlugin(dockerClient *docker.Client, logger *zap.Logger) *PostgreSQLPlugin {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &PostgreSQLPlugin{
		docker: dockerClient,
		logger: logger,
	}
}

// Name returns the service identifier
func (p *PostgreSQLPlugin) Name() string {
	return "postgresql"
}

// Launch starts the PostgreSQL service with given configuration
func (p *PostgreSQLPlugin) Launch(ctx context.Context, config map[string]interface{}) error {
	p.logger.Info("launching PostgreSQL service")

	// Parse configuration
	cfg, err := p.parseConfig(config)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to parse PostgreSQL configuration")
	}
	p.config = cfg

	// Pull image
	p.logger.Info("pulling PostgreSQL image", zap.String("image", cfg.Image))
	if err := p.docker.EnsureImage(ctx, cfg.Image); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to pull PostgreSQL image")
	}

	// Create container
	containerConfig := &docker.ContainerConfig{
		Image: cfg.Image,
		Name:  cfg.ContainerName,
		Env: []string{
			fmt.Sprintf("POSTGRES_USER=%s", cfg.User),
			fmt.Sprintf("POSTGRES_PASSWORD=%s", cfg.Password),
			fmt.Sprintf("POSTGRES_DB=%s", cfg.Database),
		},
		PortBindings: map[string]string{
			"5432": cfg.Port,
		},
		Labels: map[string]string{
			"managed-by": "gtool",
			"service":    "postgresql",
		},
	}

	p.logger.Info("creating PostgreSQL container",
		zap.String("name", cfg.ContainerName),
		zap.String("port", cfg.Port))

	containerID, err := p.docker.CreateContainer(ctx, containerConfig)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to create PostgreSQL container")
	}
	p.containerID = containerID

	// Start container
	p.logger.Info("starting PostgreSQL container", zap.String("containerID", containerID))
	if err := p.docker.StartContainer(ctx, containerID); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to start PostgreSQL container")
	}

	// Wait for PostgreSQL to be ready
	p.logger.Info("waiting for PostgreSQL to be ready")
	if err := p.waitForReady(ctx); err != nil {
		// Cleanup on failure
		_ = p.Stop(ctx)
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "PostgreSQL did not become ready")
	}

	// Execute SQL scripts if provided
	if cfg.ScriptsPath != "" {
		p.logger.Info("executing SQL scripts", zap.String("path", cfg.ScriptsPath))
		if err := p.executeScripts(ctx, cfg.ScriptsPath); err != nil {
			// Don't fail if scripts fail, just log the error
			p.logger.Error("failed to execute SQL scripts",
				zap.Error(err),
				zap.String("path", cfg.ScriptsPath))
		}
	}

	p.logger.Info("PostgreSQL service launched successfully",
		zap.String("containerID", containerID),
		zap.String("port", cfg.Port))

	return nil
}

// IsReady checks if the PostgreSQL service is ready to accept connections
func (p *PostgreSQLPlugin) IsReady(ctx context.Context) (bool, error) {
	if p.containerID == "" {
		return false, gtErrors.New(gtErrors.ErrServiceNotRunning, "PostgreSQL container not started")
	}

	// Check if container is running
	running, err := p.docker.IsContainerRunning(ctx, p.containerID)
	if err != nil {
		return false, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to check container status")
	}

	if !running {
		return false, nil
	}

	// Check if PostgreSQL is ready by executing pg_isready
	output, err := p.docker.ExecInContainer(ctx, p.containerID, &docker.ExecConfig{
		Cmd:          []string{"pg_isready", "-U", p.config.User, "-d", p.config.Database},
		AttachStdout: true,
		AttachStderr: true,
	})

	if err != nil {
		p.logger.Debug("PostgreSQL not ready yet", zap.String("output", output))
		return false, nil
	}

	return true, nil
}

// Stop terminates the PostgreSQL service
func (p *PostgreSQLPlugin) Stop(ctx context.Context) error {
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
			"service":    "postgresql",
		})

		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list PostgreSQL containers")
		}

		if len(containers) == 0 {
			p.logger.Warn("no PostgreSQL containers found to stop")
			return nil
		}

		// Stop all matching containers
		for _, container := range containers {
			p.containerID = container.ID
			p.logger.Info("found PostgreSQL container",
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
func (p *PostgreSQLPlugin) stopContainer(ctx context.Context) error {
	p.logger.Info("stopping PostgreSQL service", zap.String("containerID", p.containerID))

	// Stop container
	timeout := 10
	if err := p.docker.StopContainer(ctx, p.containerID, &timeout); err != nil {
		p.logger.Error("failed to stop container", zap.Error(err))
		// Continue to remove anyway
	}

	// Remove container
	if err := p.docker.RemoveContainer(ctx, p.containerID, true); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to remove PostgreSQL container")
	}

	p.logger.Info("PostgreSQL service stopped successfully")
	p.containerID = ""
	return nil
}

// GetConnectionInfo returns connection details
func (p *PostgreSQLPlugin) GetConnectionInfo() (*plugin.ConnectionInfo, error) {
	if p.config == nil {
		return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "PostgreSQL service not launched")
	}

	return &plugin.ConnectionInfo{
		Host:     "localhost",
		Port:     mustParsePort(p.config.Port),
		Protocol: "postgresql",
		Metadata: map[string]string{
			"user":     p.config.User,
			"password": p.config.Password,
			"database": p.config.Database,
			"sslmode":  "disable",
		},
	}, nil
}

// GetLogs retrieves service logs
func (p *PostgreSQLPlugin) GetLogs(ctx context.Context, opts *plugin.LogOptions) ([]string, error) {
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
			"service":    "postgresql",
		})

		if err != nil {
			return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list PostgreSQL containers")
		}

		if len(containers) == 0 {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "PostgreSQL container not found")
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
		p.logger.Info("found PostgreSQL container for logs",
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

// parseConfig parses the configuration map into PostgreSQLConfig
func (p *PostgreSQLPlugin) parseConfig(config map[string]interface{}) (*PostgreSQLConfig, error) {
	cfg := &PostgreSQLConfig{
		Image:         defaultImage,
		Port:          defaultPort,
		User:          defaultUser,
		Password:      defaultPassword,
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
	if user, ok := config["user"].(string); ok && user != "" {
		cfg.User = user
	}
	if password, ok := config["password"].(string); ok && password != "" {
		cfg.Password = password
	}
	if database, ok := config["database"].(string); ok && database != "" {
		cfg.Database = database
	}
	if scriptsPath, ok := config["scripts-path"].(string); ok && scriptsPath != "" {
		cfg.ScriptsPath = scriptsPath
	}
	if containerName, ok := config["container-name"].(string); ok && containerName != "" {
		cfg.ContainerName = containerName
	}

	return cfg, nil
}

// waitForReady waits for PostgreSQL to be ready
func (p *PostgreSQLPlugin) waitForReady(ctx context.Context) error {
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
			p.logger.Info("PostgreSQL is ready", zap.Int("attempts", i+1))
			return nil
		}

		select {
		case <-ctx.Done():
			return gtErrors.New(gtErrors.ErrServiceFailed, "context cancelled while waiting for PostgreSQL")
		case <-time.After(interval):
			// Continue to next attempt
		}
	}

	return gtErrors.New(gtErrors.ErrServiceFailed,
		fmt.Sprintf("PostgreSQL did not become ready after %d attempts", maxRetries))
}

// executeScripts executes SQL scripts from the specified directory
func (p *PostgreSQLPlugin) executeScripts(ctx context.Context, scriptsPath string) error {
	// Check if scripts path exists
	if _, err := os.Stat(scriptsPath); os.IsNotExist(err) {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid,
			fmt.Sprintf("scripts path does not exist: %s", scriptsPath))
	}

	// Get all .sql files
	sqlFiles, err := filepath.Glob(filepath.Join(scriptsPath, "*.sql"))
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to list SQL files")
	}

	if len(sqlFiles) == 0 {
		p.logger.Warn("no SQL files found in scripts path", zap.String("path", scriptsPath))
		return nil
	}

	p.logger.Info("found SQL files to execute",
		zap.Int("count", len(sqlFiles)),
		zap.Strings("files", sqlFiles))

	// Execute each SQL file
	for _, sqlFile := range sqlFiles {
		if err := p.executeScript(ctx, sqlFile); err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
				fmt.Sprintf("failed to execute script: %s", sqlFile))
		}
	}

	return nil
}

// executeScript executes a single SQL script
func (p *PostgreSQLPlugin) executeScript(ctx context.Context, scriptPath string) error {
	p.logger.Info("executing SQL script", zap.String("script", scriptPath))

	// Read the script file
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to read SQL script")
	}

	// Execute using psql
	cmd := []string{
		"psql",
		"-U", p.config.User,
		"-d", p.config.Database,
		"-c", string(content),
	}

	output, err := p.docker.ExecInContainer(ctx, p.containerID, &docker.ExecConfig{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		Env: []string{
			fmt.Sprintf("PGPASSWORD=%s", p.config.Password),
		},
	})

	if err != nil {
		p.logger.Error("failed to execute SQL script",
			zap.Error(err),
			zap.String("script", scriptPath),
			zap.String("output", output))
		return err
	}

	p.logger.Info("SQL script executed successfully",
		zap.String("script", scriptPath),
		zap.String("output", strings.TrimSpace(output)))

	return nil
}

// mustParsePort parses port string to int, panics on error
func mustParsePort(port string) int {
	var p int
	fmt.Sscanf(port, "%d", &p)
	return p
}
