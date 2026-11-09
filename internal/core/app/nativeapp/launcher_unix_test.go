//go:build unix

package nativeapp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"pkill", "pgrep", "bash"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
}

func cmdlineAlive(t *testing.T, pattern string) bool {
	t.Helper()
	alive, err := pgrepLike("pgrep", "-f", pattern)
	require.NoError(t, err)
	return alive
}

// waitAlive waits for bash to exec the named process before asserting on it.
func waitAlive(t *testing.T, pattern string) {
	t.Helper()
	require.Eventually(t, func() bool { return cmdlineAlive(t, pattern) }, 2*time.Second, 10*time.Millisecond,
		"expected a running process matching %s", pattern)
}

// startNamed starts "sleep <secs>" with argv[0] replaced by name (bash exec -a),
// the way a running binary shows up in the process list.
func startNamed(t *testing.T, name, secs string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("bash", "-c", "exec -a '"+name+"' sleep "+secs)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd
}

// TestOSRunnerStopsItsOwnProcessGroup runs real processes: a wrapper script
// that exec's sleep is stopped through its process group, and a bystander
// whose command line mentions the binary survives.
func TestOSRunnerStopsItsOwnProcessGroup(t *testing.T) {
	requireTools(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "orders-api")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\nexec sleep 1317\n"), 0o755))
	startNamed(t, "tail -f "+bin+".log", "1317")
	waitAlive(t, "^tail -f .*orders-api.log 1317$")

	r := &osRunner{stateDir: filepath.Join(dir, "state")}
	require.NoError(t, r.start(bin, []string{"--port", "8080"}, os.Environ()))
	data, err := os.ReadFile(r.pidFile(bin))
	require.NoError(t, err)
	pid, err := strconv.Atoi(string(data))
	require.NoError(t, err)
	waitAlive(t, "^sleep 1317$")

	require.NoError(t, r.stop(bin))

	assert.Error(t, syscall.Kill(pid, 0), "the exec'd process must be gone when stop returns")
	assert.False(t, cmdlineAlive(t, "^sleep 1317$"))
	assert.True(t, cmdlineAlive(t, "^tail -f .*orders-api.log 1317$"), "the bystander must survive")
	assert.NoError(t, r.stop(bin), "stopping again is a no-op")
	assert.NoFileExists(t, r.pidFile(bin))
}

// TestOSRunnerStopFallsBackToCommandLine covers instances started outside
// gtool (no pid file): only an exact command-line match is stopped.
func TestOSRunnerStopFallsBackToCommandLine(t *testing.T) {
	requireTools(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "orders-api")
	startNamed(t, bin+" --port 8080", "1318")
	startNamed(t, "vim "+bin, "1318")
	waitAlive(t, "^"+bin+" --port 8080 1318$")
	waitAlive(t, "^vim .*orders-api 1318$")

	r := &osRunner{stateDir: filepath.Join(dir, "state")}
	require.NoError(t, r.stop(bin))

	assert.Eventually(t, func() bool { return !cmdlineAlive(t, "^"+bin+" --port 8080 1318$") },
		2*time.Second, 20*time.Millisecond, "a process whose command line is the binary path is stopped")
	assert.True(t, cmdlineAlive(t, "^vim .*orders-api 1318$"), "an editor on the binary survives")
}
