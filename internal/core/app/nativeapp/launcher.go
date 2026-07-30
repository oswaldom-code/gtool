// Package nativeapp reproduces the legacy DIA component tool's golang app
// launcher (app-launchers/golang.bash, no docker image): it starts the project
// binaries built into $GOPATH/bin as detached native processes, each on an
// incrementing port with the emulator host env vars, and stops them by name.
package nativeapp

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

const (
	basePort      = 8080
	baseExtraPort = 7080

	// Emulator hosts match the legacy launcher's exported PUBSUB/STORAGE values
	// (the STABLE pubsub/gcs mocks publish on these host ports).
	pubsubEmulatorHost  = "127.0.0.1:9085"
	storageEmulatorHost = "127.0.0.1:9086"

	// readyTimeout bounds how long we wait for the primary service port to
	// accept connections before proceeding anyway (replaces a blind "sleep 5").
	readyTimeout = 30 * time.Second
	readyPoll    = 300 * time.Millisecond

	defaultBuildConfig = "build-config.yml"
)

// runner abstracts process start/stop so tests don't spawn real processes.
type runner interface {
	start(binaryPath string, args []string, env []string) error
	stopByName(name string) error
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
		run:             &osRunner{},
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
				fmt.Sprintf("binary %s not found (build it first, e.g. go build -o %s)", binPath, binPath))
		}

		// Replace any previously running instance, like the legacy "pkill" step.
		_ = l.run.stopByName(name)

		port := basePort + i
		extraPort := baseExtraPort + i
		env := append(os.Environ(),
			fmt.Sprintf("CUSTOM_SERVER_ADDRESS=0.0.0.0:%d", extraPort),
			"PUBSUB_EMULATOR_HOST="+pubsubEmulatorHost,
			"STORAGE_EMULATOR_HOST="+storageEmulatorHost,
		)

		fmt.Printf("🚀 Starting %s on port %d...\n", name, port)
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
// accepts connections, replacing the legacy blind "sleep 5". If it never comes
// up within readyTimeout it warns and proceeds (best-effort: not every app
// binds that port).
func (l *Launcher) waitForApp(ctx context.Context) error {
	if l.readyTimeout <= 0 || l.dial == nil {
		return nil
	}

	addr := fmt.Sprintf("127.0.0.1:%d", basePort)
	fmt.Printf("⏳ Waiting for app on %s...\n", addr)

	attempts := int(l.readyTimeout / readyPoll)
	if attempts < 1 {
		attempts = 1
	}
	for i := 0; i < attempts; i++ {
		if l.dial(addr) == nil {
			fmt.Printf("✅ App ready on %s\n", addr)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(readyPoll):
		}
	}

	l.logger.Warn("app did not become ready in time; continuing",
		zap.String("addr", addr), zap.Duration("timeout", l.readyTimeout))
	return nil
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
		if err := l.run.stopByName(name); err != nil {
			l.logger.Warn("failed to stop binary", zap.String("binary", name), zap.Error(err))
			continue
		}
		fmt.Printf("🛑 %s stopped\n", name)
	}
	return nil
}

// binaries returns the "<app>-<name>" binary names, where <app> is the working
// directory basename, mirroring go-tool's list-binaries.
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

// osRunner runs real OS processes.
type osRunner struct{}

func (o *osRunner) start(binaryPath string, args, env []string) error {
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = env
	// Detach into its own process group so it survives the gtool process exit,
	// matching the legacy launcher's backgrounded "&" binaries.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}

func (o *osRunner) stopByName(name string) error {
	// Mirror the legacy "pkill -f $BINARY_NAME". Exit status 1 means no process
	// matched, which is not an error for our purposes.
	err := exec.Command("pkill", "-f", name).Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil
		}
		return err
	}
	return nil
}

func goBin() (string, error) {
	if gobin := strings.TrimSpace(goEnv("GOBIN")); gobin != "" {
		return gobin, nil
	}
	gopath := strings.TrimSpace(goEnv("GOPATH"))
	if gopath == "" {
		return "", gtErrors.New(gtErrors.ErrInvalidArgument, "could not resolve GOPATH")
	}
	return filepath.Join(gopath, "bin"), nil
}

func goEnv(key string) string {
	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		return ""
	}
	return string(out)
}
