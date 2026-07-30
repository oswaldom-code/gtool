package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/internal/plugin"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
	"go.uber.org/zap"
)

const appContainerPrefix = "gtool-app"

// appLabels identify the application container managed by gtool. They are
// distinct from the mock service labels so the two never collide.
func appLabels() map[string]string {
	return map[string]string{
		"managed-by": "gtool",
		"gtool-role": "app",
	}
}

// DockerClient is the subset of *docker.Client used by the DockerManager.
type DockerClient interface {
	PullImage(ctx context.Context, image string) error
	CreateContainer(ctx context.Context, config *docker.ContainerConfig) (string, error)
	StartContainer(ctx context.Context, containerID string) error
	StopContainer(ctx context.Context, containerID string, timeout *int) error
	RemoveContainer(ctx context.Context, containerID string, force bool) error
	ListContainersByLabels(ctx context.Context, labels map[string]string) ([]types.Container, error)
	GetContainerLogs(ctx context.Context, containerID string, tail int) (string, error)
}

// DockerManager runs the application under test as a Docker container. Like the
// mock services, the container is labelled so separate CLI invocations can find
// it again.
type DockerManager struct {
	docker DockerClient
	logger *zap.Logger
}

// NewDockerManager creates a Docker-backed app manager.
func NewDockerManager(dockerClient DockerClient, logger *zap.Logger) *DockerManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DockerManager{docker: dockerClient, logger: logger}
}

// AppStatus describes the current state of the application container.
type AppStatus struct {
	Running bool
	State   string
	Image   string
	Port    int
}

// Start launches the application container. It fails if an application is
// already running.
func (m *DockerManager) Start(ctx context.Context, cfg *plugin.AppConfig) error {
	if cfg.DockerImage == "" {
		return gtErrors.New(gtErrors.ErrInvalidArgument, "docker-image is required to start the application")
	}

	running, err := m.docker.ListContainersByLabels(ctx, appLabels())
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to check for running application")
	}
	if len(running) > 0 {
		return gtErrors.New(gtErrors.ErrServiceFailed, "application is already running")
	}

	m.logger.Info("pulling application image", zap.String("image", cfg.DockerImage))
	if err := m.docker.PullImage(ctx, cfg.DockerImage); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to pull application image")
	}

	containerConfig := &docker.ContainerConfig{
		Image:        cfg.DockerImage,
		Name:         fmt.Sprintf("%s-%d", appContainerPrefix, time.Now().Unix()),
		Env:          envSlice(cfg.Environment),
		PortBindings: portBindings(cfg.Port),
		Labels:       appLabels(),
	}

	m.logger.Info("creating application container",
		zap.String("image", cfg.DockerImage),
		zap.Int("port", cfg.Port))

	containerID, err := m.docker.CreateContainer(ctx, containerConfig)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to create application container")
	}

	if err := m.docker.StartContainer(ctx, containerID); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to start application container")
	}

	m.logger.Info("application started", zap.String("containerID", containerID))
	return nil
}

// Stop stops and removes the application container.
func (m *DockerManager) Stop(ctx context.Context) error {
	containers, err := m.docker.ListContainersByLabels(ctx, appLabels())
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list application containers")
	}
	if len(containers) == 0 {
		return gtErrors.New(gtErrors.ErrServiceNotRunning, "no application is running")
	}

	timeout := 10
	for _, c := range containers {
		m.logger.Info("stopping application container", zap.String("containerID", c.ID))
		if err := m.docker.StopContainer(ctx, c.ID, &timeout); err != nil {
			m.logger.Error("failed to stop container", zap.Error(err), zap.String("containerID", c.ID))
		}
		if err := m.docker.RemoveContainer(ctx, c.ID, true); err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to remove application container")
		}
	}
	return nil
}

// Restart stops the running application (if any) and starts it again.
func (m *DockerManager) Restart(ctx context.Context, cfg *plugin.AppConfig) error {
	if err := m.Stop(ctx); err != nil && !gtErrors.Is(err, gtErrors.ErrServiceNotRunning) {
		return err
	}
	return m.Start(ctx, cfg)
}

// Status reports the state of the application container.
func (m *DockerManager) Status(ctx context.Context) (*AppStatus, error) {
	containers, err := m.docker.ListContainersByLabels(ctx, appLabels())
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list application containers")
	}
	if len(containers) == 0 {
		return &AppStatus{Running: false, State: "not-found"}, nil
	}

	c := containers[0]
	status := &AppStatus{
		State:   c.State,
		Image:   c.Image,
		Running: c.State == "running",
	}
	// A container may expose unmapped ports (PublicPort 0) alongside the
	// published one; report the first port actually bound to the host.
	for _, p := range c.Ports {
		if p.PublicPort > 0 {
			status.Port = int(p.PublicPort)
			break
		}
	}
	return status, nil
}

// Logs returns the last tail lines of the application container logs.
func (m *DockerManager) Logs(ctx context.Context, tail int) ([]string, error) {
	containers, err := m.docker.ListContainersByLabels(ctx, appLabels())
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list application containers")
	}
	if len(containers) == 0 {
		return nil, gtErrors.New(gtErrors.ErrServiceNotRunning, "no application is running")
	}

	logs, err := m.docker.GetContainerLogs(ctx, containers[0].ID, tail)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to get application logs")
	}
	return strings.Split(strings.TrimSpace(logs), "\n"), nil
}

// envSlice converts an environment map to KEY=VALUE entries.
func envSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

// portBindings maps the application port to the same host port, if set.
func portBindings(port int) map[string]string {
	if port <= 0 {
		return nil
	}
	p := strconv.Itoa(port)
	return map[string]string{p: p}
}
