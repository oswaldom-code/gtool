package process

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

func waitNotRunning(t *testing.T, m *Manager, pid int) {
	t.Helper()
	require.Eventually(t, func() bool { return !m.IsRunning(pid) }, 2*time.Second, 5*time.Millisecond)
}

func TestManager_Start_CapturesStdoutAndStderr(t *testing.T) {
	m := NewManager(nil)

	p, err := m.Start(context.Background(), StartOptions{
		Command: "sh",
		Args:    []string{"-c", "echo out; echo err >&2"},
	})
	require.NoError(t, err)
	assert.Greater(t, p.PID, 0)

	waitNotRunning(t, m, p.PID)

	out, err := m.GetOutput(p.PID)
	require.NoError(t, err)
	assert.Contains(t, out, "out")
	assert.Contains(t, out, "err")
}

func TestManager_Start_EmptyCommand(t *testing.T) {
	m := NewManager(nil)

	_, err := m.Start(context.Background(), StartOptions{})

	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrInvalidArgument))
}

func TestManager_Start_InvalidCommand(t *testing.T) {
	m := NewManager(nil)

	_, err := m.Start(context.Background(), StartOptions{Command: "this-binary-does-not-exist-gtool"})

	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrProcessFailed))
}

func TestManager_Env(t *testing.T) {
	m := NewManager(nil)

	p, err := m.Start(context.Background(), StartOptions{
		Command: "sh",
		Args:    []string{"-c", "echo $GTOOL_TEST_VAR"},
		Env:     map[string]string{"GTOOL_TEST_VAR": "hello-env"},
	})
	require.NoError(t, err)
	waitNotRunning(t, m, p.PID)

	out, err := m.GetOutput(p.PID)
	require.NoError(t, err)
	assert.Contains(t, out, "hello-env")
}

func TestManager_WorkDir(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(nil)

	p, err := m.Start(context.Background(), StartOptions{
		Command: "pwd",
		WorkDir: dir,
	})
	require.NoError(t, err)
	waitNotRunning(t, m, p.PID)

	out, err := m.GetOutput(p.PID)
	require.NoError(t, err)
	require.NotEmpty(t, out)
	// macOS/Linux may resolve symlinks; assert the basename is present.
	assert.Contains(t, out[0], filepathBase(dir))
}

func TestManager_IsRunning(t *testing.T) {
	m := NewManager(nil)

	p, err := m.Start(context.Background(), StartOptions{
		Command: "sleep",
		Args:    []string{"5"},
	})
	require.NoError(t, err)

	assert.True(t, m.IsRunning(p.PID))

	require.NoError(t, m.Kill(p.PID))
	assert.False(t, m.IsRunning(p.PID))
}

func TestManager_Stop_Graceful(t *testing.T) {
	m := NewManager(nil)

	p, err := m.Start(context.Background(), StartOptions{
		Command: "sleep",
		Args:    []string{"30"},
	})
	require.NoError(t, err)
	require.True(t, m.IsRunning(p.PID))

	start := time.Now()
	require.NoError(t, m.Stop(p.PID, 2*time.Second))
	assert.False(t, m.IsRunning(p.PID))
	assert.Less(t, time.Since(start), 2*time.Second, "SIGTERM should stop sleep promptly")
}

func TestManager_Stop_EscalatesToKill(t *testing.T) {
	m := NewManager(nil)

	// Trap SIGTERM so only SIGKILL can stop it; Stop must escalate after timeout.
	p, err := m.Start(context.Background(), StartOptions{
		Command: "sh",
		Args:    []string{"-c", "trap '' TERM; sleep 30"},
	})
	require.NoError(t, err)
	require.True(t, m.IsRunning(p.PID))

	require.NoError(t, m.Stop(p.PID, 200*time.Millisecond))
	assert.False(t, m.IsRunning(p.PID))
}

func TestManager_Stop_AlreadyExited(t *testing.T) {
	m := NewManager(nil)

	p, err := m.Start(context.Background(), StartOptions{Command: "true"})
	require.NoError(t, err)
	waitNotRunning(t, m, p.PID)

	// Stopping an already-exited process is a no-op.
	require.NoError(t, m.Stop(p.PID, time.Second))
}

func TestManager_ExitError(t *testing.T) {
	m := NewManager(nil)

	p, err := m.Start(context.Background(), StartOptions{Command: "false"})
	require.NoError(t, err)
	waitNotRunning(t, m, p.PID)

	assert.Error(t, p.ExitError(), "non-zero exit should be recorded")
}

func TestManager_UnknownPID(t *testing.T) {
	m := NewManager(nil)

	assert.False(t, m.IsRunning(99999999))

	_, err := m.GetOutput(99999999)
	require.Error(t, err)

	require.Error(t, m.Stop(99999999, time.Second))
	require.Error(t, m.Kill(99999999))
}

func TestLineBuffer_PartialLine(t *testing.T) {
	b := &lineBuffer{}

	_, _ = b.Write([]byte("complete\npar"))
	lines := b.Lines()

	require.Len(t, lines, 2)
	assert.Equal(t, "complete", lines[0])
	assert.Equal(t, "par", lines[1], "trailing partial line should be included")
}

func TestLineBuffer_ConcurrentWrites(t *testing.T) {
	b := &lineBuffer{}
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = b.Write([]byte("line\n"))
		}()
	}
	wg.Wait()

	assert.Len(t, b.Lines(), 50)
}

// filepathBase returns the last path element, avoiding an extra import in the
// test for a single use.
func filepathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}
