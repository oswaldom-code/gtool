// Package process provides native OS process management: starting processes,
// capturing their output, and stopping them gracefully. It is the foundation
// for the local-execution app launchers (Phase 3).
package process

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	gtErrors "github.com/oswaldo-montano/gtool/pkg/errors"
	"go.uber.org/zap"
)

// StartOptions describes a process to launch.
type StartOptions struct {
	Command string
	Args    []string
	Env     map[string]string
	WorkDir string
}

// Process represents a managed OS process and its captured output.
type Process struct {
	PID       int
	StartTime time.Time

	cmd    *exec.Cmd
	output *lineBuffer
	doneCh chan struct{}

	mu      sync.Mutex
	done    bool
	exitErr error
}

// IsRunning reports whether the process has not yet exited.
func (p *Process) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.done
}

// ExitError returns the error from the process exit, if any. It is nil while
// the process is still running or if it exited successfully.
func (p *Process) ExitError() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exitErr
}

// Output returns the lines captured from stdout and stderr so far.
func (p *Process) Output() []string {
	return p.output.Lines()
}

func (p *Process) markDone(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		return
	}
	p.done = true
	p.exitErr = err
	close(p.doneCh)
}

// Manager starts and tracks native OS processes.
type Manager struct {
	logger *zap.Logger

	mu        sync.RWMutex
	processes map[int]*Process
}

// NewManager creates a new process Manager.
func NewManager(logger *zap.Logger) *Manager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Manager{
		logger:    logger,
		processes: make(map[int]*Process),
	}
}

// Start launches a process and begins capturing its output. The returned
// Process is tracked by the Manager and can be referenced later by its PID.
func (m *Manager) Start(ctx context.Context, opts StartOptions) (*Process, error) {
	if opts.Command == "" {
		return nil, gtErrors.New(gtErrors.ErrInvalidArgument, "command is required")
	}

	cmd := exec.CommandContext(ctx, opts.Command, opts.Args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = mergeEnv(os.Environ(), opts.Env)

	out := &lineBuffer{}
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Start(); err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrProcessFailed, "failed to start process")
	}

	p := &Process{
		PID:       cmd.Process.Pid,
		StartTime: time.Now(),
		cmd:       cmd,
		output:    out,
		doneCh:    make(chan struct{}),
	}

	m.logger.Info("process started",
		zap.String("command", opts.Command),
		zap.Int("pid", p.PID))

	// Reap the process in the background and record its exit state.
	go func() {
		err := cmd.Wait()
		p.markDone(err)
		m.logger.Info("process exited", zap.Int("pid", p.PID), zap.Error(err))
	}()

	m.mu.Lock()
	m.processes[p.PID] = p
	m.mu.Unlock()

	return p, nil
}

// Stop gracefully terminates a process: it sends SIGTERM and, if the process
// has not exited within timeout, escalates to SIGKILL.
func (m *Manager) Stop(pid int, timeout time.Duration) error {
	p := m.get(pid)
	if p == nil {
		return gtErrors.New(gtErrors.ErrProcessFailed, "process not found")
	}

	if !p.IsRunning() {
		return nil
	}

	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// Process may have just exited; fall through to wait/kill.
		m.logger.Debug("failed to send SIGTERM", zap.Int("pid", pid), zap.Error(err))
	}

	select {
	case <-p.doneCh:
		return nil
	case <-time.After(timeout):
		m.logger.Warn("process did not stop, sending SIGKILL", zap.Int("pid", pid))
		if err := p.cmd.Process.Kill(); err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrProcessFailed, "failed to kill process")
		}
		<-p.doneCh
		return nil
	}
}

// Kill terminates a process immediately with SIGKILL.
func (m *Manager) Kill(pid int) error {
	p := m.get(pid)
	if p == nil {
		return gtErrors.New(gtErrors.ErrProcessFailed, "process not found")
	}

	if !p.IsRunning() {
		return nil
	}

	if err := p.cmd.Process.Kill(); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrProcessFailed, "failed to kill process")
	}
	<-p.doneCh
	return nil
}

// IsRunning reports whether the tracked process with the given PID is running.
func (m *Manager) IsRunning(pid int) bool {
	p := m.get(pid)
	return p != nil && p.IsRunning()
}

// GetOutput returns the captured output lines for the tracked process.
func (m *Manager) GetOutput(pid int) ([]string, error) {
	p := m.get(pid)
	if p == nil {
		return nil, gtErrors.New(gtErrors.ErrProcessFailed, "process not found")
	}
	return p.Output(), nil
}

func (m *Manager) get(pid int) *Process {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.processes[pid]
}

// mergeEnv returns base with the key/value pairs from extra applied as
// KEY=VALUE entries (overrides are appended; later entries win in exec).
func mergeEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return base
	}
	merged := make([]string, 0, len(base)+len(extra))
	merged = append(merged, base...)
	for k, v := range extra {
		merged = append(merged, k+"="+v)
	}
	return merged
}

// lineBuffer is a thread-safe io.Writer that accumulates written bytes and
// exposes them split into lines.
type lineBuffer struct {
	mu      sync.Mutex
	lines   []string
	partial []byte
}

func (b *lineBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.partial = append(b.partial, p...)
	for {
		i := bytes.IndexByte(b.partial, '\n')
		if i < 0 {
			break
		}
		line := string(bytes.TrimSuffix(b.partial[:i], []byte("\r")))
		b.lines = append(b.lines, line)
		b.partial = b.partial[i+1:]
	}
	return len(p), nil
}

// Lines returns a copy of the captured lines, including any trailing
// partial line that was not newline-terminated.
func (b *lineBuffer) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]string, 0, len(b.lines)+1)
	out = append(out, b.lines...)
	if len(b.partial) > 0 {
		out = append(out, string(b.partial))
	}
	return out
}
