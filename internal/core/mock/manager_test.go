package mock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oswaldom-code/gtool/internal/plugin"
	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

// readyResult models a single IsReady response. The last element repeats.
type readyResult struct {
	ready bool
	err   error
}

// fakePlugin is a controllable ServicePlugin for testing the Manager.
type fakePlugin struct {
	name string

	launchErr   error
	launchCalls int

	stopErr   error
	stopCalls int

	readyResults []readyResult
	readyIdx     int

	connInfo *plugin.ConnectionInfo
	connErr  error

	logs    []string
	logsErr error
}

func (f *fakePlugin) Name() string { return f.name }

func (f *fakePlugin) Launch(_ context.Context, _ map[string]interface{}) error {
	f.launchCalls++
	return f.launchErr
}

func (f *fakePlugin) IsReady(_ context.Context) (bool, error) {
	if len(f.readyResults) == 0 {
		return true, nil
	}
	idx := f.readyIdx
	if idx >= len(f.readyResults) {
		idx = len(f.readyResults) - 1
	}
	f.readyIdx++
	r := f.readyResults[idx]
	return r.ready, r.err
}

func (f *fakePlugin) Stop(_ context.Context) error {
	f.stopCalls++
	return f.stopErr
}

func (f *fakePlugin) GetConnectionInfo() (*plugin.ConnectionInfo, error) {
	return f.connInfo, f.connErr
}

func (f *fakePlugin) GetLogs(_ context.Context, _ *plugin.LogOptions) ([]string, error) {
	return f.logs, f.logsErr
}

// fakeDocker is a controllable DockerClient.
type fakeDocker struct {
	containers []types.Container
	err        error
	calls      int
}

func (d *fakeDocker) ListContainersByLabels(_ context.Context, _ map[string]string) ([]types.Container, error) {
	d.calls++
	return d.containers, d.err
}

// fastOrchestration returns config that makes health-check loops finish quickly.
func fastOrchestration(parallel bool) config.OrchestrationConfig {
	return config.OrchestrationConfig{
		ParallelMocks:       parallel,
		HealthCheckInterval: time.Millisecond,
		HealthCheckRetries:  3,
		CleanupOnFailure:    true,
	}
}

func newRegistryWith(t *testing.T, plugins ...plugin.ServicePlugin) *plugin.Registry {
	t.Helper()
	reg := plugin.NewRegistry()
	for _, p := range plugins {
		require.NoError(t, reg.RegisterService(p))
	}
	return reg
}

func TestManager_Start(t *testing.T) {
	ctx := context.Background()

	t.Run("starts service and stores state", func(t *testing.T) {
		p := &fakePlugin{name: "postgresql"}
		m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), nil)

		err := m.Start(ctx, "postgresql", nil)

		require.NoError(t, err)
		assert.Equal(t, 1, p.launchCalls)
		assert.Equal(t, []string{"postgresql"}, m.ListRunning())
	})

	t.Run("idempotent when already running", func(t *testing.T) {
		p := &fakePlugin{name: "postgresql"}
		m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), nil)
		require.NoError(t, m.Start(ctx, "postgresql", nil))

		err := m.Start(ctx, "postgresql", nil)

		require.NoError(t, err)
		assert.Equal(t, 1, p.launchCalls, "should not relaunch an already running service")
	})

	t.Run("service not in registry", func(t *testing.T) {
		m := NewManager(plugin.NewRegistry(), nil, fastOrchestration(false), nil)

		err := m.Start(ctx, "unknown", nil)

		require.Error(t, err)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrServiceFailed))
	})

	t.Run("launch failure is wrapped", func(t *testing.T) {
		p := &fakePlugin{name: "postgresql", launchErr: errors.New("boom")}
		m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), nil)

		err := m.Start(ctx, "postgresql", nil)

		require.Error(t, err)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrServiceFailed))
	})

	t.Run("never ready triggers cleanup and ErrServiceNotReady", func(t *testing.T) {
		p := &fakePlugin{name: "postgresql", readyResults: []readyResult{{ready: false}}}
		m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), nil)

		err := m.Start(ctx, "postgresql", nil)

		require.Error(t, err)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrServiceNotReady))
		assert.Equal(t, 1, p.stopCalls, "CleanupOnFailure should stop the service")
		assert.Empty(t, m.ListRunning())
	})

	t.Run("context cancelled while waiting", func(t *testing.T) {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		p := &fakePlugin{name: "postgresql", readyResults: []readyResult{{ready: false}}}
		orch := fastOrchestration(false)
		orch.HealthCheckInterval = time.Hour // ensure ctx.Done wins the select
		m := NewManager(newRegistryWith(t, p), nil, orch, nil)

		err := m.Start(cancelled, "postgresql", nil)

		require.Error(t, err)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrServiceFailed))
	})

	t.Run("becomes ready after a few attempts", func(t *testing.T) {
		p := &fakePlugin{name: "postgresql", readyResults: []readyResult{
			{ready: false}, {ready: false}, {ready: true},
		}}
		orch := fastOrchestration(false)
		orch.HealthCheckRetries = 5
		m := NewManager(newRegistryWith(t, p), nil, orch, nil)

		err := m.Start(ctx, "postgresql", nil)

		require.NoError(t, err)
		assert.Equal(t, []string{"postgresql"}, m.ListRunning())
	})
}

func TestManager_Stop(t *testing.T) {
	ctx := context.Background()

	t.Run("stops in-memory service and removes it", func(t *testing.T) {
		p := &fakePlugin{name: "postgresql"}
		m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), nil)
		require.NoError(t, m.Start(ctx, "postgresql", nil))

		err := m.Stop(ctx, "postgresql")

		require.NoError(t, err)
		assert.Equal(t, 1, p.stopCalls)
		assert.Empty(t, m.ListRunning())
	})

	t.Run("plugin stop failure is wrapped and keeps state with error", func(t *testing.T) {
		p := &fakePlugin{name: "postgresql", stopErr: errors.New("boom")}
		m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), nil)
		require.NoError(t, m.Start(ctx, "postgresql", nil))

		err := m.Stop(ctx, "postgresql")

		require.Error(t, err)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrServiceFailed))
	})

	t.Run("not in memory, no container in docker", func(t *testing.T) {
		p := &fakePlugin{name: "postgresql"}
		dock := &fakeDocker{containers: nil}
		m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), dock)

		err := m.Stop(ctx, "postgresql")

		require.Error(t, err)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrServiceNotRunning))
	})

	t.Run("not in memory, container exists, stops via plugin", func(t *testing.T) {
		p := &fakePlugin{name: "postgresql"}
		dock := &fakeDocker{containers: []types.Container{{State: "running"}}}
		m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), dock)

		err := m.Stop(ctx, "postgresql")

		require.NoError(t, err)
		assert.Equal(t, 1, p.stopCalls)
	})

	t.Run("not in memory and not in registry", func(t *testing.T) {
		dock := &fakeDocker{containers: []types.Container{{State: "running"}}}
		m := NewManager(plugin.NewRegistry(), nil, fastOrchestration(false), dock)

		err := m.Stop(ctx, "unknown")

		require.Error(t, err)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrServiceFailed))
	})
}

func TestManager_StopAll(t *testing.T) {
	ctx := context.Background()
	p1 := &fakePlugin{name: "postgresql"}
	p2 := &fakePlugin{name: "kafka"}
	m := NewManager(newRegistryWith(t, p1, p2), nil, fastOrchestration(false), nil)
	require.NoError(t, m.Start(ctx, "postgresql", nil))
	require.NoError(t, m.Start(ctx, "kafka", nil))

	require.NoError(t, m.StopAll(ctx))

	assert.Equal(t, 1, p1.stopCalls)
	assert.Equal(t, 1, p2.stopCalls)
	assert.Empty(t, m.ListRunning())
}

func TestManager_GetStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("in-memory running reports running with port", func(t *testing.T) {
		p := &fakePlugin{
			name:     "postgresql",
			connInfo: &plugin.ConnectionInfo{Port: 5432},
		}
		m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), nil)
		require.NoError(t, m.Start(ctx, "postgresql", nil))

		status, err := m.GetStatus(ctx, "postgresql")

		require.NoError(t, err)
		assert.Equal(t, "running", status.Status)
		assert.Equal(t, 5432, status.Port)
	})

	t.Run("in-memory with error reports error", func(t *testing.T) {
		// Stop failure leaves the service in memory with its error set.
		p := &fakePlugin{name: "postgresql", stopErr: errors.New("boom")}
		m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), nil)
		require.NoError(t, m.Start(ctx, "postgresql", nil))
		_ = m.Stop(ctx, "postgresql")

		status, err := m.GetStatus(ctx, "postgresql")

		require.NoError(t, err)
		assert.Equal(t, "error", status.Status)
		assert.Equal(t, "boom", status.Error)
	})

	t.Run("in-memory not ready reports error", func(t *testing.T) {
		p := &fakePlugin{name: "postgresql", readyResults: []readyResult{
			{ready: true},  // Start
			{ready: false}, // GetStatus
		}}
		m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), nil)
		require.NoError(t, m.Start(ctx, "postgresql", nil))

		status, err := m.GetStatus(ctx, "postgresql")

		require.NoError(t, err)
		assert.Equal(t, "error", status.Status)
	})

	t.Run("not in memory, found running in docker", func(t *testing.T) {
		dock := &fakeDocker{containers: []types.Container{{
			State:   "running",
			Created: time.Now().Add(-time.Minute).Unix(),
			Ports:   []types.Port{{PublicPort: 5432}},
		}}}
		m := NewManager(plugin.NewRegistry(), nil, fastOrchestration(false), dock)

		status, err := m.GetStatus(ctx, "postgresql")

		require.NoError(t, err)
		assert.Equal(t, "running", status.Status)
		assert.Equal(t, 5432, status.Port)
		assert.Greater(t, status.Uptime, time.Duration(0))
	})

	t.Run("not in memory, not in docker reports stopped", func(t *testing.T) {
		m := NewManager(plugin.NewRegistry(), nil, fastOrchestration(false), &fakeDocker{})

		status, err := m.GetStatus(ctx, "postgresql")

		require.NoError(t, err)
		assert.Equal(t, "stopped", status.Status)
	})
}

func TestManager_GetAllStatuses(t *testing.T) {
	ctx := context.Background()
	p := &fakePlugin{name: "postgresql", connInfo: &plugin.ConnectionInfo{Port: 5432}}
	m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), nil)
	require.NoError(t, m.Start(ctx, "postgresql", nil))

	statuses := m.GetAllStatuses(ctx)

	require.Len(t, statuses, 1)
	assert.Equal(t, "postgresql", statuses[0].Name)
	assert.Equal(t, "running", statuses[0].Status)
}

func TestManager_GetLogs(t *testing.T) {
	ctx := context.Background()

	t.Run("in-memory uses plugin logs", func(t *testing.T) {
		p := &fakePlugin{name: "postgresql", logs: []string{"line1", "line2"}}
		m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), nil)
		require.NoError(t, m.Start(ctx, "postgresql", nil))

		logs, err := m.GetLogs(ctx, "postgresql", &plugin.LogOptions{Tail: 10})

		require.NoError(t, err)
		assert.Equal(t, []string{"line1", "line2"}, logs)
	})

	t.Run("not in memory, container exists", func(t *testing.T) {
		p := &fakePlugin{name: "postgresql", logs: []string{"line1"}}
		dock := &fakeDocker{containers: []types.Container{{State: "running"}}}
		m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), dock)

		logs, err := m.GetLogs(ctx, "postgresql", nil)

		require.NoError(t, err)
		assert.Equal(t, []string{"line1"}, logs)
	})

	t.Run("not in memory, no container", func(t *testing.T) {
		p := &fakePlugin{name: "postgresql"}
		m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), &fakeDocker{})

		_, err := m.GetLogs(ctx, "postgresql", nil)

		require.Error(t, err)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrServiceNotRunning))
	})

	t.Run("docker error is wrapped", func(t *testing.T) {
		p := &fakePlugin{name: "postgresql"}
		dock := &fakeDocker{err: errors.New("docker down")}
		m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), dock)

		_, err := m.GetLogs(ctx, "postgresql", nil)

		require.Error(t, err)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrDockerFailed))
	})
}

func TestManager_ListRunning(t *testing.T) {
	ctx := context.Background()

	t.Run("merges in-memory and docker services", func(t *testing.T) {
		p := &fakePlugin{name: "postgresql"}
		dock := &fakeDocker{containers: []types.Container{{
			State:  "running",
			Labels: map[string]string{"service": "kafka"},
		}}}
		m := NewManager(newRegistryWith(t, p), nil, fastOrchestration(false), dock)
		require.NoError(t, m.Start(ctx, "postgresql", nil))

		running := m.ListRunning()

		assert.ElementsMatch(t, []string{"postgresql", "kafka"}, running)
	})

	t.Run("ignores non-running docker containers", func(t *testing.T) {
		dock := &fakeDocker{containers: []types.Container{{
			State:  "exited",
			Labels: map[string]string{"service": "kafka"},
		}}}
		m := NewManager(plugin.NewRegistry(), nil, fastOrchestration(false), dock)

		assert.Empty(t, m.ListRunning())
	})
}

func TestManager_StartAll(t *testing.T) {
	ctx := context.Background()
	configs := map[string]map[string]interface{}{
		"postgresql": nil,
		"kafka":      nil,
	}

	t.Run("sequential", func(t *testing.T) {
		p1 := &fakePlugin{name: "postgresql"}
		p2 := &fakePlugin{name: "kafka"}
		m := NewManager(newRegistryWith(t, p1, p2), nil, fastOrchestration(false), nil)

		require.NoError(t, m.StartAll(ctx, configs))
		assert.ElementsMatch(t, []string{"postgresql", "kafka"}, m.ListRunning())
	})

	t.Run("parallel", func(t *testing.T) {
		p1 := &fakePlugin{name: "postgresql"}
		p2 := &fakePlugin{name: "kafka"}
		m := NewManager(newRegistryWith(t, p1, p2), nil, fastOrchestration(true), nil)

		require.NoError(t, m.StartAll(ctx, configs))
		assert.ElementsMatch(t, []string{"postgresql", "kafka"}, m.ListRunning())
	})

	t.Run("sequential failure triggers cleanup", func(t *testing.T) {
		p1 := &fakePlugin{name: "postgresql", launchErr: errors.New("boom")}
		m := NewManager(newRegistryWith(t, p1), nil, fastOrchestration(false),
			&fakeDocker{})

		err := m.StartAll(ctx, map[string]map[string]interface{}{"postgresql": nil})
		require.Error(t, err)
	})
}
