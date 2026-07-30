package gcs

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/internal/plugin"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
	"go.uber.org/zap"
)

const (
	defaultImage        = "fsouza/fake-gcs-server:1.49.0"
	defaultPort         = "4443"
	defaultProjectID    = "test-project"
	containerNamePrefix = "gtool-gcs"
)

// GCSPlugin implements the ServicePlugin interface for the fake-gcs-server
// Google Cloud Storage emulator.
type GCSPlugin struct {
	docker      *docker.Client
	logger      *zap.Logger
	httpClient  *http.Client
	containerID string
	config      *GCSConfig
}

// GCSConfig holds GCS-specific configuration
type GCSConfig struct {
	Image         string   `json:"image"`
	Port          string   `json:"port"`
	ProjectID     string   `json:"project-id"`
	Buckets       []string `json:"buckets"`
	ContainerName string   `json:"container-name"`
}

// NewGCSPlugin creates a new GCS service plugin
func NewGCSPlugin(dockerClient *docker.Client, logger *zap.Logger) *GCSPlugin {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &GCSPlugin{
		docker:     dockerClient,
		logger:     logger,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Name returns the service identifier
func (p *GCSPlugin) Name() string {
	return "gcs"
}

// Launch starts the GCS emulator with given configuration
func (p *GCSPlugin) Launch(ctx context.Context, config map[string]interface{}) error {
	p.logger.Info("launching GCS emulator")

	cfg, err := p.parseConfig(config)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to parse GCS configuration")
	}
	p.config = cfg

	p.logger.Info("pulling GCS image", zap.String("image", cfg.Image))
	if err := p.docker.PullImage(ctx, cfg.Image); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to pull GCS image")
	}

	containerConfig := &docker.ContainerConfig{
		Image: cfg.Image,
		Name:  cfg.ContainerName,
		Cmd:   buildCmd(cfg),
		PortBindings: map[string]string{
			"4443": cfg.Port,
		},
		Labels: map[string]string{
			"managed-by": "gtool",
			"service":    "gcs",
		},
	}

	p.logger.Info("creating GCS container",
		zap.String("name", cfg.ContainerName),
		zap.String("port", cfg.Port))

	containerID, err := p.docker.CreateContainer(ctx, containerConfig)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to create GCS container")
	}
	p.containerID = containerID

	p.logger.Info("starting GCS container", zap.String("containerID", containerID))
	if err := p.docker.StartContainer(ctx, containerID); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to start GCS container")
	}

	p.logger.Info("waiting for GCS emulator to be ready")
	if err := p.waitForReady(ctx); err != nil {
		// Cleanup on failure
		_ = p.Stop(ctx)
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "GCS emulator did not become ready")
	}

	// Create the configured buckets
	if len(cfg.Buckets) > 0 {
		p.logger.Info("creating buckets", zap.Strings("buckets", cfg.Buckets))
		if err := p.createBuckets(ctx, cfg); err != nil {
			// Don't fail the launch if bucket creation fails, just log the error
			p.logger.Error("failed to create buckets", zap.Error(err))
		}
	}

	p.logger.Info("GCS emulator launched successfully",
		zap.String("containerID", containerID),
		zap.String("port", cfg.Port))

	return nil
}

// IsReady checks if the GCS emulator REST API is accepting requests
func (p *GCSPlugin) IsReady(ctx context.Context) (bool, error) {
	if p.containerID == "" {
		return false, gtErrors.New(gtErrors.ErrServiceNotRunning, "GCS container not started")
	}

	running, err := p.docker.IsContainerRunning(ctx, p.containerID)
	if err != nil {
		return false, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to check container status")
	}

	if !running {
		return false, nil
	}

	// Listing buckets succeeds only once the emulator is serving requests
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.bucketsURL(), nil)
	if err != nil {
		return false, gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to build readiness request")
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Debug("GCS not ready yet", zap.Error(err))
		return false, nil
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// Stop terminates the GCS service
func (p *GCSPlugin) Stop(ctx context.Context) error {
	if p.containerID == "" {
		if p.docker == nil {
			p.logger.Debug("no container ID and no Docker client")
			return nil
		}

		p.logger.Info("no container ID, searching by labels")

		containers, err := p.docker.ListContainersByLabels(ctx, map[string]string{
			"managed-by": "gtool",
			"service":    "gcs",
		})

		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list GCS containers")
		}

		if len(containers) == 0 {
			p.logger.Warn("no GCS containers found to stop")
			return nil
		}

		for _, container := range containers {
			p.containerID = container.ID
			p.logger.Info("found GCS container",
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
func (p *GCSPlugin) stopContainer(ctx context.Context) error {
	p.logger.Info("stopping GCS service", zap.String("containerID", p.containerID))

	timeout := 10
	if err := p.docker.StopContainer(ctx, p.containerID, &timeout); err != nil {
		p.logger.Error("failed to stop container", zap.Error(err))
		// Continue to remove anyway
	}

	if err := p.docker.RemoveContainer(ctx, p.containerID, true); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to remove GCS container")
	}

	p.logger.Info("GCS service stopped successfully")
	p.containerID = ""
	return nil
}

// GetConnectionInfo returns connection details
func (p *GCSPlugin) GetConnectionInfo() (*plugin.ConnectionInfo, error) {
	if p.config == nil {
		return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "GCS service not launched")
	}

	return &plugin.ConnectionInfo{
		Host:     "localhost",
		Port:     mustParsePort(p.config.Port),
		Protocol: "http",
		Metadata: map[string]string{
			"storage-emulator-host": fmt.Sprintf("http://localhost:%s", p.config.Port),
			"project-id":            p.config.ProjectID,
		},
	}, nil
}

// GetLogs retrieves service logs
func (p *GCSPlugin) GetLogs(ctx context.Context, opts *plugin.LogOptions) ([]string, error) {
	containerID := p.containerID

	if containerID == "" {
		if p.docker == nil {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "no container ID and no Docker client")
		}

		p.logger.Info("no container ID for logs, searching by labels")

		containers, err := p.docker.ListContainersByLabels(ctx, map[string]string{
			"managed-by": "gtool",
			"service":    "gcs",
		})

		if err != nil {
			return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list GCS containers")
		}

		if len(containers) == 0 {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "GCS container not found")
		}

		var foundContainer *types.Container
		for i := range containers {
			if containers[i].State == "running" {
				foundContainer = &containers[i]
				break
			}
		}

		if foundContainer == nil {
			foundContainer = &containers[0]
		}

		containerID = foundContainer.ID
		p.logger.Info("found GCS container for logs",
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

	lines := strings.Split(strings.TrimSpace(logs), "\n")
	return lines, nil
}

// parseConfig parses the configuration map into GCSConfig
func (p *GCSPlugin) parseConfig(config map[string]interface{}) (*GCSConfig, error) {
	cfg := &GCSConfig{
		Image:         defaultImage,
		Port:          defaultPort,
		ProjectID:     defaultProjectID,
		ContainerName: fmt.Sprintf("%s-%d", containerNamePrefix, time.Now().Unix()),
	}

	if image, ok := config["image"].(string); ok && image != "" {
		cfg.Image = image
	}
	if port, ok := config["port"].(string); ok && port != "" {
		cfg.Port = port
	} else if port, ok := config["port"].(float64); ok {
		cfg.Port = fmt.Sprintf("%.0f", port)
	}
	if projectID, ok := config["project-id"].(string); ok && projectID != "" {
		cfg.ProjectID = projectID
	}
	if buckets, ok := config["buckets"].([]interface{}); ok {
		for _, b := range buckets {
			if name, ok := b.(string); ok && name != "" {
				cfg.Buckets = append(cfg.Buckets, name)
			}
		}
	}
	if containerName, ok := config["container-name"].(string); ok && containerName != "" {
		cfg.ContainerName = containerName
	}

	return cfg, nil
}

// waitForReady waits for the GCS emulator to start serving requests
func (p *GCSPlugin) waitForReady(ctx context.Context) error {
	maxRetries := 30
	interval := time.Second

	for i := 0; i < maxRetries; i++ {
		ready, err := p.IsReady(ctx)
		if err != nil {
			p.logger.Debug("error checking readiness",
				zap.Error(err),
				zap.Int("attempt", i+1))
		}

		if ready {
			p.logger.Info("GCS emulator is ready", zap.Int("attempts", i+1))
			return nil
		}

		select {
		case <-ctx.Done():
			return gtErrors.New(gtErrors.ErrServiceFailed, "context cancelled while waiting for GCS")
		case <-time.After(interval):
			// Continue to next attempt
		}
	}

	return gtErrors.New(gtErrors.ErrServiceFailed,
		fmt.Sprintf("GCS emulator did not become ready after %d attempts", maxRetries))
}

// createBuckets creates the configured buckets via the GCS JSON API
func (p *GCSPlugin) createBuckets(ctx context.Context, cfg *GCSConfig) error {
	for _, bucket := range cfg.Buckets {
		body := []byte(fmt.Sprintf(`{"name":%q}`, bucket))

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.bucketsURL(), bytes.NewReader(body))
		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to build bucket request")
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := p.httpClient.Do(req)
		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
				fmt.Sprintf("failed to create bucket: %s", bucket))
		}
		ok := resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusConflict
		resp.Body.Close()

		if !ok {
			return gtErrors.New(gtErrors.ErrServiceFailed,
				fmt.Sprintf("unexpected status %d creating bucket %s", resp.StatusCode, bucket))
		}

		p.logger.Info("bucket created", zap.String("bucket", bucket))
	}
	return nil
}

// bucketsURL returns the GCS JSON API buckets endpoint for the project
func (p *GCSPlugin) bucketsURL() string {
	return fmt.Sprintf("http://localhost:%s/storage/v1/b?project=%s", p.config.Port, p.config.ProjectID)
}

// buildCmd builds the fake-gcs-server startup command. external-url and
// public-host are set to the host-facing address so object URLs resolve for
// clients running on the host.
func buildCmd(cfg *GCSConfig) []string {
	hostURL := fmt.Sprintf("http://localhost:%s", cfg.Port)
	return []string{
		"-scheme", "http",
		"-host", "0.0.0.0",
		"-port", "4443",
		"-external-url", hostURL,
		"-public-host", fmt.Sprintf("localhost:%s", cfg.Port),
	}
}

// mustParsePort parses port string to int, returns 0 on error
func mustParsePort(port string) int {
	var p int
	fmt.Sscanf(port, "%d", &p)
	return p
}
