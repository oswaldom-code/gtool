package app

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	coreApp "github.com/oswaldom-code/gtool/internal/core/app"
	"github.com/oswaldom-code/gtool/internal/core/app/nativeapp"
	coreConfig "github.com/oswaldom-code/gtool/internal/core/config"
	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/internal/plugin"
	"github.com/oswaldom-code/gtool/pkg/config"
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
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
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
	cmd.Flags().BoolVar(&nativeMode, "native", false, "launch the build-config binaries as native processes (like legacy 'component r')")
	cmd.Flags().StringVar(&buildConfigFile, "build-config", "build-config.yml", "path to build-config.yml (with --native)")
	return cmd
}

func newStopCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "stop", Short: "Stop the application", RunE: runStop}
	cmd.Flags().BoolVar(&nativeMode, "native", false, "stop the native build-config binaries (like legacy 'component p')")
	cmd.Flags().StringVar(&buildConfigFile, "build-config", "build-config.yml", "path to build-config.yml (with --native)")
	return cmd
}

func newRestartCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "restart", Short: "Restart the application", RunE: runRestart}
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

func runStart(_ *cobra.Command, _ []string) error {
	ctx := context.Background()
	log := logger.Default()
	defer log.Sync()

	if nativeMode {
		launcher, err := nativeapp.New(log.Logger, buildConfigFile)
		if err != nil {
			return err
		}
		if err := launcher.Start(ctx); err != nil {
			return err
		}
		fmt.Printf("✅ Native app started. Use 'gtool app stop --native' to stop it.\n")
		return nil
	}

	cfg, err := loadConfigOrDefault(configFilePath())
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	deps, err := newAppDeps(log.Logger)
	if err != nil {
		return err
	}
	defer deps.close()

	appConfig := buildAppConfig(cfg)
	fmt.Printf("🚀 Starting application (%s)...\n", appConfig.DockerImage)
	if err := deps.manager.Start(ctx, appConfig); err != nil {
		return fmt.Errorf("failed to start application: %w", err)
	}

	fmt.Printf("✅ Application started\n")
	fmt.Printf("Use 'gtool app status' to check status\n")
	return nil
}

func runStop(_ *cobra.Command, _ []string) error {
	ctx := context.Background()
	log := logger.Default()
	defer log.Sync()

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
	defer deps.close()

	fmt.Println("🛑 Stopping application...")
	if err := deps.manager.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop application: %w", err)
	}

	fmt.Println("✅ Application stopped")
	return nil
}

func runRestart(_ *cobra.Command, _ []string) error {
	ctx := context.Background()
	log := logger.Default()
	defer log.Sync()

	cfg, err := loadConfigOrDefault(configFilePath())
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	deps, err := newAppDeps(log.Logger)
	if err != nil {
		return err
	}
	defer deps.close()

	fmt.Println("🔄 Restarting application...")
	if err := deps.manager.Restart(ctx, buildAppConfig(cfg)); err != nil {
		return fmt.Errorf("failed to restart application: %w", err)
	}

	fmt.Println("✅ Application restarted")
	return nil
}

func runStatus(_ *cobra.Command, _ []string) error {
	ctx := context.Background()
	log := zap.NewNop()

	deps, err := newAppDeps(log)
	if err != nil {
		return err
	}
	defer deps.close()

	status, err := deps.manager.Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get application status: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "STATUS\tIMAGE\tPORT")
	fmt.Fprintln(w, "------\t-----\t----")

	state := status.State
	if status.Running {
		state = "running ✓"
	}
	image := status.Image
	if image == "" {
		image = "-"
	}
	port := "-"
	if status.Port > 0 {
		port = fmt.Sprintf("%d", status.Port)
	}
	fmt.Fprintf(w, "%s\t%s\t%s\n", state, image, port)
	return w.Flush()
}

func runLogs(_ *cobra.Command, _ []string) error {
	ctx := context.Background()
	log := zap.NewNop()

	deps, err := newAppDeps(log)
	if err != nil {
		return err
	}
	defer deps.close()

	logs, err := deps.manager.Logs(ctx, logsTail)
	if err != nil {
		return fmt.Errorf("failed to get application logs: %w", err)
	}

	for _, line := range logs {
		fmt.Println(line)
	}
	return nil
}

// buildAppConfig maps the file configuration to a plugin.AppConfig, applying
// command-line flag overrides.
func buildAppConfig(cfg *config.Config) *plugin.AppConfig {
	ac := &plugin.AppConfig{
		BinaryName:  cfg.AppConfig.BinaryName,
		BinaryPath:  cfg.AppConfig.BinaryPath,
		DockerImage: cfg.AppConfig.DockerImage,
		Port:        cfg.AppConfig.Port,
		Environment: cfg.AppConfig.Environment,
	}

	if dockerImage != "" {
		ac.DockerImage = dockerImage
	}
	if appPort > 0 {
		ac.Port = appPort
	}
	if len(appEnv) > 0 {
		if ac.Environment == nil {
			ac.Environment = make(map[string]string, len(appEnv))
		}
		for k, v := range appEnv {
			ac.Environment[k] = v
		}
	}
	return ac
}

func loadConfigOrDefault(cfgFile string) (*config.Config, error) {
	if cfgFile != "" {
		return coreConfig.LoadConfig(cfgFile)
	}

	cfg, err := coreConfig.LoadConfig("")
	if err != nil {
		return config.DefaultConfig(), nil
	}
	return cfg, nil
}
