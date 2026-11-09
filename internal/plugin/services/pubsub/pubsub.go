package pubsub

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
	"github.com/oswaldom-code/gtool/internal/plugin/services/svcconfig"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
	"go.uber.org/zap"
)

const (
	defaultImage        = "gcr.io/google.com/cloudsdktool/cloud-sdk:emulators"
	defaultPort         = "8085"
	defaultProjectID    = "test-project"
	containerNamePrefix = "gtool-pubsub"
)

// ConfigKeys lists every mock-config key the plugin accepts (aliases included).
func ConfigKeys() []string {
	return []string{"image", "port", "project-id", "topics", "subscriptions", "container-name"}
}

// DefaultPorts returns the host port used for each port key when the config
// does not set it.
func DefaultPorts() map[string]string {
	return map[string]string{
		"port": defaultPort,
	}
}

// PubSubPlugin implements the ServicePlugin interface for the Google Cloud
// Pub/Sub emulator.
type PubSubPlugin struct {
	docker      *docker.Client
	logger      *zap.Logger
	httpClient  *http.Client
	containerID string
	config      *PubSubConfig
}

// SubscriptionConfig describes a subscription bound to a topic
type SubscriptionConfig struct {
	Name  string `json:"name"`
	Topic string `json:"topic"`
}

// PubSubConfig holds Pub/Sub-specific configuration
type PubSubConfig struct {
	Image         string               `json:"image"`
	Port          string               `json:"port"`
	ProjectID     string               `json:"project-id"`
	Topics        []string             `json:"topics"`
	Subscriptions []SubscriptionConfig `json:"subscriptions"`
	ContainerName string               `json:"container-name"`
}

// NewPubSubPlugin creates a new Pub/Sub service plugin
func NewPubSubPlugin(dockerClient *docker.Client, logger *zap.Logger) *PubSubPlugin {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &PubSubPlugin{
		docker:     dockerClient,
		logger:     logger,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Name returns the service identifier
func (p *PubSubPlugin) Name() string {
	return "pubsub"
}

// Launch starts the Pub/Sub emulator with given configuration
func (p *PubSubPlugin) Launch(ctx context.Context, config map[string]interface{}) error {
	p.logger.Info("launching Pub/Sub emulator")

	cfg, err := p.parseConfig(config)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to parse Pub/Sub configuration")
	}
	p.config = cfg

	p.logger.Info("ensuring Pub/Sub image", zap.String("image", cfg.Image))
	if err := p.docker.EnsureImage(ctx, cfg.Image); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to get Pub/Sub image")
	}

	containerConfig := &docker.ContainerConfig{
		Image: cfg.Image,
		Name:  cfg.ContainerName,
		Cmd: []string{
			"gcloud", "beta", "emulators", "pubsub", "start",
			"--host-port=0.0.0.0:8085",
			fmt.Sprintf("--project=%s", cfg.ProjectID),
		},
		PortBindings: map[string]string{
			"8085": cfg.Port,
		},
		Labels: map[string]string{
			"managed-by": "gtool",
			"service":    "pubsub",
		},
	}

	p.logger.Info("creating Pub/Sub container",
		zap.String("name", cfg.ContainerName),
		zap.String("port", cfg.Port))

	containerID, err := p.docker.RunContainer(ctx, containerConfig)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to start Pub/Sub container")
	}
	p.containerID = containerID

	p.logger.Info("waiting for Pub/Sub emulator to be ready")
	if err := p.waitForReady(ctx); err != nil {
		// Cleanup on failure
		_ = p.Stop(ctx)
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "Pub/Sub emulator did not become ready")
	}

	// Create topics, then subscriptions (which reference topics)
	if len(cfg.Topics) > 0 {
		p.logger.Info("creating topics", zap.Strings("topics", cfg.Topics))
		if err := p.createTopics(ctx, cfg); err != nil {
			_ = p.Stop(context.Background())
			return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to create Pub/Sub topics")
		}
	}
	if len(cfg.Subscriptions) > 0 {
		p.logger.Info("creating subscriptions", zap.Int("count", len(cfg.Subscriptions)))
		if err := p.createSubscriptions(ctx, cfg); err != nil {
			_ = p.Stop(context.Background())
			return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to create Pub/Sub subscriptions")
		}
	}

	p.logger.Info("Pub/Sub emulator launched successfully",
		zap.String("containerID", containerID),
		zap.String("port", cfg.Port))

	return nil
}

// IsReady checks if the Pub/Sub emulator REST API is accepting requests
func (p *PubSubPlugin) IsReady(ctx context.Context) (bool, error) {
	if p.containerID == "" {
		return false, gtErrors.New(gtErrors.ErrServiceNotRunning, "Pub/Sub container not started")
	}

	running, err := p.docker.IsContainerRunning(ctx, p.containerID)
	if err != nil {
		return false, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to check container status")
	}

	if !running {
		return false, nil
	}

	// Listing topics succeeds only once the emulator is serving requests
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.listTopicsURL(), nil)
	if err != nil {
		return false, gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to build readiness request")
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Debug("Pub/Sub not ready yet", zap.Error(err))
		return false, nil
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode == http.StatusOK, nil
}

// Stop terminates the Pub/Sub service
func (p *PubSubPlugin) Stop(ctx context.Context) error {
	if p.containerID == "" {
		if p.docker == nil {
			p.logger.Debug("no container ID and no Docker client")
			return nil
		}

		p.logger.Info("no container ID, searching by labels")

		containers, err := p.docker.ListContainersByLabels(ctx, map[string]string{
			"managed-by": "gtool",
			"service":    "pubsub",
		})

		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list Pub/Sub containers")
		}

		if len(containers) == 0 {
			p.logger.Warn("no Pub/Sub containers found to stop")
			return nil
		}

		var firstErr error
		for _, container := range containers {
			p.containerID = container.ID
			p.logger.Info("found Pub/Sub container",
				zap.String("containerID", container.ID),
				zap.Strings("names", container.Names))

			if err := p.stopContainer(ctx); err != nil {
				p.logger.Error("failed to stop container", zap.Error(err), zap.String("containerID", container.ID))
				if firstErr == nil {
					firstErr = err
				}
			}
		}

		return firstErr
	}

	return p.stopContainer(ctx)
}

// stopContainer stops and removes a specific container
func (p *PubSubPlugin) stopContainer(ctx context.Context) error {
	p.logger.Info("stopping Pub/Sub service", zap.String("containerID", p.containerID))

	timeout := 10
	if err := p.docker.StopContainer(ctx, p.containerID, &timeout); err != nil {
		p.logger.Error("failed to stop container", zap.Error(err))
		// Continue to remove anyway
	}

	if err := p.docker.RemoveContainer(ctx, p.containerID, true); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to remove Pub/Sub container")
	}

	p.logger.Info("Pub/Sub service stopped successfully")
	p.containerID = ""
	p.config = nil
	return nil
}

// GetConnectionInfo returns connection details
func (p *PubSubPlugin) GetConnectionInfo() (*plugin.ConnectionInfo, error) {
	if p.config == nil {
		return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "Pub/Sub service not launched")
	}

	return &plugin.ConnectionInfo{
		Host:     "localhost",
		Port:     mustParsePort(p.config.Port),
		Protocol: "http",
		Metadata: map[string]string{
			"emulator-host": fmt.Sprintf("localhost:%s", p.config.Port),
			"project-id":    p.config.ProjectID,
		},
	}, nil
}

// GetLogs retrieves service logs
func (p *PubSubPlugin) GetLogs(ctx context.Context, opts *plugin.LogOptions) ([]string, error) {
	containerID := p.containerID

	if containerID == "" {
		if p.docker == nil {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "no container ID and no Docker client")
		}

		p.logger.Info("no container ID for logs, searching by labels")

		containers, err := p.docker.ListContainersByLabels(ctx, map[string]string{
			"managed-by": "gtool",
			"service":    "pubsub",
		})

		if err != nil {
			return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list Pub/Sub containers")
		}

		if len(containers) == 0 {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "Pub/Sub container not found")
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
		p.logger.Info("found Pub/Sub container for logs",
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

// parseConfig parses the configuration map into PubSubConfig
func (p *PubSubPlugin) parseConfig(config map[string]interface{}) (*PubSubConfig, error) {
	cfg := &PubSubConfig{
		Image:         defaultImage,
		Port:          defaultPort,
		ProjectID:     defaultProjectID,
		ContainerName: fmt.Sprintf("%s-%d", containerNamePrefix, time.Now().Unix()),
	}

	cfg.Image = svcconfig.String(config, "image", cfg.Image)
	cfg.Port = svcconfig.Port(config, "port", cfg.Port)
	cfg.ProjectID = svcconfig.String(config, "project-id", cfg.ProjectID)
	cfg.Topics = append(cfg.Topics, svcconfig.Strings(config, "topics")...)
	if subs, ok := config["subscriptions"].([]interface{}); ok {
		for _, s := range subs {
			if m, ok := s.(map[string]interface{}); ok {
				name := svcconfig.String(m, "name", "")
				topic := svcconfig.String(m, "topic", "")
				if name != "" && topic != "" {
					cfg.Subscriptions = append(cfg.Subscriptions, SubscriptionConfig{Name: name, Topic: topic})
				}
			}
		}
	}
	cfg.ContainerName = svcconfig.String(config, "container-name", cfg.ContainerName)

	return cfg, nil
}

// waitForReady waits for the Pub/Sub emulator to start serving requests
func (p *PubSubPlugin) waitForReady(ctx context.Context) error {
	maxRetries := 30
	interval := time.Second

	for i := 0; i < maxRetries; i++ {
		ready, err := p.IsReady(ctx)
		if err != nil {
			if gtErrors.Is(err, gtErrors.ErrContainerExited) {
				return err
			}
			p.logger.Debug("error checking readiness",
				zap.Error(err),
				zap.Int("attempt", i+1))
		}

		if ready {
			p.logger.Info("Pub/Sub emulator is ready", zap.Int("attempts", i+1))
			return nil
		}

		select {
		case <-ctx.Done():
			return gtErrors.New(gtErrors.ErrServiceFailed, "context cancelled while waiting for Pub/Sub")
		case <-time.After(interval):
			// Continue to next attempt
		}
	}

	return gtErrors.New(gtErrors.ErrServiceFailed,
		fmt.Sprintf("Pub/Sub emulator did not become ready after %d attempts", maxRetries))
}

// createTopics creates the configured topics via the emulator REST API
func (p *PubSubPlugin) createTopics(ctx context.Context, cfg *PubSubConfig) error {
	for _, topic := range cfg.Topics {
		if err := p.putResource(ctx, p.topicURL(topic), nil); err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
				fmt.Sprintf("failed to create topic: %s", topic))
		}
		p.logger.Info("topic created", zap.String("topic", topic))
	}
	return nil
}

// createSubscriptions creates the configured subscriptions, each bound to its topic
func (p *PubSubPlugin) createSubscriptions(ctx context.Context, cfg *PubSubConfig) error {
	for _, sub := range cfg.Subscriptions {
		body := []byte(fmt.Sprintf(`{"topic":"projects/%s/topics/%s"}`, cfg.ProjectID, sub.Topic))
		if err := p.putResource(ctx, p.subscriptionURL(sub.Name), body); err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
				fmt.Sprintf("failed to create subscription: %s", sub.Name))
		}
		p.logger.Info("subscription created",
			zap.String("subscription", sub.Name),
			zap.String("topic", sub.Topic))
	}
	return nil
}

// putResource issues a PUT to the emulator REST API to create a resource
func (p *PubSubPlugin) putResource(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to build request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "request failed")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return gtErrors.New(gtErrors.ErrServiceFailed,
			fmt.Sprintf("unexpected status %d for %s", resp.StatusCode, url))
	}
	return nil
}

// baseURL returns the emulator REST API base for the configured project
func (p *PubSubPlugin) baseURL() string {
	return fmt.Sprintf("http://localhost:%s/v1/projects/%s", p.config.Port, p.config.ProjectID)
}

func (p *PubSubPlugin) listTopicsURL() string {
	return p.baseURL() + "/topics"
}

func (p *PubSubPlugin) topicURL(topic string) string {
	return fmt.Sprintf("%s/topics/%s", p.baseURL(), topic)
}

func (p *PubSubPlugin) subscriptionURL(sub string) string {
	return fmt.Sprintf("%s/subscriptions/%s", p.baseURL(), sub)
}

// mustParsePort parses port string to int, returns 0 on error
func mustParsePort(port string) int {
	var p int
	if _, err := fmt.Sscanf(port, "%d", &p); err != nil {
		return 0
	}
	return p
}
