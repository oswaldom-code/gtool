package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/oswaldo-montano/gtool/internal/core/mock"
	"github.com/oswaldo-montano/gtool/internal/plugin"
	"github.com/oswaldo-montano/gtool/pkg/config"
)

// fakeManager is a controllable serviceManager for exercising the RunE bodies
// without a real Docker client.
type fakeManager struct {
	startErr   error
	stopErr    error
	running    []string
	statuses   []*mock.ServiceStatus
	logs       []string
	logsErr    error
	startedSvc []string
	stoppedSvc []string
}

func (f *fakeManager) Start(_ context.Context, name string, _ map[string]interface{}) error {
	f.startedSvc = append(f.startedSvc, name)
	return f.startErr
}

func (f *fakeManager) Stop(_ context.Context, name string) error {
	f.stoppedSvc = append(f.stoppedSvc, name)
	return f.stopErr
}

func (f *fakeManager) ListRunning() []string { return f.running }

func (f *fakeManager) GetAllStatuses(_ context.Context) []*mock.ServiceStatus { return f.statuses }

func (f *fakeManager) GetLogs(_ context.Context, _ string, _ *plugin.LogOptions) ([]string, error) {
	return f.logs, f.logsErr
}

// injectDeps replaces the package factory with one returning the given manager
// and restores the original after the test.
func injectDeps(t *testing.T, mgr serviceManager, pingErr error) {
	t.Helper()
	orig := newServiceDeps
	t.Cleanup(func() { newServiceDeps = orig })
	newServiceDeps = func(_ *config.Config, _ *zap.Logger) (*serviceDeps, error) {
		return &serviceDeps{
			manager: mgr,
			ping:    func(context.Context) error { return pingErr },
			close:   func() error { return nil },
		}, nil
	}
}

func TestNewServicesCmd(t *testing.T) {
	cmd := NewServicesCmd(nil)

	assert.Equal(t, "services", cmd.Name())
	assert.Contains(t, cmd.Aliases, "s")

	want := map[string]bool{"up": false, "down": false, "status": false, "logs": false}
	for _, sub := range cmd.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		assert.True(t, found, "expected subcommand %q to be registered", name)
	}
}

func TestNewServicesCmd_StoresConfigFile(t *testing.T) {
	file := "my-config.yml"
	_ = NewServicesCmd(&file)
	assert.Equal(t, "my-config.yml", configFilePath())
}

func TestServicesLogsCmd_Flags(t *testing.T) {
	cmd := newServicesLogsCmd()

	require.NotNil(t, cmd.Flags().Lookup("follow"))
	require.NotNil(t, cmd.Flags().Lookup("all"))
	require.NotNil(t, cmd.Flags().Lookup("tail"))
}

func TestRunServicesLogs_RequiresServiceOrAll(t *testing.T) {
	allLogs = false
	err := runServicesLogs(newServicesLogsCmd(), nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "specify a service")
}

func TestRunServicesUp(t *testing.T) {
	t.Run("starts requested services", func(t *testing.T) {
		cfgFile = nil
		mgr := &fakeManager{}
		injectDeps(t, mgr, nil)

		err := runServicesUp(newServicesUpCmd(), []string{"postgresql", "kafka"})

		require.NoError(t, err)
		assert.Equal(t, []string{"postgresql", "kafka"}, mgr.startedSvc)
	})

	t.Run("fails when daemon is unavailable", func(t *testing.T) {
		cfgFile = nil
		injectDeps(t, &fakeManager{}, errors.New("no daemon"))

		err := runServicesUp(newServicesUpCmd(), []string{"postgresql"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "Docker daemon not available")
	})

	t.Run("errors when no services configured", func(t *testing.T) {
		cfgFile = nil
		injectDeps(t, &fakeManager{}, nil)

		// Default config has no mocks, so an argless up has nothing to start.
		err := runServicesUp(newServicesUpCmd(), nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "no services")
	})

	t.Run("propagates start failure", func(t *testing.T) {
		cfgFile = nil
		injectDeps(t, &fakeManager{startErr: errors.New("boom")}, nil)

		err := runServicesUp(newServicesUpCmd(), []string{"postgresql"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to start postgresql")
	})
}

func TestRunServicesDown(t *testing.T) {
	t.Run("stops explicit services", func(t *testing.T) {
		cfgFile = nil
		mgr := &fakeManager{}
		injectDeps(t, mgr, nil)

		err := runServicesDown(newServicesDownCmd(), []string{"postgresql"})

		require.NoError(t, err)
		assert.Equal(t, []string{"postgresql"}, mgr.stoppedSvc)
	})

	t.Run("stops running services when none specified", func(t *testing.T) {
		cfgFile = nil
		mgr := &fakeManager{running: []string{"kafka"}}
		injectDeps(t, mgr, nil)

		err := runServicesDown(newServicesDownCmd(), nil)

		require.NoError(t, err)
		assert.Equal(t, []string{"kafka"}, mgr.stoppedSvc)
	})

	t.Run("no-op when nothing is running", func(t *testing.T) {
		cfgFile = nil
		mgr := &fakeManager{running: nil}
		injectDeps(t, mgr, nil)

		err := runServicesDown(newServicesDownCmd(), nil)

		require.NoError(t, err)
		assert.Empty(t, mgr.stoppedSvc)
	})

	t.Run("continues past a stop failure", func(t *testing.T) {
		cfgFile = nil
		mgr := &fakeManager{stopErr: errors.New("boom")}
		injectDeps(t, mgr, nil)

		err := runServicesDown(newServicesDownCmd(), []string{"postgresql", "kafka"})

		require.NoError(t, err)
		assert.Equal(t, []string{"postgresql", "kafka"}, mgr.stoppedSvc)
	})
}

func TestRunServicesStatus(t *testing.T) {
	t.Run("renders statuses", func(t *testing.T) {
		cfgFile = nil
		mgr := &fakeManager{statuses: []*mock.ServiceStatus{
			{Name: "postgresql", Status: "running", Port: 5432, Uptime: time.Minute},
			{Name: "kafka", Status: "stopped"},
			{Name: "gcs", Status: "error"},
		}}
		injectDeps(t, mgr, nil)

		err := runServicesStatus(newServicesStatusCmd(), nil)
		require.NoError(t, err)
	})

	t.Run("handles no services", func(t *testing.T) {
		cfgFile = nil
		injectDeps(t, &fakeManager{}, nil)

		err := runServicesStatus(newServicesStatusCmd(), nil)
		require.NoError(t, err)
	})
}

func TestRunServicesLogs(t *testing.T) {
	t.Run("prints logs for a service", func(t *testing.T) {
		cfgFile = nil
		allLogs = false
		mgr := &fakeManager{logs: []string{"line1", "line2"}}
		injectDeps(t, mgr, nil)

		err := runServicesLogs(newServicesLogsCmd(), []string{"postgresql"})
		require.NoError(t, err)
	})

	t.Run("with --all uses running services", func(t *testing.T) {
		cfgFile = nil
		mgr := &fakeManager{running: []string{"postgresql"}, logs: []string{"line1"}}
		injectDeps(t, mgr, nil)

		// newServicesLogsCmd resets allLogs to its flag default, so set it after.
		cmd := newServicesLogsCmd()
		allLogs = true
		t.Cleanup(func() { allLogs = false })

		err := runServicesLogs(cmd, nil)
		require.NoError(t, err)
	})

	t.Run("errors when --all but nothing running", func(t *testing.T) {
		cfgFile = nil
		injectDeps(t, &fakeManager{running: nil}, nil)

		cmd := newServicesLogsCmd()
		allLogs = true
		t.Cleanup(func() { allLogs = false })

		err := runServicesLogs(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no running services")
	})

	t.Run("continues past a logs failure", func(t *testing.T) {
		cfgFile = nil
		allLogs = false
		mgr := &fakeManager{logsErr: errors.New("boom")}
		injectDeps(t, mgr, nil)

		err := runServicesLogs(newServicesLogsCmd(), []string{"postgresql"})
		require.NoError(t, err)
	})
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"seconds", 45 * time.Second, "45s"},
		{"minutes", 5 * time.Minute, "5m"},
		{"hours", 3 * time.Hour, "3h"},
		{"days", 48 * time.Hour, "2d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatDuration(tt.in))
		})
	}
}

func TestLoadConfigOrDefault(t *testing.T) {
	t.Run("loads explicit config file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yml")
		content := []byte("version: v1\n" +
			"app-technology: golang\n" +
			"app-config:\n" +
			"  binary-name: test-app\n" +
			"  port: 8080\n" +
			"test-launcher: test-launcher-back\n")
		require.NoError(t, os.WriteFile(path, content, 0o600))

		cfg, err := loadConfigOrDefault(path)

		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Equal(t, "golang", cfg.AppTechnology)
	})

	t.Run("returns error for invalid explicit path", func(t *testing.T) {
		_, err := loadConfigOrDefault(filepath.Join(t.TempDir(), "does-not-exist.yml"))
		require.Error(t, err)
	})

	t.Run("falls back to defaults when no file given", func(t *testing.T) {
		cfg, err := loadConfigOrDefault("")

		require.NoError(t, err)
		require.NotNil(t, cfg)
	})
}
