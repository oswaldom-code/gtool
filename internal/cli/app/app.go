package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/oswaldom-code/gtool/internal/cli/cliconfig"
	coreApp "github.com/oswaldom-code/gtool/internal/core/app"
	"github.com/oswaldom-code/gtool/internal/core/app/nativeapp"
	"github.com/oswaldom-code/gtool/internal/core/ports"
	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/internal/infra/termout"
	"github.com/oswaldom-code/gtool/internal/plugin"
	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
	"github.com/oswaldom-code/gtool/pkg/logger"
)

var (
	cfgFile         *string
	dockerImage     string
	appPort         int
	appEnv          map[string]string
	logsTail        int
	nativeMode      bool
	buildConfigFile string
	allRuns         bool
)

// configFilePath returns the current --config value, dereferenced at run time
// because the persistent flag is parsed after the command is constructed.
func configFilePath() string {
	if cfgFile == nil {
		return ""
	}
	return *cfgFile
}

// appManager is the subset of *coreApp.DockerManager used by the commands, so
// tests can inject a fake.
type appManager interface {
	Start(ctx context.Context, cfg *plugin.AppConfig) error
	Stop(ctx context.Context) error
	Restart(ctx context.Context, cfg *plugin.AppConfig) error
	Status(ctx context.Context) (*coreApp.AppStatus, error)
	Logs(ctx context.Context, tail int) ([]string, error)
}

// appDeps bundles the runtime dependencies a command needs.
type appDeps struct {
	manager appManager
	close   func() error
}

// depsFactory builds the dependencies; a package variable so tests can replace
// it with a Docker-free implementation.
type depsFactory func(log *zap.Logger) (*appDeps, error)

var newAppDeps depsFactory = defaultAppDeps

func defaultAppDeps(log *zap.Logger) (*appDeps, error) {
	dockerClient, err := docker.NewClient(log)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrDockerConnection, "failed to create Docker client")
	}
	dockerClient.SetRunID(docker.ResolveRunID(false))
	dockerClient.SetAllRuns(allRuns)
	return &appDeps{
		manager: coreApp.NewDockerManager(dockerClient, log),
		close:   dockerClient.Close,
	}, nil
}

// NewAppCmd builds the `app` command tree.
func NewAppCmd(configFile *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app",
		Short: "Manage the application under test",
		Long: `Start, stop and inspect the application under test as a Docker container.

The application is run as a labelled container so separate invocations can
find it again (gtool app status / stop / logs).

Examples:
  gtool app start --docker-image myapp:latest --port 8080
  gtool app status
  gtool app logs --tail 50
  gtool app stop`,
	}

	cfgFile = configFile

	cmd.AddCommand(newStartCmd(), newStopCmd(), newRestartCmd(), newStatusCmd(), newLogsCmd())
	cmd.PersistentFlags().BoolVar(&allRuns, "all-runs", false,
		"without GTOOL_RUN_ID, also act on containers of other gtool runs (e.g. concurrent gtool test pipelines)")
	return cmd
}

func newStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the application",
		RunE:  runStart,
	}
	cmd.Flags().StringVar(&dockerImage, "docker-image", "", "Docker image to run (overrides config)")
	cmd.Flags().IntVar(&appPort, "port", 0, "application port (overrides config)")
	cmd.Flags().StringToStringVar(&appEnv, "env", nil, "environment variables (KEY=VALUE)")
	cmd.Flags().BoolVar(&nativeMode, "native", false, "launch the build-config binaries as native processes")
	cmd.Flags().StringVar(&buildConfigFile, "build-config", "build-config.yml", "path to build-config.yml (with --native)")
	return cmd
}

func newStopCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "stop", Short: "Stop the application", RunE: runStop}
	cmd.Flags().BoolVar(&nativeMode, "native", false, "stop the native build-config binaries")
	cmd.Flags().StringVar(&buildConfigFile, "build-config", "build-config.yml", "path to build-config.yml (with --native)")
	return cmd
}

func newRestartCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "restart", Short: "Restart the application", RunE: runRestart}
	cmd.Flags().BoolVar(&nativeMode, "native", false, "restart the native build-config binaries")
	cmd.Flags().StringVar(&buildConfigFile, "build-config", "build-config.yml", "path to build-config.yml (with --native)")
	cmd.Flags().StringVar(&dockerImage, "docker-image", "", "Docker image to run (overrides config)")
	cmd.Flags().IntVar(&appPort, "port", 0, "application port (overrides config)")
	cmd.Flags().StringToStringVar(&appEnv, "env", nil, "environment variables (KEY=VALUE)")
	return cmd
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Aliases: []string{"ps"}, Short: "Show application status", RunE: runStatus}
}

func newLogsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "logs", Short: "View application logs", RunE: runLogs}
	cmd.Flags().IntVar(&logsTail, "tail", 100, "number of lines to show from the end of the logs")
	return cmd
}

// applyFlagOverrides applies --docker-image, --port and --env to cfg. It runs
// before ports.Resolve so placeholders see the overridden values.
func applyFlagOverrides(cfg *config.Config) {
	if dockerImage != "" {
		cfg.AppConfig.DockerImage = dockerImage
	}
	if appPort > 0 {
		cfg.AppConfig.Port = appPort
	}
	if len(appEnv) > 0 {
		if cfg.AppConfig.Environment == nil {
			cfg.AppConfig.Environment = make(map[string]string, len(appEnv))
		}
		for k, v := range appEnv {
			cfg.AppConfig.Environment[k] = v
		}
	}
}

// buildAppConfig maps the resolved configuration to a plugin.AppConfig.
func buildAppConfig(cfg *config.Config) *plugin.AppConfig {
	return &plugin.AppConfig{
		BinaryName:  cfg.AppConfig.BinaryName,
		BinaryPath:  cfg.AppConfig.BinaryPath,
		DockerImage: cfg.AppConfig.DockerImage,
		Port:        cfg.AppConfig.Port,
		HostPort:    ports.AppHostPort(cfg),
		Environment: cfg.AppConfig.Environment,
	}
}

func runStart(_ *cobra.Command, _ []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	if nativeMode {
		launcher, err := nativeapp.New(log.Logger, buildConfigFile)
		if err != nil {
			return err
		}
		if err := launcher.Start(ctx); err != nil {
			return err
		}
		termout.Printf("✅ Native app started. Use 'gtool app stop --native' to stop it.\n")
		return nil
	}

	cfg, err := cliconfig.Load(configFilePath(), cliconfig.Start, os.Stderr)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to load configuration")
	}
	applyFlagOverrides(cfg)
	if _, err := ports.Resolve(cfg, ports.NoAuto); err != nil {
		return err
	}

	deps, err := newAppDeps(log.Logger)
	if err != nil {
		return err
	}
	defer func() { _ = deps.close() }()

	appConfig := buildAppConfig(cfg)
	termout.Printf("🚀 Starting application (%s)...\n", appConfig.DockerImage)
	if err := deps.manager.Start(ctx, appConfig); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to start application")
	}

	termout.Printf("✅ Application started\n")
	fmt.Printf("Use 'gtool app status' to check status\n")
	return nil
}

func runStop(_ *cobra.Command, _ []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	if nativeMode {
		launcher, err := nativeapp.New(log.Logger, buildConfigFile)
		if err != nil {
			return err
		}
		return launcher.Stop(ctx)
	}

	deps, err := newAppDeps(log.Logger)
	if err != nil {
		return err
	}
	defer func() { _ = deps.close() }()

	termout.Println("🛑 Stopping application...")
	if err := deps.manager.Stop(ctx); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to stop application")
	}

	termout.Println("✅ Application stopped")
	return nil
}

func runRestart(_ *cobra.Command, _ []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	if nativeMode {
		launcher, err := nativeapp.New(log.Logger, buildConfigFile)
		if err != nil {
			return err
		}
		if err := launcher.Stop(ctx); err != nil {
			return err
		}
		return launcher.Start(ctx)
	}

	cfg, err := cliconfig.Load(configFilePath(), cliconfig.Start, os.Stderr)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to load configuration")
	}
	applyFlagOverrides(cfg)
	if _, err := ports.Resolve(cfg, ports.NoAuto); err != nil {
		return err
	}

	deps, err := newAppDeps(log.Logger)
	if err != nil {
		return err
	}
	defer func() { _ = deps.close() }()

	termout.Println("🔄 Restarting application...")
	if err := deps.manager.Restart(ctx, buildAppConfig(cfg)); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to restart application")
	}

	termout.Println("✅ Application restarted")
	return nil
}

func runStatus(_ *cobra.Command, _ []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := zap.NewNop()

	deps, err := newAppDeps(log)
	if err != nil {
		return err
	}
	defer func() { _ = deps.close() }()

	status, err := deps.manager.Status(ctx)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to get application status")
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "STATUS\tIMAGE\tPORT")
	_, _ = fmt.Fprintln(w, "------\t-----\t----")

	state := status.State
	if status.Running {
		state = termout.Text("running ✓")
	}
	image := status.Image
	if image == "" {
		image = "-"
	}
	port := "-"
	if status.Port > 0 {
		port = fmt.Sprintf("%d", status.Port)
	}
	_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", state, image, port)
	return w.Flush()
}

func runLogs(_ *cobra.Command, _ []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := zap.NewNop()

	deps, err := newAppDeps(log)
	if err != nil {
		return err
	}
	defer func() { _ = deps.close() }()

	logs, err := deps.manager.Logs(ctx, logsTail)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to get application logs")
	}

	for _, line := range logs {
		fmt.Println(line)
	}
	return nil
}
