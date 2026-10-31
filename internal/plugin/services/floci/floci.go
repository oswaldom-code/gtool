package floci

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/internal/plugin"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
	"go.uber.org/zap"
)

const (
	defaultImage        = "floci/floci:2.3.0"
	defaultPort         = "4566"
	defaultRegion       = "us-east-1"
	defaultAccountID    = "000000000000"
	containerNamePrefix = "gtool-floci"
)

// FlociPlugin implements the ServicePlugin interface for Floci, a local AWS
// emulator serving S3, SQS and other AWS APIs on a single port.
type FlociPlugin struct {
	docker      *docker.Client
	logger      *zap.Logger
	httpClient  *http.Client
	containerID string
	config      *FlociConfig
}

// FlociConfig holds Floci-specific configuration
type FlociConfig struct {
	Image         string   `json:"image"`
	Port          string   `json:"port"`
	Region        string   `json:"region"`
	Buckets       []string `json:"buckets"`
	Queues        []string `json:"queues"`
	ContainerName string   `json:"container-name"`
}

// NewFlociPlugin creates a new Floci service plugin
func NewFlociPlugin(dockerClient *docker.Client, logger *zap.Logger) *FlociPlugin {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &FlociPlugin{
		docker:     dockerClient,
		logger:     logger,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Name returns the service identifier
func (p *FlociPlugin) Name() string {
	return "floci"
}

// Launch starts the Floci emulator with given configuration
func (p *FlociPlugin) Launch(ctx context.Context, config map[string]interface{}) error {
	p.logger.Info("launching Floci emulator")

	cfg, err := p.parseConfig(config)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to parse Floci configuration")
	}
	p.config = cfg

	p.logger.Info("pulling Floci image", zap.String("image", cfg.Image))
	if err := p.docker.EnsureImage(ctx, cfg.Image); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to pull Floci image")
	}

	containerConfig := &docker.ContainerConfig{
		Image: cfg.Image,
		Name:  cfg.ContainerName,
		Env:   []string{fmt.Sprintf("FLOCI_DEFAULT_REGION=%s", cfg.Region)},
		PortBindings: map[string]string{
			"4566": cfg.Port,
		},
		Labels: map[string]string{
			"managed-by": "gtool",
			"service":    "floci",
		},
	}

	p.logger.Info("creating Floci container",
		zap.String("name", cfg.ContainerName),
		zap.String("port", cfg.Port))

	containerID, err := p.docker.CreateContainer(ctx, containerConfig)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to create Floci container")
	}
	p.containerID = containerID

	p.logger.Info("starting Floci container", zap.String("containerID", containerID))
	if err := p.docker.StartContainer(ctx, containerID); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to start Floci container")
	}

	p.logger.Info("waiting for Floci emulator to be ready")
	if err := p.waitForReady(ctx); err != nil {
		// Cleanup on failure
		_ = p.Stop(ctx)
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "Floci emulator did not become ready")
	}

	if err := p.createResources(ctx, cfg); err != nil {
		_ = p.Stop(ctx)
		return err
	}

	p.logger.Info("Floci emulator launched successfully",
		zap.String("containerID", containerID),
		zap.String("port", cfg.Port))

	return nil
}

// IsReady checks if the Floci emulator is serving requests
func (p *FlociPlugin) IsReady(ctx context.Context) (bool, error) {
	if p.containerID == "" {
		return false, gtErrors.New(gtErrors.ErrServiceNotRunning, "Floci container not started")
	}

	running, err := p.docker.IsContainerRunning(ctx, p.containerID)
	if err != nil {
		return false, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to check container status")
	}

	if !running {
		return false, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint()+"/_localstack/health", nil)
	if err != nil {
		return false, gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to build readiness request")
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Debug("Floci not ready yet", zap.Error(err))
		return false, nil
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode == http.StatusOK, nil
}

// Stop terminates the Floci service
func (p *FlociPlugin) Stop(ctx context.Context) error {
	if p.containerID == "" {
		if p.docker == nil {
			p.logger.Debug("no container ID and no Docker client")
			return nil
		}

		p.logger.Info("no container ID, searching by labels")

		containers, err := p.docker.ListContainersByLabels(ctx, map[string]string{
			"managed-by": "gtool",
			"service":    "floci",
		})

		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list Floci containers")
		}

		if len(containers) == 0 {
			p.logger.Warn("no Floci containers found to stop")
			return nil
		}

		for _, container := range containers {
			p.containerID = container.ID
			p.logger.Info("found Floci container",
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
func (p *FlociPlugin) stopContainer(ctx context.Context) error {
	p.logger.Info("stopping Floci service", zap.String("containerID", p.containerID))

	timeout := 10
	if err := p.docker.StopContainer(ctx, p.containerID, &timeout); err != nil {
		p.logger.Error("failed to stop container", zap.Error(err))
		// Continue to remove anyway
	}

	if err := p.docker.RemoveContainer(ctx, p.containerID, true); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to remove Floci container")
	}

	p.logger.Info("Floci service stopped successfully")
	p.containerID = ""
	return nil
}

// GetConnectionInfo returns connection details
func (p *FlociPlugin) GetConnectionInfo() (*plugin.ConnectionInfo, error) {
	if p.config == nil {
		return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "Floci service not launched")
	}

	return &plugin.ConnectionInfo{
		Host:     "localhost",
		Port:     mustParsePort(p.config.Port),
		Protocol: "http",
		Metadata: map[string]string{
			"endpoint":   p.endpoint(),
			"region":     p.config.Region,
			"account-id": defaultAccountID,
		},
	}, nil
}

// GetLogs retrieves service logs
func (p *FlociPlugin) GetLogs(ctx context.Context, opts *plugin.LogOptions) ([]string, error) {
	containerID := p.containerID

	if containerID == "" {
		if p.docker == nil {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "no container ID and no Docker client")
		}

		p.logger.Info("no container ID for logs, searching by labels")

		containers, err := p.docker.ListContainersByLabels(ctx, map[string]string{
			"managed-by": "gtool",
			"service":    "floci",
		})

		if err != nil {
			return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list Floci containers")
		}

		if len(containers) == 0 {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "Floci container not found")
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
		p.logger.Info("found Floci container for logs",
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

// stringList extracts the non-empty strings of a YAML/JSON list value
func stringList(value interface{}) []string {
	var out []string
	items, _ := value.([]interface{})
	for _, item := range items {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// parseConfig parses the configuration map into FlociConfig
func (p *FlociPlugin) parseConfig(config map[string]interface{}) (*FlociConfig, error) {
	cfg := &FlociConfig{
		Image:         defaultImage,
		Port:          defaultPort,
		Region:        defaultRegion,
		ContainerName: fmt.Sprintf("%s-%d", containerNamePrefix, time.Now().Unix()),
	}

	if image, ok := config["image"].(string); ok && image != "" {
		cfg.Image = image
	}
	if port, ok := config["port"].(string); ok && port != "" {
		cfg.Port = port
	} else if port, ok := config["port"].(float64); ok {
		cfg.Port = fmt.Sprintf("%.0f", port)
	} else if port, ok := config["port"].(int); ok {
		cfg.Port = fmt.Sprintf("%d", port)
	}
	if region, ok := config["region"].(string); ok && region != "" {
		cfg.Region = region
	}
	cfg.Buckets = stringList(config["buckets"])
	cfg.Queues = stringList(config["queues"])
	if containerName, ok := config["container-name"].(string); ok && containerName != "" {
		cfg.ContainerName = containerName
	}

	return cfg, nil
}

// waitForReady waits for the Floci emulator to start serving requests
func (p *FlociPlugin) waitForReady(ctx context.Context) error {
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
			p.logger.Info("Floci emulator is ready", zap.Int("attempts", i+1))
			return nil
		}

		select {
		case <-ctx.Done():
			return gtErrors.New(gtErrors.ErrServiceFailed, "context cancelled while waiting for Floci")
		case <-time.After(interval):
			// Continue to next attempt
		}
	}

	return gtErrors.New(gtErrors.ErrServiceFailed,
		fmt.Sprintf("Floci emulator did not become ready after %d attempts", maxRetries))
}

// send executes req and fails unless the response status is 200 OK
func (p *FlociPlugin) send(req *http.Request, resource string) error {
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, fmt.Sprintf("failed to create %s", resource))
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return gtErrors.New(gtErrors.ErrServiceFailed,
			fmt.Sprintf("unexpected status %d creating %s", resp.StatusCode, resource))
	}

	p.logger.Info("resource created", zap.String("resource", resource))
	return nil
}

// createBucket creates an S3 bucket with a path-style PUT. Floci does not
// enforce request signing by default, so the request is unsigned.
func (p *FlociPlugin) createBucket(ctx context.Context, bucket string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, p.endpoint()+"/"+bucket, nil)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to build bucket request")
	}
	return p.send(req, "bucket "+bucket)
}

// createQueue creates an SQS queue through the SQS query API
func (p *FlociPlugin) createQueue(ctx context.Context, queue string) error {
	form := url.Values{"Action": {"CreateQueue"}, "QueueName": {queue}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint()+"/", strings.NewReader(form.Encode()))
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to build queue request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return p.send(req, "queue "+queue)
}

// createResources creates the configured buckets and queues
func (p *FlociPlugin) createResources(ctx context.Context, cfg *FlociConfig) error {
	for _, bucket := range cfg.Buckets {
		if err := p.createBucket(ctx, bucket); err != nil {
			return err
		}
	}
	for _, queue := range cfg.Queues {
		if err := p.createQueue(ctx, queue); err != nil {
			return err
		}
	}
	return nil
}

// endpoint returns the host-facing Floci API endpoint
func (p *FlociPlugin) endpoint() string {
	return fmt.Sprintf("http://localhost:%s", p.config.Port)
}

// mustParsePort parses port string to int, returns 0 on error
func mustParsePort(port string) int {
	var p int
	_, _ = fmt.Sscanf(port, "%d", &p)
	return p
}
