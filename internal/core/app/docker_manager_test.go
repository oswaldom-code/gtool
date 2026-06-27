package app

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oswaldo-montano/gtool/internal/infra/docker"
	"github.com/oswaldo-montano/gtool/internal/plugin"
	gtErrors "github.com/oswaldo-montano/gtool/pkg/errors"
)

// fakeDocker is a controllable DockerClient for the app manager tests.
type fakeDocker struct {
	containers []types.Container
	listErr    error
	pullErr    error
	createErr  error
	startErr   error
	removeErr  error
	logs       string
	logsErr    error

	created   *docker.ContainerConfig
	pulled    string
	stopped   []string
	removed   []string
	startedID string
}

func (d *fakeDocker) PullImage(_ context.Context, image string) error {
	d.pulled = image
	return d.pullErr
}

func (d *fakeDocker) CreateContainer(_ context.Context, cfg *docker.ContainerConfig) (string, error) {
	d.created = cfg
	if d.createErr != nil {
		return "", d.createErr
	}
	return "container-id", nil
}

func (d *fakeDocker) StartContainer(_ context.Context, id string) error {
	d.startedID = id
	return d.startErr
}

func (d *fakeDocker) StopContainer(_ context.Context, id string, _ *int) error {
	d.stopped = append(d.stopped, id)
	return nil
}

func (d *fakeDocker) RemoveContainer(_ context.Context, id string, _ bool) error {
	d.removed = append(d.removed, id)
	return d.removeErr
}

func (d *fakeDocker) ListContainersByLabels(_ context.Context, _ map[string]string) ([]types.Container, error) {
	return d.containers, d.listErr
}

func (d *fakeDocker) GetContainerLogs(_ context.Context, _ string, _ int) (string, error) {
	return d.logs, d.logsErr
}

func TestDockerManager_Start(t *testing.T) {
	ctx := context.Background()

	t.Run("starts the application", func(t *testing.T) {
		d := &fakeDocker{}
		m := NewDockerManager(d, nil)

		err := m.Start(ctx, &plugin.AppConfig{
			DockerImage: "myapp:latest",
			Port:        8080,
			Environment: map[string]string{"FOO": "bar"},
		})

		require.NoError(t, err)
		assert.Equal(t, "myapp:latest", d.pulled)
		assert.Equal(t, "container-id", d.startedID)
		assert.Equal(t, "myapp:latest", d.created.Image)
		assert.Equal(t, map[string]string{"8080": "8080"}, d.created.PortBindings)
		assert.Contains(t, d.created.Env, "FOO=bar")
		assert.Equal(t, "app", d.created.Labels["gtool-role"])
	})

	t.Run("requires docker image", func(t *testing.T) {
		m := NewDockerManager(&fakeDocker{}, nil)

		err := m.Start(ctx, &plugin.AppConfig{})

		require.Error(t, err)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrInvalidArgument))
	})

	t.Run("fails when already running", func(t *testing.T) {
		d := &fakeDocker{containers: []types.Container{{ID: "x", State: "running"}}}
		m := NewDockerManager(d, nil)

		err := m.Start(ctx, &plugin.AppConfig{DockerImage: "myapp:latest"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "already running")
	})

	t.Run("wraps pull failure", func(t *testing.T) {
		d := &fakeDocker{pullErr: errors.New("boom")}
		m := NewDockerManager(d, nil)

		err := m.Start(ctx, &plugin.AppConfig{DockerImage: "myapp:latest"})

		require.Error(t, err)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrDockerFailed))
	})
}

func TestDockerManager_Stop(t *testing.T) {
	ctx := context.Background()

	t.Run("stops and removes the container", func(t *testing.T) {
		d := &fakeDocker{containers: []types.Container{{ID: "c1", State: "running"}}}
		m := NewDockerManager(d, nil)

		require.NoError(t, m.Stop(ctx))
		assert.Equal(t, []string{"c1"}, d.stopped)
		assert.Equal(t, []string{"c1"}, d.removed)
	})

	t.Run("errors when nothing running", func(t *testing.T) {
		m := NewDockerManager(&fakeDocker{}, nil)

		err := m.Stop(ctx)
		require.Error(t, err)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrServiceNotRunning))
	})
}

func TestDockerManager_Restart(t *testing.T) {
	ctx := context.Background()

	t.Run("starts when nothing was running", func(t *testing.T) {
		d := &fakeDocker{} // no containers -> Stop returns not-running, restart ignores it
		m := NewDockerManager(d, nil)

		err := m.Restart(ctx, &plugin.AppConfig{DockerImage: "myapp:latest"})
		require.NoError(t, err)
		assert.Equal(t, "myapp:latest", d.pulled)
	})
}

func TestDockerManager_Status(t *testing.T) {
	ctx := context.Background()

	t.Run("running with port", func(t *testing.T) {
		d := &fakeDocker{containers: []types.Container{{
			State: "running",
			Image: "myapp:latest",
			Ports: []types.Port{{PublicPort: 8080}},
		}}}
		m := NewDockerManager(d, nil)

		st, err := m.Status(ctx)
		require.NoError(t, err)
		assert.True(t, st.Running)
		assert.Equal(t, "running", st.State)
		assert.Equal(t, "myapp:latest", st.Image)
		assert.Equal(t, 8080, st.Port)
	})

	t.Run("skips unmapped exposed ports", func(t *testing.T) {
		d := &fakeDocker{containers: []types.Container{{
			State: "running",
			Image: "nginx:alpine",
			Ports: []types.Port{{PrivatePort: 80}, {PublicPort: 8080}},
		}}}
		m := NewDockerManager(d, nil)

		st, err := m.Status(ctx)
		require.NoError(t, err)
		assert.Equal(t, 8080, st.Port, "should report the host-published port, not the exposed one")
	})

	t.Run("not found", func(t *testing.T) {
		m := NewDockerManager(&fakeDocker{}, nil)

		st, err := m.Status(ctx)
		require.NoError(t, err)
		assert.False(t, st.Running)
		assert.Equal(t, "not-found", st.State)
	})
}

func TestDockerManager_Logs(t *testing.T) {
	ctx := context.Background()

	t.Run("returns log lines", func(t *testing.T) {
		d := &fakeDocker{
			containers: []types.Container{{ID: "c1"}},
			logs:       "line1\nline2\n",
		}
		m := NewDockerManager(d, nil)

		logs, err := m.Logs(ctx, 100)
		require.NoError(t, err)
		assert.Equal(t, []string{"line1", "line2"}, logs)
	})

	t.Run("errors when nothing running", func(t *testing.T) {
		m := NewDockerManager(&fakeDocker{}, nil)

		_, err := m.Logs(ctx, 100)
		require.Error(t, err)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrServiceNotRunning))
	})
}
