package couchbase

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
	defaultImage        = "couchbase:community-7.6.2"
	defaultAdminPort    = "8091"
	defaultDataPort     = "11210"
	defaultUsername     = "Administrator"
	defaultPassword     = "password"
	defaultBucket       = "default"
	defaultBucketRAMMB  = 128
	defaultClusterRAMMB = 256
	defaultServices     = "data,index,query"
	containerNamePrefix = "gtool-couchbase"

	// couchbaseCLIBin is the path to the couchbase-cli tool in the official image.
	couchbaseCLIBin = "/opt/couchbase/bin/couchbase-cli"
	// clusterEndpoint is the in-container REST endpoint used by couchbase-cli.
	clusterEndpoint = "localhost:8091"
)

// CouchbasePlugin implements the ServicePlugin interface for a single-node
// Couchbase Server cluster.
type CouchbasePlugin struct {
	docker      *docker.Client
	logger      *zap.Logger
	containerID string
	config      *CouchbaseConfig
}

// CouchbaseConfig holds Couchbase-specific configuration
type CouchbaseConfig struct {
	Image         string `json:"image"`
	AdminPort     string `json:"admin-port"`
	DataPort      string `json:"data-port"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	Bucket        string `json:"bucket"`
	BucketRAMMB   int    `json:"bucket-ram-mb"`
	ClusterRAMMB  int    `json:"cluster-ram-mb"`
	Services      string `json:"services"`
	ContainerName string `json:"container-name"`
}

// NewCouchbasePlugin creates a new Couchbase service plugin
func NewCouchbasePlugin(dockerClient *docker.Client, logger *zap.Logger) *CouchbasePlugin {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &CouchbasePlugin{
		docker: dockerClient,
		logger: logger,
	}
}

// Name returns the service identifier
func (p *CouchbasePlugin) Name() string {
	return "couchbase"
}

// Launch starts the Couchbase service and initializes a single-node cluster
func (p *CouchbasePlugin) Launch(ctx context.Context, config map[string]interface{}) error {
	p.logger.Info("launching Couchbase service")

	cfg, err := p.parseConfig(config)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to parse Couchbase configuration")
	}
	p.config = cfg

	p.logger.Info("pulling Couchbase image", zap.String("image", cfg.Image))
	if err := p.docker.PullImage(ctx, cfg.Image); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to pull Couchbase image")
	}

	containerConfig := &docker.ContainerConfig{
		Image: cfg.Image,
		Name:  cfg.ContainerName,
		PortBindings: map[string]string{
			"8091":  cfg.AdminPort,
			"11210": cfg.DataPort,
		},
		Labels: map[string]string{
			"managed-by": "gtool",
			"service":    "couchbase",
		},
	}

	p.logger.Info("creating Couchbase container",
		zap.String("name", cfg.ContainerName),
		zap.String("admin-port", cfg.AdminPort))

	containerID, err := p.docker.CreateContainer(ctx, containerConfig)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to create Couchbase container")
	}
	p.containerID = containerID

	p.logger.Info("starting Couchbase container", zap.String("containerID", containerID))
	if err := p.docker.StartContainer(ctx, containerID); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to start Couchbase container")
	}

	// Initialize the cluster (retries until the REST API is up)
	p.logger.Info("initializing Couchbase cluster")
	if err := p.initializeCluster(ctx); err != nil {
		// Cleanup on failure
		_ = p.Stop(ctx)
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "Couchbase cluster did not initialize")
	}

	// Create the configured bucket
	if cfg.Bucket != "" {
		p.logger.Info("creating bucket", zap.String("bucket", cfg.Bucket))
		if err := p.createBucket(ctx, cfg); err != nil {
			// Don't fail the launch if bucket creation fails, just log the error
			p.logger.Error("failed to create bucket",
				zap.Error(err),
				zap.String("bucket", cfg.Bucket))
		}
	}

	p.logger.Info("Couchbase service launched successfully",
		zap.String("containerID", containerID),
		zap.String("admin-port", cfg.AdminPort))

	return nil
}

// IsReady checks if the Couchbase cluster is healthy
func (p *CouchbasePlugin) IsReady(ctx context.Context) (bool, error) {
	if p.containerID == "" {
		return false, gtErrors.New(gtErrors.ErrServiceNotRunning, "Couchbase container not started")
	}

	running, err := p.docker.IsContainerRunning(ctx, p.containerID)
	if err != nil {
		return false, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to check container status")
	}

	if !running {
		return false, nil
	}

	// server-info succeeds only once the cluster is initialized and serving
	output, err := p.docker.ExecInContainer(ctx, p.containerID, &docker.ExecConfig{
		Cmd: []string{
			couchbaseCLIBin, "server-info",
			"--cluster", clusterEndpoint,
			"--username", p.config.Username,
			"--password", p.config.Password,
		},
		AttachStdout: true,
		AttachStderr: true,
	})

	if err != nil {
		p.logger.Debug("Couchbase not ready yet", zap.String("output", output))
		return false, nil
	}

	return true, nil
}

// Stop terminates the Couchbase service
func (p *CouchbasePlugin) Stop(ctx context.Context) error {
	if p.containerID == "" {
		if p.docker == nil {
			p.logger.Debug("no container ID and no Docker client")
			return nil
		}

		p.logger.Info("no container ID, searching by labels")

		containers, err := p.docker.ListContainersByLabels(ctx, map[string]string{
			"managed-by": "gtool",
			"service":    "couchbase",
		})

		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list Couchbase containers")
		}

		if len(containers) == 0 {
			p.logger.Warn("no Couchbase containers found to stop")
			return nil
		}

		for _, container := range containers {
			p.containerID = container.ID
			p.logger.Info("found Couchbase container",
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
func (p *CouchbasePlugin) stopContainer(ctx context.Context) error {
	p.logger.Info("stopping Couchbase service", zap.String("containerID", p.containerID))

	timeout := 10
	if err := p.docker.StopContainer(ctx, p.containerID, &timeout); err != nil {
		p.logger.Error("failed to stop container", zap.Error(err))
		// Continue to remove anyway
	}

	if err := p.docker.RemoveContainer(ctx, p.containerID, true); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to remove Couchbase container")
	}

	p.logger.Info("Couchbase service stopped successfully")
	p.containerID = ""
	return nil
}

// GetConnectionInfo returns connection details
func (p *CouchbasePlugin) GetConnectionInfo() (*plugin.ConnectionInfo, error) {
	if p.config == nil {
		return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "Couchbase service not launched")
	}

	return &plugin.ConnectionInfo{
		Host:     "localhost",
		Port:     mustParsePort(p.config.AdminPort),
		Protocol: "couchbase",
		Metadata: map[string]string{
			"username":          p.config.Username,
			"password":          p.config.Password,
			"bucket":            p.config.Bucket,
			"connection-string": "couchbase://localhost",
		},
	}, nil
}

// GetLogs retrieves service logs
func (p *CouchbasePlugin) GetLogs(ctx context.Context, opts *plugin.LogOptions) ([]string, error) {
	containerID := p.containerID

	if containerID == "" {
		if p.docker == nil {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "no container ID and no Docker client")
		}

		p.logger.Info("no container ID for logs, searching by labels")

		containers, err := p.docker.ListContainersByLabels(ctx, map[string]string{
			"managed-by": "gtool",
			"service":    "couchbase",
		})

		if err != nil {
			return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list Couchbase containers")
		}

		if len(containers) == 0 {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "Couchbase container not found")
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
		p.logger.Info("found Couchbase container for logs",
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

// parseConfig parses the configuration map into CouchbaseConfig
func (p *CouchbasePlugin) parseConfig(config map[string]interface{}) (*CouchbaseConfig, error) {
	cfg := &CouchbaseConfig{
		Image:         defaultImage,
		AdminPort:     defaultAdminPort,
		DataPort:      defaultDataPort,
		Username:      defaultUsername,
		Password:      defaultPassword,
		Bucket:        defaultBucket,
		BucketRAMMB:   defaultBucketRAMMB,
		ClusterRAMMB:  defaultClusterRAMMB,
		Services:      defaultServices,
		ContainerName: fmt.Sprintf("%s-%d", containerNamePrefix, time.Now().Unix()),
	}

	if image, ok := config["image"].(string); ok && image != "" {
		cfg.Image = image
	}
	cfg.AdminPort = stringOrFloat(config, "admin-port", cfg.AdminPort)
	cfg.DataPort = stringOrFloat(config, "data-port", cfg.DataPort)
	if username, ok := config["username"].(string); ok && username != "" {
		cfg.Username = username
	}
	if password, ok := config["password"].(string); ok && password != "" {
		cfg.Password = password
	}
	if bucket, ok := config["bucket"].(string); ok && bucket != "" {
		cfg.Bucket = bucket
	}
	if services, ok := config["services"].(string); ok && services != "" {
		cfg.Services = services
	}
	cfg.BucketRAMMB = intOrFloat(config, "bucket-ram-mb", cfg.BucketRAMMB)
	cfg.ClusterRAMMB = intOrFloat(config, "cluster-ram-mb", cfg.ClusterRAMMB)
	if containerName, ok := config["container-name"].(string); ok && containerName != "" {
		cfg.ContainerName = containerName
	}

	return cfg, nil
}

// initializeCluster runs couchbase-cli cluster-init, retrying until the REST
// API is available or the context is cancelled.
func (p *CouchbasePlugin) initializeCluster(ctx context.Context) error {
	maxRetries := 30
	interval := 2 * time.Second

	cmd := []string{
		couchbaseCLIBin, "cluster-init",
		"--cluster", clusterEndpoint,
		"--cluster-username", p.config.Username,
		"--cluster-password", p.config.Password,
		"--services", p.config.Services,
		"--cluster-ramsize", fmt.Sprintf("%d", p.config.ClusterRAMMB),
	}
	if strings.Contains(p.config.Services, "index") {
		cmd = append(cmd, "--cluster-index-ramsize", fmt.Sprintf("%d", p.config.ClusterRAMMB))
	}

	for i := 0; i < maxRetries; i++ {
		output, err := p.docker.ExecInContainer(ctx, p.containerID, &docker.ExecConfig{
			Cmd:          cmd,
			AttachStdout: true,
			AttachStderr: true,
		})
		if err == nil {
			p.logger.Info("Couchbase cluster initialized", zap.Int("attempts", i+1))
			return nil
		}

		p.logger.Debug("cluster not ready yet",
			zap.Error(err),
			zap.String("output", output),
			zap.Int("attempt", i+1))

		select {
		case <-ctx.Done():
			return gtErrors.New(gtErrors.ErrServiceFailed, "context cancelled while initializing Couchbase")
		case <-time.After(interval):
			// Continue to next attempt
		}
	}

	return gtErrors.New(gtErrors.ErrServiceFailed,
		fmt.Sprintf("Couchbase cluster did not initialize after %d attempts", maxRetries))
}

// createBucket creates the configured bucket via couchbase-cli
func (p *CouchbasePlugin) createBucket(ctx context.Context, cfg *CouchbaseConfig) error {
	output, err := p.docker.ExecInContainer(ctx, p.containerID, &docker.ExecConfig{
		Cmd: []string{
			couchbaseCLIBin, "bucket-create",
			"--cluster", clusterEndpoint,
			"--username", cfg.Username,
			"--password", cfg.Password,
			"--bucket", cfg.Bucket,
			"--bucket-type", "couchbase",
			"--bucket-ramsize", fmt.Sprintf("%d", cfg.BucketRAMMB),
			"--wait",
		},
		AttachStdout: true,
		AttachStderr: true,
	})

	if err != nil {
		p.logger.Error("failed to create bucket",
			zap.Error(err),
			zap.String("bucket", cfg.Bucket),
			zap.String("output", output))
		return err
	}

	p.logger.Info("bucket created successfully",
		zap.String("bucket", cfg.Bucket),
		zap.String("output", strings.TrimSpace(output)))

	return nil
}

// mustParsePort parses port string to int, returns 0 on error
func mustParsePort(port string) int {
	var p int
	fmt.Sscanf(port, "%d", &p)
	return p
}

// stringOrFloat reads a config value that may be a string or a numeric (YAML/JSON
// may decode ports as float64), returning fallback when absent.
func stringOrFloat(config map[string]interface{}, key, fallback string) string {
	if v, ok := config[key].(string); ok && v != "" {
		return v
	}
	if v, ok := config[key].(float64); ok {
		return fmt.Sprintf("%.0f", v)
	}
	return fallback
}

// intOrFloat reads a positive integer config value (decoded as float64 or int),
// returning fallback when absent or non-positive.
func intOrFloat(config map[string]interface{}, key string, fallback int) int {
	if v, ok := config[key].(float64); ok && v > 0 {
		return int(v)
	}
	if v, ok := config[key].(int); ok && v > 0 {
		return v
	}
	return fallback
}
