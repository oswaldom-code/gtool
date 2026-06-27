package app

import (
	"context"
	"net"
	"strconv"
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

// GenericLauncher runs an arbitrary local executable or script as the
// application under test. It implements the plugin.AppLauncher interface.
type GenericLauncher struct {
	procMgr *process.Manager
	logger  *zap.Logger

	config *plugin.AppConfig
	pid    int
}

// NewGenericLauncher creates a generic launcher backed by the given process
// manager.
func NewGenericLauncher(procMgr *process.Manager, logger *zap.Logger) *GenericLauncher {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &GenericLauncher{
		procMgr: procMgr,
		logger:  logger,
	}
}

// Technology returns the launcher identifier.
func (g *GenericLauncher) Technology() string {
	return "generic"
}

// Launch starts the application process.
func (g *GenericLauncher) Launch(ctx context.Context, config *plugin.AppConfig) error {
	command := g.resolveCommand(config)
	if command == "" {
		return gtErrors.New(gtErrors.ErrInvalidArgument,
			"generic launcher requires binary-path or binary-name")
	}

	g.logger.Info("launching application",
		zap.String("command", command),
		zap.Int("port", config.Port))

	proc, err := g.procMgr.Start(ctx, process.StartOptions{
		Command: command,
		Env:     config.Environment,
		WorkDir: config.WorkDir,
	})
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrProcessFailed, "failed to launch application")
	}

	g.config = config
	g.pid = proc.PID

	g.logger.Info("application launched", zap.Int("pid", g.pid))
	return nil
}

// IsReady reports whether the application is up. With a configured port it
// probes that the port accepts TCP connections; otherwise a running process is
// considered ready.
func (g *GenericLauncher) IsReady(_ context.Context) (bool, error) {
	if g.pid == 0 {
		return false, gtErrors.New(gtErrors.ErrProcessFailed, "application not launched")
	}

	if !g.procMgr.IsRunning(g.pid) {
		return false, nil
	}

	if g.config.Port <= 0 {
		return true, nil
	}

	addr := net.JoinHostPort("localhost", strconv.Itoa(g.config.Port))
	conn, err := net.DialTimeout("tcp", addr, probeTimeout)
	if err != nil {
		g.logger.Debug("application port not ready", zap.String("addr", addr), zap.Error(err))
		return false, nil
	}
	_ = conn.Close()
	return true, nil
}

// Stop terminates the application process gracefully.
func (g *GenericLauncher) Stop(_ context.Context) error {
	if g.pid == 0 {
		return nil
	}

	g.logger.Info("stopping application", zap.Int("pid", g.pid))
	err := g.procMgr.Stop(g.pid, stopTimeout)
	g.pid = 0
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrProcessFailed, "failed to stop application")
	}
	return nil
}

// Restart stops and relaunches the application with the original config.
func (g *GenericLauncher) Restart(ctx context.Context) error {
	if g.config == nil {
		return gtErrors.New(gtErrors.ErrProcessFailed, "application not launched")
	}

	config := g.config
	if err := g.Stop(ctx); err != nil {
		return err
	}
	return g.Launch(ctx, config)
}

// GetPID returns the PID of the running application.
func (g *GenericLauncher) GetPID() (int, error) {
	if g.pid == 0 {
		return 0, gtErrors.New(gtErrors.ErrProcessFailed, "application not running")
	}
	return g.pid, nil
}

// resolveCommand picks the executable to run: an explicit binary path takes
// precedence, otherwise the binary name is resolved via PATH.
func (g *GenericLauncher) resolveCommand(config *plugin.AppConfig) string {
	if config.BinaryPath != "" {
		return config.BinaryPath
	}
	return config.BinaryName
}
