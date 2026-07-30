package nativeapp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type startCall struct {
	path string
	args []string
	env  []string
}

type fakeRunner struct {
	started []startCall
	stopped []string
}

func (f *fakeRunner) start(path string, args, env []string) error {
	f.started = append(f.started, startCall{path: path, args: args, env: env})
	return nil
}

func (f *fakeRunner) stopByName(name string) error {
	f.stopped = append(f.stopped, name)
	return nil
}

const buildCfg = `build:
  binaries:
    - name: api
      path: cmd/notification-server/main.go
    - name: consumer
      path: cmd/notification-consumer/main.go
`

// newTestLauncher wires a launcher with a fake runner, a working dir whose
// basename is "notification" and a build-config on disk.
func newTestLauncher(t *testing.T) (*Launcher, *fakeRunner, string) {
	t.Helper()
	base := t.TempDir()
	workDir := filepath.Join(base, "notification")
	require.NoError(t, os.Mkdir(workDir, 0o755))

	cfgPath := filepath.Join(workDir, "build-config.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(buildCfg), 0o644))

	gobin := filepath.Join(base, "bin")
	require.NoError(t, os.Mkdir(gobin, 0o755))

	fr := &fakeRunner{}
	l := &Launcher{
		logger:          zap.NewNop(),
		gobin:           gobin,
		workDir:         workDir,
		buildConfigPath: cfgPath,
		run:             fr,
		readyTimeout:    0, // skip readiness polling in unit tests
	}
	return l, fr, gobin
}

func TestBinaries(t *testing.T) {
	l, _, _ := newTestLauncher(t)
	names, err := l.binaries()
	require.NoError(t, err)
	assert.Equal(t, []string{"notification-api", "notification-consumer"}, names)
}

func TestBinariesMissingConfig(t *testing.T) {
	l := &Launcher{logger: zap.NewNop(), buildConfigPath: "/nope/build-config.yml"}
	_, err := l.binaries()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStartLaunchesWithPortsAndEnv(t *testing.T) {
	l, fr, gobin := newTestLauncher(t)
	// The binaries must exist on disk for Start to launch them.
	for _, n := range []string{"notification-api", "notification-consumer"} {
		require.NoError(t, os.WriteFile(filepath.Join(gobin, n), []byte("#!/bin/sh\n"), 0o755))
	}

	require.NoError(t, l.Start(context.Background()))

	require.Len(t, fr.started, 2)
	assert.Equal(t, filepath.Join(gobin, "notification-api"), fr.started[0].path)
	assert.Equal(t, []string{"--port", "8080"}, fr.started[0].args)
	assert.Equal(t, []string{"--port", "8081"}, fr.started[1].args)
	assert.Contains(t, fr.started[0].env, "CUSTOM_SERVER_ADDRESS=0.0.0.0:7080")
	assert.Contains(t, fr.started[1].env, "CUSTOM_SERVER_ADDRESS=0.0.0.0:7081")
	assert.Contains(t, fr.started[0].env, "PUBSUB_EMULATOR_HOST=127.0.0.1:9085")
	assert.Contains(t, fr.started[0].env, "STORAGE_EMULATOR_HOST=127.0.0.1:9086")
	// Each binary is stopped before being (re)started.
	assert.Equal(t, []string{"notification-api", "notification-consumer"}, fr.stopped)
}

func TestStartFailsWhenBinaryMissing(t *testing.T) {
	l, _, _ := newTestLauncher(t)
	err := l.Start(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStopStopsAllBinaries(t *testing.T) {
	l, fr, _ := newTestLauncher(t)
	require.NoError(t, l.Stop(context.Background()))
	assert.Equal(t, []string{"notification-api", "notification-consumer"}, fr.stopped)
}

func TestWaitForAppReadyWhenPortAccepts(t *testing.T) {
	l, _, _ := newTestLauncher(t)
	l.readyTimeout = 2 * time.Second

	calls := 0
	l.dial = func(addr string) error {
		calls++
		if calls < 3 { // not ready on the first two polls
			return assert.AnError
		}
		return nil // ready on the third
	}

	require.NoError(t, l.waitForApp(context.Background()))
	assert.GreaterOrEqual(t, calls, 3)
}

func TestWaitForAppProceedsOnTimeout(t *testing.T) {
	l, _, _ := newTestLauncher(t)
	l.readyTimeout = 50 * time.Millisecond
	l.dial = func(string) error { return assert.AnError } // never ready

	// Proceeds (no error) even though the app never came up.
	require.NoError(t, l.waitForApp(context.Background()))
}
