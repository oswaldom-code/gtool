package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	coreApp "github.com/oswaldo-montano/gtool/internal/core/app"
	"github.com/oswaldo-montano/gtool/internal/plugin"
	"github.com/oswaldo-montano/gtool/pkg/config"
)

// fakeManager is a controllable appManager for exercising the RunE bodies.
type fakeManager struct {
	startErr   error
	stopErr    error
	restartErr error
	status     *coreApp.AppStatus
	statusErr  error
	logs       []string
	logsErr    error

	startedCfg *plugin.AppConfig
	stopped    bool
}

func (f *fakeManager) Start(_ context.Context, cfg *plugin.AppConfig) error {
	f.startedCfg = cfg
	return f.startErr
}

func (f *fakeManager) Stop(_ context.Context) error {
	f.stopped = true
	return f.stopErr
}

func (f *fakeManager) Restart(_ context.Context, cfg *plugin.AppConfig) error {
	f.startedCfg = cfg
	return f.restartErr
}

func (f *fakeManager) Status(_ context.Context) (*coreApp.AppStatus, error) {
	return f.status, f.statusErr
}

func (f *fakeManager) Logs(_ context.Context, _ int) ([]string, error) {
	return f.logs, f.logsErr
}

func injectDeps(t *testing.T, mgr appManager) {
	t.Helper()
	orig := newAppDeps
	t.Cleanup(func() {
		newAppDeps = orig
		dockerImage = ""
		appPort = 0
		appEnv = nil
		cfgFile = nil
	})
	newAppDeps = func(_ *zap.Logger) (*appDeps, error) {
		return &appDeps{manager: mgr, close: func() error { return nil }}, nil
	}
}

func TestNewAppCmd(t *testing.T) {
	cmd := NewAppCmd(nil)

	assert.Equal(t, "app", cmd.Name())
	want := map[string]bool{"start": false, "stop": false, "restart": false, "status": false, "logs": false}
	for _, sub := range cmd.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		assert.True(t, found, "expected subcommand %q", name)
	}
}

func TestRunStart(t *testing.T) {
	t.Run("starts with image from flag", func(t *testing.T) {
		cfgFile = nil
		mgr := &fakeManager{}
		injectDeps(t, mgr)
		dockerImage = "myapp:latest"
		appPort = 8080

		// runStart reads package-level flag vars; pass nil so command
		// construction does not reset them to their defaults.
		err := runStart(nil, nil)

		require.NoError(t, err)
		require.NotNil(t, mgr.startedCfg)
		assert.Equal(t, "myapp:latest", mgr.startedCfg.DockerImage)
		assert.Equal(t, 8080, mgr.startedCfg.Port)
	})

	t.Run("propagates start failure", func(t *testing.T) {
		cfgFile = nil
		injectDeps(t, &fakeManager{startErr: errors.New("boom")})
		dockerImage = "myapp:latest"

		err := runStart(newStartCmd(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to start application")
	})
}

func TestRunStop(t *testing.T) {
	t.Run("stops the app", func(t *testing.T) {
		mgr := &fakeManager{}
		injectDeps(t, mgr)

		require.NoError(t, runStop(newStopCmd(), nil))
		assert.True(t, mgr.stopped)
	})

	t.Run("propagates stop failure", func(t *testing.T) {
		injectDeps(t, &fakeManager{stopErr: errors.New("boom")})

		err := runStop(newStopCmd(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to stop application")
	})
}

func TestRunRestart(t *testing.T) {
	cfgFile = nil
	mgr := &fakeManager{}
	injectDeps(t, mgr)
	dockerImage = "myapp:latest"

	require.NoError(t, runRestart(nil, nil))
	require.NotNil(t, mgr.startedCfg)
	assert.Equal(t, "myapp:latest", mgr.startedCfg.DockerImage)
}

func TestRunStatus(t *testing.T) {
	t.Run("running app", func(t *testing.T) {
		injectDeps(t, &fakeManager{status: &coreApp.AppStatus{
			Running: true, State: "running", Image: "myapp:latest", Port: 8080,
		}})
		require.NoError(t, runStatus(newStatusCmd(), nil))
	})

	t.Run("not found", func(t *testing.T) {
		injectDeps(t, &fakeManager{status: &coreApp.AppStatus{State: "not-found"}})
		require.NoError(t, runStatus(newStatusCmd(), nil))
	})

	t.Run("propagates error", func(t *testing.T) {
		injectDeps(t, &fakeManager{statusErr: errors.New("boom")})
		require.Error(t, runStatus(newStatusCmd(), nil))
	})
}

func TestRunLogs(t *testing.T) {
	t.Run("prints logs", func(t *testing.T) {
		injectDeps(t, &fakeManager{logs: []string{"line1", "line2"}})
		require.NoError(t, runLogs(newLogsCmd(), nil))
	})

	t.Run("propagates error", func(t *testing.T) {
		injectDeps(t, &fakeManager{logsErr: errors.New("boom")})
		require.Error(t, runLogs(newLogsCmd(), nil))
	})
}

func TestBuildAppConfig_FlagOverrides(t *testing.T) {
	t.Cleanup(func() { dockerImage = ""; appPort = 0; appEnv = nil })

	cfg := config.DefaultConfig()
	cfg.AppConfig.DockerImage = "base:1"
	cfg.AppConfig.Port = 1000
	cfg.AppConfig.Environment = map[string]string{"A": "1"}

	dockerImage = "override:2"
	appPort = 2000
	appEnv = map[string]string{"B": "2"}

	ac := buildAppConfig(cfg)

	assert.Equal(t, "override:2", ac.DockerImage)
	assert.Equal(t, 2000, ac.Port)
	assert.Equal(t, "1", ac.Environment["A"])
	assert.Equal(t, "2", ac.Environment["B"])
}
