package app

import (
	"context"
	"time"

	"github.com/oswaldo-montano/gtool/internal/infra/process"
	"github.com/oswaldo-montano/gtool/internal/plugin"
	gtErrors "github.com/oswaldo-montano/gtool/pkg/errors"
	"go.uber.org/zap"
)

const (
	stopTimeout  = 10 * time.Second
	probeTimeout = 2 * time.Second
)

// readinessProbe reports whether a service listening on the given port is
// considered ready. Implementations differ per technology (TCP vs HTTP).
type readinessProbe func(port int) bool

// processLauncher is the shared implementation for launchers that run the
// application as a local process. Concrete launchers embed it and supply a
// technology name and a readiness probe.
type processLauncher struct {
	technology string
	procMgr    *process.Manager
	logger     *zap.Logger
	probe      readinessProbe

	config *plugin.AppConfig
	pid    int
}

func newProcessLauncher(technology string, procMgr *process.Manager, logger *zap.Logger, probe readinessProbe) *processLauncher {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &processLauncher{
		technology: technology,
		procMgr:    procMgr,
		logger:     logger,
		probe:      probe,
	}
}

// Technology returns the launcher identifier.
func (l *processLauncher) Technology() string {
	return l.technology
}

// Launch starts the application process.
func (l *processLauncher) Launch(ctx context.Context, config *plugin.AppConfig) error {
	command := resolveCommand(config)
	if command == "" {
		return gtErrors.New(gtErrors.ErrInvalidArgument,
			"launcher requires binary-path or binary-name")
	}

	l.logger.Info("launching application",
		zap.String("technology", l.technology),
		zap.String("command", command),
		zap.Int("port", config.Port))

	proc, err := l.procMgr.Start(ctx, process.StartOptions{
		Command: command,
		Env:     config.Environment,
		WorkDir: config.WorkDir,
	})
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrProcessFailed, "failed to launch application")
	}

	l.config = config
	l.pid = proc.PID

	l.logger.Info("application launched", zap.Int("pid", l.pid))
	return nil
}

// IsReady reports whether the application is up. With a configured port it runs
// the technology-specific probe; otherwise a running process is considered ready.
func (l *processLauncher) IsReady(_ context.Context) (bool, error) {
	if l.pid == 0 {
		return false, gtErrors.New(gtErrors.ErrProcessFailed, "application not launched")
	}

	if !l.procMgr.IsRunning(l.pid) {
		return false, nil
	}

	if l.config.Port <= 0 {
		return true, nil
	}

	return l.probe(l.config.Port), nil
}

// Stop terminates the application process gracefully.
func (l *processLauncher) Stop(_ context.Context) error {
	if l.pid == 0 {
		return nil
	}

	l.logger.Info("stopping application", zap.Int("pid", l.pid))
	err := l.procMgr.Stop(l.pid, stopTimeout)
	l.pid = 0
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrProcessFailed, "failed to stop application")
	}
	return nil
}

// Restart stops and relaunches the application with the original config.
func (l *processLauncher) Restart(ctx context.Context) error {
	if l.config == nil {
		return gtErrors.New(gtErrors.ErrProcessFailed, "application not launched")
	}

	config := l.config
	if err := l.Stop(ctx); err != nil {
		return err
	}
	return l.Launch(ctx, config)
}

// GetPID returns the PID of the running application.
func (l *processLauncher) GetPID() (int, error) {
	if l.pid == 0 {
		return 0, gtErrors.New(gtErrors.ErrProcessFailed, "application not running")
	}
	return l.pid, nil
}

// resolveCommand picks the executable to run: an explicit binary path takes
// precedence, otherwise the binary name is resolved via PATH.
func resolveCommand(config *plugin.AppConfig) string {
	if config.BinaryPath != "" {
		return config.BinaryPath
	}
	return config.BinaryName
}
