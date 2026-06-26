package kafka

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
	defaultImage        = "apache/kafka:3.7.0"
	defaultPort         = "9092"
	defaultPartitions   = 1
	containerNamePrefix = "gtool-kafka"

	// internalBootstrap is the in-container listener used for admin operations
	// (health check, topic creation). It is independent of the host port so it
	// works regardless of how the external listener is mapped.
	internalBootstrap = "localhost:29092"
	// kafkaTopicsBin is the path to the kafka-topics CLI in the apache/kafka image.
	kafkaTopicsBin = "/opt/kafka/bin/kafka-topics.sh"
)

// KafkaPlugin implements the ServicePlugin interface for a single-node Kafka
// broker running in KRaft mode.
type KafkaPlugin struct {
	docker      *docker.Client
	logger      *zap.Logger
	containerID string
	config      *KafkaConfig
}

// KafkaConfig holds Kafka-specific configuration
type KafkaConfig struct {
	Image         string   `json:"image"`
	Port          string   `json:"port"`
	Topics        []string `json:"topics"`
	Partitions    int      `json:"partitions"`
	ContainerName string   `json:"container-name"`
}

// NewKafkaPlugin creates a new Kafka service plugin
func NewKafkaPlugin(dockerClient *docker.Client, logger *zap.Logger) *KafkaPlugin {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &KafkaPlugin{
		docker: dockerClient,
		logger: logger,
	}
}

// Name returns the service identifier
func (p *KafkaPlugin) Name() string {
	return "kafka"
}

// Launch starts the Kafka broker with given configuration
func (p *KafkaPlugin) Launch(ctx context.Context, config map[string]interface{}) error {
	p.logger.Info("launching Kafka service")

	cfg, err := p.parseConfig(config)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to parse Kafka configuration")
	}
	p.config = cfg

	p.logger.Info("pulling Kafka image", zap.String("image", cfg.Image))
	if err := p.docker.PullImage(ctx, cfg.Image); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to pull Kafka image")
	}

	containerConfig := &docker.ContainerConfig{
		Image: cfg.Image,
		Name:  cfg.ContainerName,
		Env:   p.brokerEnv(cfg),
		PortBindings: map[string]string{
			"9092": cfg.Port,
		},
		Labels: map[string]string{
			"managed-by": "gtool",
			"service":    "kafka",
		},
	}

	p.logger.Info("creating Kafka container",
		zap.String("name", cfg.ContainerName),
		zap.String("port", cfg.Port))

	containerID, err := p.docker.CreateContainer(ctx, containerConfig)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to create Kafka container")
	}
	p.containerID = containerID

	p.logger.Info("starting Kafka container", zap.String("containerID", containerID))
	if err := p.docker.StartContainer(ctx, containerID); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to start Kafka container")
	}

	p.logger.Info("waiting for Kafka to be ready")
	if err := p.waitForReady(ctx); err != nil {
		// Cleanup on failure
		_ = p.Stop(ctx)
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "Kafka did not become ready")
	}

	// Create topics if any are configured
	if len(cfg.Topics) > 0 {
		p.logger.Info("creating topics", zap.Strings("topics", cfg.Topics))
		if err := p.createTopics(ctx, cfg.Topics, cfg.Partitions); err != nil {
			// Don't fail the launch if topic creation fails, just log the error
			p.logger.Error("failed to create topics",
				zap.Error(err),
				zap.Strings("topics", cfg.Topics))
		}
	}

	p.logger.Info("Kafka service launched successfully",
		zap.String("containerID", containerID),
		zap.String("port", cfg.Port))

	return nil
}

// IsReady checks if the Kafka broker is accepting requests
func (p *KafkaPlugin) IsReady(ctx context.Context) (bool, error) {
	if p.containerID == "" {
		return false, gtErrors.New(gtErrors.ErrServiceNotRunning, "Kafka container not started")
	}

	running, err := p.docker.IsContainerRunning(ctx, p.containerID)
	if err != nil {
		return false, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to check container status")
	}

	if !running {
		return false, nil
	}

	// Listing topics succeeds only once the broker is serving requests
	output, err := p.docker.ExecInContainer(ctx, p.containerID, &docker.ExecConfig{
		Cmd:          []string{kafkaTopicsBin, "--bootstrap-server", internalBootstrap, "--list"},
		AttachStdout: true,
		AttachStderr: true,
	})

	if err != nil {
		p.logger.Debug("Kafka not ready yet", zap.String("output", output))
		return false, nil
	}

	return true, nil
}

// Stop terminates the Kafka service
func (p *KafkaPlugin) Stop(ctx context.Context) error {
	if p.containerID == "" {
		if p.docker == nil {
			p.logger.Debug("no container ID and no Docker client")
			return nil
		}

		p.logger.Info("no container ID, searching by labels")

		containers, err := p.docker.ListContainersByLabels(ctx, map[string]string{
			"managed-by": "gtool",
			"service":    "kafka",
		})

		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list Kafka containers")
		}

		if len(containers) == 0 {
			p.logger.Warn("no Kafka containers found to stop")
			return nil
		}

		for _, container := range containers {
			p.containerID = container.ID
			p.logger.Info("found Kafka container",
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
func (p *KafkaPlugin) stopContainer(ctx context.Context) error {
	p.logger.Info("stopping Kafka service", zap.String("containerID", p.containerID))

	timeout := 10
	if err := p.docker.StopContainer(ctx, p.containerID, &timeout); err != nil {
		p.logger.Error("failed to stop container", zap.Error(err))
		// Continue to remove anyway
	}

	if err := p.docker.RemoveContainer(ctx, p.containerID, true); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to remove Kafka container")
	}

	p.logger.Info("Kafka service stopped successfully")
	p.containerID = ""
	return nil
}

// GetConnectionInfo returns connection details
func (p *KafkaPlugin) GetConnectionInfo() (*plugin.ConnectionInfo, error) {
	if p.config == nil {
		return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "Kafka service not launched")
	}

	return &plugin.ConnectionInfo{
		Host:     "localhost",
		Port:     mustParsePort(p.config.Port),
		Protocol: "kafka",
		Metadata: map[string]string{
			"bootstrap-servers": p.bootstrapServers(),
		},
	}, nil
}

// GetLogs retrieves service logs
func (p *KafkaPlugin) GetLogs(ctx context.Context, opts *plugin.LogOptions) ([]string, error) {
	containerID := p.containerID

	if containerID == "" {
		if p.docker == nil {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "no container ID and no Docker client")
		}

		p.logger.Info("no container ID for logs, searching by labels")

		containers, err := p.docker.ListContainersByLabels(ctx, map[string]string{
			"managed-by": "gtool",
			"service":    "kafka",
		})

		if err != nil {
			return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list Kafka containers")
		}

		if len(containers) == 0 {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "Kafka container not found")
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
		p.logger.Info("found Kafka container for logs",
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

// parseConfig parses the configuration map into KafkaConfig
func (p *KafkaPlugin) parseConfig(config map[string]interface{}) (*KafkaConfig, error) {
	cfg := &KafkaConfig{
		Image:         defaultImage,
		Port:          defaultPort,
		Partitions:    defaultPartitions,
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
	if partitions, ok := config["partitions"].(float64); ok && partitions > 0 {
		cfg.Partitions = int(partitions)
	} else if partitions, ok := config["partitions"].(int); ok && partitions > 0 {
		cfg.Partitions = partitions
	}
	if topics, ok := config["topics"].([]interface{}); ok {
		for _, t := range topics {
			if name, ok := t.(string); ok && name != "" {
				cfg.Topics = append(cfg.Topics, name)
			}
		}
	}
	if containerName, ok := config["container-name"].(string); ok && containerName != "" {
		cfg.ContainerName = containerName
	}

	return cfg, nil
}

// brokerEnv builds the KRaft single-node broker environment. Two PLAINTEXT
// listeners are used: INTERNAL for in-container admin operations and EXTERNAL
// (advertised on the host port) for clients running on the host.
func (p *KafkaPlugin) brokerEnv(cfg *KafkaConfig) []string {
	return []string{
		"KAFKA_NODE_ID=1",
		"KAFKA_PROCESS_ROLES=broker,controller",
		"KAFKA_CONTROLLER_QUORUM_VOTERS=1@localhost:9093",
		"KAFKA_LISTENERS=INTERNAL://:29092,EXTERNAL://:9092,CONTROLLER://:9093",
		fmt.Sprintf("KAFKA_ADVERTISED_LISTENERS=INTERNAL://%s,EXTERNAL://localhost:%s", internalBootstrap, cfg.Port),
		"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=CONTROLLER:PLAINTEXT,INTERNAL:PLAINTEXT,EXTERNAL:PLAINTEXT",
		"KAFKA_INTER_BROKER_LISTENER_NAME=INTERNAL",
		"KAFKA_CONTROLLER_LISTENER_NAMES=CONTROLLER",
		"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1",
		"KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR=1",
		"KAFKA_TRANSACTION_STATE_LOG_MIN_ISR=1",
		"KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS=0",
	}
}

// waitForReady waits for the Kafka broker to start serving requests
func (p *KafkaPlugin) waitForReady(ctx context.Context) error {
	maxRetries := 30
	interval := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		ready, err := p.IsReady(ctx)
		if err != nil {
			p.logger.Debug("error checking readiness",
				zap.Error(err),
				zap.Int("attempt", i+1))
		}

		if ready {
			p.logger.Info("Kafka is ready", zap.Int("attempts", i+1))
			return nil
		}

		select {
		case <-ctx.Done():
			return gtErrors.New(gtErrors.ErrServiceFailed, "context cancelled while waiting for Kafka")
		case <-time.After(interval):
			// Continue to next attempt
		}
	}

	return gtErrors.New(gtErrors.ErrServiceFailed,
		fmt.Sprintf("Kafka did not become ready after %d attempts", maxRetries))
}

// createTopics creates the configured topics inside the broker
func (p *KafkaPlugin) createTopics(ctx context.Context, topics []string, partitions int) error {
	for _, topic := range topics {
		if err := p.createTopic(ctx, topic, partitions); err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
				fmt.Sprintf("failed to create topic: %s", topic))
		}
	}
	return nil
}

// createTopic creates a single topic with replication-factor 1 (single node)
func (p *KafkaPlugin) createTopic(ctx context.Context, topic string, partitions int) error {
	p.logger.Info("creating topic", zap.String("topic", topic), zap.Int("partitions", partitions))

	output, err := p.docker.ExecInContainer(ctx, p.containerID, &docker.ExecConfig{
		Cmd: []string{
			kafkaTopicsBin,
			"--bootstrap-server", internalBootstrap,
			"--create",
			"--if-not-exists",
			"--topic", topic,
			"--partitions", fmt.Sprintf("%d", partitions),
			"--replication-factor", "1",
		},
		AttachStdout: true,
		AttachStderr: true,
	})

	if err != nil {
		p.logger.Error("failed to create topic",
			zap.Error(err),
			zap.String("topic", topic),
			zap.String("output", output))
		return err
	}

	p.logger.Info("topic created successfully",
		zap.String("topic", topic),
		zap.String("output", strings.TrimSpace(output)))

	return nil
}

// bootstrapServers returns the host-facing bootstrap server address
func (p *KafkaPlugin) bootstrapServers() string {
	return fmt.Sprintf("localhost:%s", p.config.Port)
}

// mustParsePort parses port string to int, returns 0 on error
func mustParsePort(port string) int {
	var p int
	fmt.Sscanf(port, "%d", &p)
	return p
}
