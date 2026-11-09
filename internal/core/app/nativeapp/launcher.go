// Package nativeapp runs the app under test as native processes: it starts the
// project binaries built into $GOPATH/bin as detached processes, each on an
// incrementing port with the emulator host env vars, and stops them again.
package nativeapp

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/oswaldom-code/gtool/internal/infra/termout"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

const (
	basePort      = 8080
	baseExtraPort = 7080

	// Emulator hosts of the fixed-contract pubsub/gcs mocks.
	pubsubEmulatorHost  = "127.0.0.1:9085"
	storageEmulatorHost = "127.0.0.1:9086"

	// readyTimeout bounds how long we wait for the primary service port to
	// accept connections before proceeding anyway (replaces a blind "sleep 5").
	readyTimeout = 30 * time.Second
	readyPoll    = 300 * time.Millisecond

	stopTimeout = 5 * time.Second
	stopPoll    = 100 * time.Millisecond

	defaultBuildConfig = "build-config.yml"
)

// runner abstracts process start/stop so tests don't spawn real processes.
type runner interface {
	start(binaryPath string, args []string, env []string) error
	stop(binaryPath string) error
}

// Launcher starts and stops the native app binaries.
type Launcher struct {
	logger          *zap.Logger
	gobin           string
	workDir         string
	buildConfigPath string
	run             runner
	readyTimeout    time.Duration
	dial            func(addr string) error
}

// New builds a Launcher resolving $GOPATH/bin and the working directory.
func New(logger *zap.Logger, buildConfigPath string) (*Launcher, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	if buildConfigPath == "" {
		buildConfigPath = defaultBuildConfig
	}
	gobin, err := goBin()
	if err != nil {
		return nil, err
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to resolve working directory")
	}
	return &Launcher{
		logger:          logger,
		gobin:           gobin,
		workDir:         wd,
		buildConfigPath: buildConfigPath,
		run:             newOSRunner(),
		readyTimeout:    readyTimeout,
		dial:            tcpDial,
	}, nil
}

// Start launches every configured binary as a detached process.
func (l *Launcher) Start(ctx context.Context) error {
	binaries, err := l.binaries()
	if err != nil {
		return err
	}

	for i, name := range binaries {
		binPath := filepath.Join(l.gobin, name)
		if _, statErr := os.Stat(binPath); statErr != nil {
			return gtErrors.New(gtErrors.ErrProcessFailed,
				fmt.Sprintf("binary %s not found: build the build-config binaries into %s first (e.g. go build -o %s ./<cmd package>)",
					binPath, l.gobin, binPath))
		}

		if err := l.run.stop(binPath); err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrProcessFailed,
				fmt.Sprintf("failed to stop the previous %s", name))
		}

		port := basePort + i
		extraPort := baseExtraPort + i
		env := append(os.Environ(),
			fmt.Sprintf("CUSTOM_SERVER_ADDRESS=0.0.0.0:%d", extraPort),
			"PUBSUB_EMULATOR_HOST="+pubsubEmulatorHost,
			"STORAGE_EMULATOR_HOST="+storageEmulatorHost,
		)

		termout.Printf("🚀 Starting %s on port %d...\n", name, port)
		if err := l.run.start(binPath, []string{"--port", fmt.Sprintf("%d", port)}, env); err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrProcessFailed,
				fmt.Sprintf("failed to start %s", name))
		}
		l.logger.Info("started native app binary",
			zap.String("binary", name), zap.Int("port", port), zap.Int("extra-port", extraPort))
	}

	return l.waitForApp(ctx)
}

// waitForApp polls the primary service port (the first binary's, 8080) until it
// accepts connections. If it never comes
// up within readyTimeout it warns and proceeds (best-effort: not every app
// binds that port).
func (l *Launcher) waitForApp(ctx context.Context) error {
	if l.readyTimeout <= 0 || l.dial == nil {
		return nil
	}

	addr := fmt.Sprintf("127.0.0.1:%d", basePort)
	termout.Printf("⏳ Waiting for app on %s...\n", addr)

	attempts := int(l.readyTimeout / readyPoll)
	if attempts < 1 {
		attempts = 1
	}
	for i := 0; i < attempts; i++ {
		if l.dial(addr) == nil {
			termout.Printf("✅ App ready on %s\n", addr)
			return nil
		}
		select {
		case <-ctx.Done():
			return gtErrors.Wrap(ctx.Err(), gtErrors.ErrServiceNotReady, "cancelled while waiting for the app")
		case <-time.After(readyPoll):
		}
	}

	return gtErrors.New(gtErrors.ErrServiceNotReady,
		fmt.Sprintf("app did not accept connections on %s within %s", addr, l.readyTimeout))
}

func tcpDial(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return err
	}
	return conn.Close()
}

// Stop terminates every configured binary by name.
func (l *Launcher) Stop(ctx context.Context) error {
	binaries, err := l.binaries()
	if err != nil {
		return err
	}
	for _, name := range binaries {
		if err := l.run.stop(filepath.Join(l.gobin, name)); err != nil {
			l.logger.Warn("failed to stop binary", zap.String("binary", name), zap.Error(err))
			continue
		}
		termout.Printf("🛑 %s stopped\n", name)
	}
	return nil
}

// binaries returns the "<app>-<name>" binary names, where <app> is the working
// directory basename.
func (l *Launcher) binaries() ([]string, error) {
	data, err := os.ReadFile(l.buildConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, gtErrors.New(gtErrors.ErrConfigNotFound,
				fmt.Sprintf("%s not found (required to list app binaries)", l.buildConfigPath))
		}
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to read build config")
	}

	var cfg struct {
		Build struct {
			Binaries []struct {
				Name string `yaml:"name"`
			} `yaml:"binaries"`
		} `yaml:"build"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to parse build config")
	}
	if len(cfg.Build.Binaries) == 0 {
		return nil, gtErrors.New(gtErrors.ErrConfigInvalid, "no binaries defined in build config")
	}

	appName := filepath.Base(l.workDir)
	names := make([]string, 0, len(cfg.Build.Binaries))
	for _, b := range cfg.Build.Binaries {
		if b.Name == "" {
			continue
		}
		names = append(names, appName+"-"+b.Name)
	}
	return names, nil
}

// osRunner runs real OS processes. Each started binary leads its own process
// group and its PID is recorded, so stop can terminate the whole group even
// when the binary is a wrapper script that exec'd something else.
type osRunner struct {
	stateDir string
}

func newOSRunner() *osRunner {
	return &osRunner{stateDir: filepath.Join(os.TempDir(), "gtool-native")}
}

func (o *osRunner) pidFile(binaryPath string) string {
	sum := sha1.Sum([]byte(binaryPath))
	return filepath.Join(o.stateDir, hex.EncodeToString(sum[:8])+".pid")
}

func (o *osRunner) start(binaryPath string, args, env []string) error {
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = env
	cmd.SysProcAttr = detachAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()

	if err := os.MkdirAll(o.stateDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(o.pidFile(binaryPath), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644)
}

// commandPattern matches processes whose command line starts with exactly
// binaryPath, so editors, "tail -f <name>.log" or "go build -o <path>" are
// never hit.
func commandPattern(binaryPath string) string {
	return "^" + regexp.QuoteMeta(binaryPath) + "( |$)"
}

// pgrepLike runs pkill/pgrep; exit status 1 means no process matched.
func pgrepLike(name string, args ...string) (bool, error) {
	err := exec.Command(name, args...).Run()
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

// stop terminates the binary: by process group when this runner started it
// (pid file), otherwise by exact command line, as a fallback for instances
// started outside gtool. It waits for the exit and escalates to SIGKILL, so a
// restart never races the old process for its port.
func (o *osRunner) stop(binaryPath string) error {
	pidFile := o.pidFile(binaryPath)
	if data, err := os.ReadFile(pidFile); err == nil {
		defer func() { _ = os.Remove(pidFile) }()
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			return stopGroup(pid)
		}
	}
	return o.stopByPattern(commandPattern(binaryPath))
}

func (o *osRunner) stopByPattern(pattern string) error {
	found, err := pgrepLike("pkill", "-TERM", "-f", pattern)
	if err != nil || !found {
		return err
	}
	for deadline := time.Now().Add(stopTimeout); time.Now().Before(deadline); time.Sleep(stopPoll) {
		alive, err := pgrepLike("pgrep", "-f", pattern)
		if err != nil {
			return err
		}
		if !alive {
			return nil
		}
	}
	_, err = pgrepLike("pkill", "-KILL", "-f", pattern)
	return err
}

func goBin() (string, error) {
	if gobin := strings.TrimSpace(goEnv("GOBIN")); gobin != "" {
		return gobin, nil
	}
	gopath := strings.TrimSpace(goEnv("GOPATH"))
	if gopath == "" {
		return "", gtErrors.New(gtErrors.ErrInvalidArgument,
			"could not resolve GOPATH: is the go command installed and on PATH?")
	}
	return filepath.Join(filepath.SplitList(gopath)[0], "bin"), nil
}

func goEnv(key string) string {
	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		return ""
	}
	return string(out)
}
