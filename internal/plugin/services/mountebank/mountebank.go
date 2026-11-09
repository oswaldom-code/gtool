package mountebank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
	defaultImage        = "bbyars/mountebank:2.9.1"
	defaultPort         = "2525"
	adminPort           = "2525"
	containerNamePrefix = "gtool-mountebank"
)

// ConfigKeys lists every mock-config key the plugin accepts (aliases included).
func ConfigKeys() []string {
	return []string{"image", "port", "imposters-path", "container-name"}
}

// DefaultPorts returns the host port used for each port key when the config
// does not set it.
func DefaultPorts() map[string]string {
	return map[string]string{
		"port": defaultPort,
	}
}

// MountebankPlugin implements the ServicePlugin interface for Mountebank,
// a service virtualization tool used to mock HTTP/TCP dependencies.
type MountebankPlugin struct {
	docker      *docker.Client
	logger      *zap.Logger
	httpClient  *http.Client
	containerID string
	config      *MountebankConfig
}

// MountebankConfig holds Mountebank-specific configuration
type MountebankConfig struct {
	Image         string `json:"image"`
	Port          string `json:"port"`
	ImpostersPath string `json:"imposters-path"`
	ContainerName string `json:"container-name"`
}

// NewMountebankPlugin creates a new Mountebank service plugin
func NewMountebankPlugin(dockerClient *docker.Client, logger *zap.Logger) *MountebankPlugin {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &MountebankPlugin{
		docker:     dockerClient,
		logger:     logger,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Name returns the service identifier
func (p *MountebankPlugin) Name() string {
	return "mountebank"
}

// imposterDefinitions returns the imposters declared in one file: either a
// single imposter object or Mountebank's {"imposters": [...]} config format.
func imposterDefinitions(content []byte) ([]json.RawMessage, error) {
	var wrapper struct {
		Imposters []json.RawMessage `json:"imposters"`
	}
	if err := json.Unmarshal(content, &wrapper); err != nil {
		return nil, err
	}
	if wrapper.Imposters != nil {
		return wrapper.Imposters, nil
	}
	return []json.RawMessage{content}, nil
}

// imposterPorts returns the port declared by each imposter in dir so the
// container can publish them alongside the admin port. Unreadable or invalid
// files are skipped here; loadImposters reports them when posting.
func imposterPorts(dir string) []string {
	if dir == "" {
		return nil
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))

	var ports []string
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		defs, err := imposterDefinitions(content)
		if err != nil {
			continue
		}
		for _, def := range defs {
			var imposter struct {
				Port int `json:"port"`
			}
			if json.Unmarshal(def, &imposter) == nil && imposter.Port > 0 {
				ports = append(ports, fmt.Sprintf("%d", imposter.Port))
			}
		}
	}
	return ports
}

// portBindings publishes the admin API on adminHostPort and every imposter
// port on the same host port. An imposter that reuses the admin port would
// silently replace its mapping, so both collisions are configuration errors.
func portBindings(adminHostPort string, imposters []string) (map[string]string, error) {
	bindings := map[string]string{adminPort: adminHostPort}
	for _, port := range imposters {
		if port == adminPort || port == adminHostPort {
			return nil, gtErrors.New(gtErrors.ErrConfigInvalid, fmt.Sprintf(
				"imposter port %s collides with the Mountebank admin port (container %s, host %s)",
				port, adminPort, adminHostPort))
		}
		bindings[port] = port
	}
	return bindings, nil
}

// Launch starts the Mountebank service with given configuration
func (p *MountebankPlugin) Launch(ctx context.Context, config map[string]interface{}) error {
	p.logger.Info("launching Mountebank service")

	cfg, err := p.parseConfig(config)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to parse Mountebank configuration")
	}
	p.config = cfg

	p.logger.Info("ensuring Mountebank image", zap.String("image", cfg.Image))
	if err := p.docker.EnsureImage(ctx, cfg.Image); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to get Mountebank image")
	}

	bindings, err := portBindings(cfg.Port, imposterPorts(cfg.ImpostersPath))
	if err != nil {
		return err
	}

	containerConfig := &docker.ContainerConfig{
		Image:        cfg.Image,
		Name:         cfg.ContainerName,
		PortBindings: bindings,
		Labels: map[string]string{
			"managed-by": "gtool",
			"service":    "mountebank",
		},
	}

	p.logger.Info("creating Mountebank container",
		zap.String("name", cfg.ContainerName),
		zap.String("port", cfg.Port))

	containerID, err := p.docker.RunContainer(ctx, containerConfig)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to start Mountebank container")
	}
	p.containerID = containerID

	p.logger.Info("waiting for Mountebank to be ready")
	if err := p.waitForReady(ctx); err != nil {
		// Cleanup on failure
		_ = p.Stop(ctx)
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "Mountebank did not become ready")
	}

	// Load imposters if a path is provided
	if cfg.ImpostersPath != "" {
		p.logger.Info("loading imposters", zap.String("path", cfg.ImpostersPath))
		if err := p.loadImposters(ctx, cfg.ImpostersPath); err != nil {
			_ = p.Stop(context.Background())
			return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to load Mountebank imposters")
		}
	}

	p.logger.Info("Mountebank service launched successfully",
		zap.String("containerID", containerID),
		zap.String("port", cfg.Port))

	return nil
}

// IsReady checks if the Mountebank admin API is accepting requests
func (p *MountebankPlugin) IsReady(ctx context.Context) (bool, error) {
	if p.containerID == "" {
		return false, gtErrors.New(gtErrors.ErrServiceNotRunning, "Mountebank container not started")
	}

	running, err := p.docker.IsContainerRunning(ctx, p.containerID)
	if err != nil {
		return false, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to check container status")
	}

	if !running {
		return false, nil
	}

	// Probe the admin API root; a 200 means Mountebank is serving requests
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.adminURL(), nil)
	if err != nil {
		return false, gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to build readiness request")
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Debug("Mountebank not ready yet", zap.Error(err))
		return false, nil
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode == http.StatusOK, nil
}

// Stop terminates the Mountebank service
func (p *MountebankPlugin) Stop(ctx context.Context) error {
	if p.containerID == "" {
		if p.docker == nil {
			p.logger.Debug("no container ID and no Docker client")
			return nil
		}

		p.logger.Info("no container ID, searching by labels")

		containers, err := p.docker.ListContainersByLabels(ctx, map[string]string{
			"managed-by": "gtool",
			"service":    "mountebank",
		})

		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list Mountebank containers")
		}

		if len(containers) == 0 {
			p.logger.Warn("no Mountebank containers found to stop")
			return nil
		}

		var firstErr error
		for _, container := range containers {
			p.containerID = container.ID
			p.logger.Info("found Mountebank container",
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
func (p *MountebankPlugin) stopContainer(ctx context.Context) error {
	p.logger.Info("stopping Mountebank service", zap.String("containerID", p.containerID))

	timeout := 10
	if err := p.docker.StopContainer(ctx, p.containerID, &timeout); err != nil {
		p.logger.Error("failed to stop container", zap.Error(err))
		// Continue to remove anyway
	}

	if err := p.docker.RemoveContainer(ctx, p.containerID, true); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to remove Mountebank container")
	}

	p.logger.Info("Mountebank service stopped successfully")
	p.containerID = ""
	p.config = nil
	return nil
}

// GetConnectionInfo returns connection details
func (p *MountebankPlugin) GetConnectionInfo() (*plugin.ConnectionInfo, error) {
	if p.config == nil {
		return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "Mountebank service not launched")
	}

	return &plugin.ConnectionInfo{
		Host:     "localhost",
		Port:     mustParsePort(p.config.Port),
		Protocol: "http",
		Metadata: map[string]string{
			"admin-url": p.adminURL(),
		},
	}, nil
}

// GetLogs retrieves service logs
func (p *MountebankPlugin) GetLogs(ctx context.Context, opts *plugin.LogOptions) ([]string, error) {
	containerID := p.containerID

	if containerID == "" {
		if p.docker == nil {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "no container ID and no Docker client")
		}

		p.logger.Info("no container ID for logs, searching by labels")

		containers, err := p.docker.ListContainersByLabels(ctx, map[string]string{
			"managed-by": "gtool",
			"service":    "mountebank",
		})

		if err != nil {
			return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list Mountebank containers")
		}

		if len(containers) == 0 {
			return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "Mountebank container not found")
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
		p.logger.Info("found Mountebank container for logs",
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

// parseConfig parses the configuration map into MountebankConfig
func (p *MountebankPlugin) parseConfig(config map[string]interface{}) (*MountebankConfig, error) {
	cfg := &MountebankConfig{
		Image:         defaultImage,
		Port:          defaultPort,
		ContainerName: fmt.Sprintf("%s-%d", containerNamePrefix, time.Now().Unix()),
	}

	cfg.Image = svcconfig.String(config, "image", cfg.Image)
	cfg.Port = svcconfig.Port(config, "port", cfg.Port)
	cfg.ImpostersPath = svcconfig.String(config, "imposters-path", cfg.ImpostersPath)
	cfg.ContainerName = svcconfig.String(config, "container-name", cfg.ContainerName)

	return cfg, nil
}

// waitForReady waits for Mountebank to be ready
func (p *MountebankPlugin) waitForReady(ctx context.Context) error {
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
			p.logger.Info("Mountebank is ready", zap.Int("attempts", i+1))
			return nil
		}

		select {
		case <-ctx.Done():
			return gtErrors.New(gtErrors.ErrServiceFailed, "context cancelled while waiting for Mountebank")
		case <-time.After(interval):
			// Continue to next attempt
		}
	}

	return gtErrors.New(gtErrors.ErrServiceFailed,
		fmt.Sprintf("Mountebank did not become ready after %d attempts", maxRetries))
}

// loadImposters posts every imposter definition found in the given directory
// to the Mountebank admin API.
func (p *MountebankPlugin) loadImposters(ctx context.Context, impostersPath string) error {
	if _, err := os.Stat(impostersPath); os.IsNotExist(err) {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid,
			fmt.Sprintf("imposters path does not exist: %s", impostersPath))
	}

	files, err := filepath.Glob(filepath.Join(impostersPath, "*.json"))
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to list imposter files")
	}

	if len(files) == 0 {
		p.logger.Warn("no imposter files found in path", zap.String("path", impostersPath))
		return nil
	}

	p.logger.Info("found imposter files to load",
		zap.Int("count", len(files)),
		zap.Strings("files", files))

	for _, file := range files {
		if err := p.postImposter(ctx, file); err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
				fmt.Sprintf("failed to load imposter: %s", file))
		}
	}

	return nil
}

func (p *MountebankPlugin) createImposter(ctx context.Context, def json.RawMessage) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.adminURL()+"imposters", bytes.NewReader(def))
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to build imposter request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to post imposter")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return gtErrors.New(gtErrors.ErrServiceFailed, fmt.Sprintf("unexpected status posting imposter: %d", resp.StatusCode))
	}
	return nil
}

// postImposter reads an imposter definition file and posts it to the admin API
func (p *MountebankPlugin) postImposter(ctx context.Context, path string) error {
	p.logger.Info("loading imposter", zap.String("file", path))

	content, err := os.ReadFile(path)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to read imposter file")
	}
	defs, err := imposterDefinitions(content)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, fmt.Sprintf("invalid imposter file %s", path))
	}

	for _, def := range defs {
		if err := p.createImposter(ctx, def); err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, fmt.Sprintf("failed to load an imposter from %s", path))
		}
	}

	p.logger.Info("imposter loaded successfully", zap.String("file", path))
	return nil
}

// adminURL builds the base URL of the Mountebank admin API
func (p *MountebankPlugin) adminURL() string {
	return fmt.Sprintf("http://localhost:%s/", p.config.Port)
}

// mustParsePort parses port string to int, returns 0 on error
func mustParsePort(port string) int {
	var p int
	if _, err := fmt.Sscanf(port, "%d", &p); err != nil {
		return 0
	}
	return p
}
