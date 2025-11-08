package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"

	gtErrors "github.com/oswaldo-montano/gtool/pkg/errors"
)

type Client struct {
	cli    *client.Client
	logger *zap.Logger
}

type ContainerConfig struct {
	Image        string
	Name         string
	Env          []string
	PortBindings map[string]string
	Mounts       []Mount
	NetworkMode  string
	AutoRemove   bool
	Labels       map[string]string
}

type Mount struct {
	Type     string
	Source   string
	Target   string
	ReadOnly bool
}

type ExecConfig struct {
	Cmd          []string
	AttachStdout bool
	AttachStderr bool
	WorkingDir   string
	Env          []string
}

func NewClient(logger *zap.Logger) (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to create Docker client")
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &Client{
		cli:    cli,
		logger: logger,
	}, nil
}

func (c *Client) Close() error {
	return c.cli.Close()
}

func (c *Client) PullImage(ctx context.Context, imageName string) error {
	c.logger.Info("pulling Docker image", zap.String("image", imageName))

	reader, err := c.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, fmt.Sprintf("failed to pull image %s", imageName))
	}
	defer reader.Close()

	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to read pull response")
	}

	c.logger.Info("successfully pulled image", zap.String("image", imageName))
	return nil
}

func (c *Client) CreateContainer(ctx context.Context, config *ContainerConfig) (string, error) {
	c.logger.Info("creating container",
		zap.String("image", config.Image),
		zap.String("name", config.Name))

	// Build port bindings
	portBindings := nat.PortMap{}
	exposedPorts := nat.PortSet{}
	for containerPort, hostPort := range config.PortBindings {
		port, err := nat.NewPort("tcp", containerPort)
		if err != nil {
			return "", gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "invalid port specification")
		}
		exposedPorts[port] = struct{}{}
		portBindings[port] = []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: hostPort,
			},
		}
	}

	// Build mounts
	var mounts []mount.Mount
	for _, m := range config.Mounts {
		mounts = append(mounts, mount.Mount{
			Type:     mount.Type(m.Type),
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}

	// Create container
	containerConfig := &container.Config{
		Image:        config.Image,
		Env:          config.Env,
		ExposedPorts: exposedPorts,
		Labels:       config.Labels,
	}

	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		Mounts:       mounts,
		AutoRemove:   config.AutoRemove,
	}

	if config.NetworkMode != "" {
		hostConfig.NetworkMode = container.NetworkMode(config.NetworkMode)
	}

	resp, err := c.cli.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		&network.NetworkingConfig{},
		nil,
		config.Name,
	)
	if err != nil {
		return "", gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to create container")
	}

	c.logger.Info("container created",
		zap.String("containerID", resp.ID),
		zap.String("name", config.Name))

	return resp.ID, nil
}

func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	c.logger.Info("starting container", zap.String("containerID", containerID))

	if err := c.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to start container")
	}

	c.logger.Info("container started", zap.String("containerID", containerID))
	return nil
}

func (c *Client) StopContainer(ctx context.Context, containerID string, timeout *int) error {
	c.logger.Info("stopping container", zap.String("containerID", containerID))

	stopTimeout := 10
	if timeout != nil {
		stopTimeout = *timeout
	}

	if err := c.cli.ContainerStop(ctx, containerID, container.StopOptions{
		Timeout: &stopTimeout,
	}); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to stop container")
	}

	c.logger.Info("container stopped", zap.String("containerID", containerID))
	return nil
}

func (c *Client) RemoveContainer(ctx context.Context, containerID string, force bool) error {
	c.logger.Info("removing container",
		zap.String("containerID", containerID),
		zap.Bool("force", force))

	err := c.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force:         force,
		RemoveVolumes: true,
	})
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to remove container")
	}

	c.logger.Info("container removed", zap.String("containerID", containerID))
	return nil
}

func (c *Client) GetContainerLogs(ctx context.Context, containerID string, tail int) (string, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", tail),
	}

	reader, err := c.cli.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return "", gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to get container logs")
	}
	defer reader.Close()

	// Docker logs come with 8-byte headers, use stdcopy to demultiplex
	var stdout, stderr bytes.Buffer
	written, err := stdcopy.StdCopy(&stdout, &stderr, reader)

	c.logger.Debug("log demultiplex result",
		zap.Int64("bytes_written", written),
		zap.Int("stdout_len", stdout.Len()),
		zap.Int("stderr_len", stderr.Len()),
		zap.Error(err))

	// If stdcopy didn't read anything or failed, fall back to raw read
	if err != nil || written == 0 {
		if err != nil {
			c.logger.Warn("failed to demultiplex logs, reading raw", zap.Error(err))
		} else {
			c.logger.Debug("stdcopy read 0 bytes, trying raw read")
		}

		// Reopen reader
		reader2, err2 := c.cli.ContainerLogs(ctx, containerID, options)
		if err2 != nil {
			return "", gtErrors.Wrap(err2, gtErrors.ErrDockerFailed, "failed to get container logs (retry)")
		}
		defer reader2.Close()

		logs, err3 := io.ReadAll(reader2)
		if err3 != nil {
			return "", gtErrors.Wrap(err3, gtErrors.ErrDockerFailed, "failed to read container logs")
		}
		c.logger.Debug("raw read result", zap.Int("bytes", len(logs)))
		return string(logs), nil
	}

	// Combine stdout and stderr
	combined := stdout.String() + stderr.String()
	c.logger.Debug("combined logs length", zap.Int("length", len(combined)))
	return combined, nil
}

func (c *Client) ListContainersByLabels(ctx context.Context, labels map[string]string) ([]types.Container, error) {
	// Build filter string
	filters := filters.NewArgs()
	for key, value := range labels {
		filters.Add("label", fmt.Sprintf("%s=%s", key, value))
	}

	c.logger.Debug("listing containers by labels", zap.Any("labels", labels))

	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters,
	})
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to list containers")
	}

	c.logger.Debug("found containers", zap.Int("count", len(containers)))
	return containers, nil
}

func (c *Client) ExecInContainer(ctx context.Context, containerID string, config *ExecConfig) (string, error) {
	c.logger.Info("executing command in container",
		zap.String("containerID", containerID),
		zap.Strings("cmd", config.Cmd))

	execConfig := types.ExecConfig{
		AttachStdout: config.AttachStdout,
		AttachStderr: config.AttachStderr,
		Cmd:          config.Cmd,
		WorkingDir:   config.WorkingDir,
		Env:          config.Env,
	}

	execID, err := c.cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to create exec instance")
	}

	resp, err := c.cli.ContainerExecAttach(ctx, execID.ID, types.ExecStartCheck{})
	if err != nil {
		return "", gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to attach to exec instance")
	}
	defer resp.Close()

	output, err := io.ReadAll(resp.Reader)
	if err != nil {
		return "", gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to read exec output")
	}

	// Check exec exit code
	inspectResp, err := c.cli.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return string(output), gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to inspect exec instance")
	}

	if inspectResp.ExitCode != 0 {
		return string(output), gtErrors.New(gtErrors.ErrDockerFailed,
			fmt.Sprintf("command exited with code %d: %s", inspectResp.ExitCode, string(output)))
	}

	return string(output), nil
}

func (c *Client) WaitForContainer(ctx context.Context, containerID string, condition container.WaitCondition) error {
	c.logger.Info("waiting for container",
		zap.String("containerID", containerID),
		zap.String("condition", string(condition)))

	statusCh, errCh := c.cli.ContainerWait(ctx, containerID, condition)

	select {
	case err := <-errCh:
		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "error waiting for container")
		}
	case <-statusCh:
		c.logger.Info("container reached desired state", zap.String("containerID", containerID))
	case <-ctx.Done():
		return gtErrors.New(gtErrors.ErrDockerFailed, "context cancelled while waiting for container")
	}

	return nil
}

func (c *Client) InspectContainer(ctx context.Context, containerID string) (*types.ContainerJSON, error) {
	inspect, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to inspect container")
	}
	return &inspect, nil
}

func (c *Client) IsContainerRunning(ctx context.Context, containerID string) (bool, error) {
	inspect, err := c.InspectContainer(ctx, containerID)
	if err != nil {
		return false, err
	}
	return inspect.State.Running, nil
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to ping Docker daemon")
	}
	return nil
}

func (c *Client) CopyToContainer(ctx context.Context, containerID, targetPath string, content io.Reader) error {
	c.logger.Info("copying to container",
		zap.String("containerID", containerID),
		zap.String("targetPath", targetPath))

	err := c.cli.CopyToContainer(ctx, containerID, targetPath, content, types.CopyToContainerOptions{})
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "failed to copy to container")
	}

	return nil
}

func (c *Client) WaitForHealthy(ctx context.Context, containerID string, interval time.Duration, retries int) error {
	c.logger.Info("waiting for container to be healthy",
		zap.String("containerID", containerID),
		zap.Duration("interval", interval),
		zap.Int("retries", retries))

	for i := 0; i < retries; i++ {
		inspect, err := c.InspectContainer(ctx, containerID)
		if err != nil {
			return err
		}

		if inspect.State.Running {
			// If no health check is defined, just check if running
			if inspect.State.Health == nil {
				c.logger.Info("container is running (no health check defined)",
					zap.String("containerID", containerID))
				return nil
			}

			// Check health status
			if inspect.State.Health.Status == "healthy" {
				c.logger.Info("container is healthy", zap.String("containerID", containerID))
				return nil
			}

			c.logger.Debug("container not healthy yet",
				zap.String("containerID", containerID),
				zap.String("status", inspect.State.Health.Status),
				zap.Int("attempt", i+1))
		} else {
			c.logger.Debug("container not running",
				zap.String("containerID", containerID),
				zap.Int("attempt", i+1))
		}

		select {
		case <-ctx.Done():
			return gtErrors.New(gtErrors.ErrDockerFailed, "context cancelled while waiting for healthy state")
		case <-time.After(interval):
			// Continue to next attempt
		}
	}

	return gtErrors.New(gtErrors.ErrDockerFailed,
		fmt.Sprintf("container did not become healthy after %d attempts", retries))
}
