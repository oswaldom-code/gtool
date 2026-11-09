package services

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/oswaldom-code/gtool/internal/cli/cliconfig"
	"github.com/oswaldom-code/gtool/internal/core/mock"
	"github.com/oswaldom-code/gtool/internal/core/mock/stablemocks"
	"github.com/oswaldom-code/gtool/internal/core/ports"
	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/internal/infra/termout"
	"github.com/oswaldom-code/gtool/internal/plugin"
	pluginServices "github.com/oswaldom-code/gtool/internal/plugin/services"
	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
	"github.com/oswaldom-code/gtool/pkg/logger"
)

var (
	followLogs bool
	allLogs    bool
	tailLines  int
	stableMode bool
	allRuns    bool
	cfgFile    *string
)

// configFilePath returns the current --config value, dereferenced at run time
// because the persistent flag is parsed after the command is constructed.
func configFilePath() string {
	if cfgFile == nil {
		return ""
	}
	return *cfgFile
}

// serviceManager is the subset of *mock.Manager used by the commands. Depending
// on the interface (instead of the concrete type) lets tests inject a fake.
type serviceManager interface {
	Start(ctx context.Context, name string, config map[string]interface{}) error
	Stop(ctx context.Context, name string) error
	ListRunning() []string
	GetAllStatuses(ctx context.Context) []*mock.ServiceStatus
	GetLogs(ctx context.Context, name string, opts *plugin.LogOptions) ([]string, error)
}

// serviceDeps bundles the runtime dependencies a command needs. ping and close
// expose the underlying Docker client without leaking it to the command bodies.
type serviceDeps struct {
	manager serviceManager
	ping    func(ctx context.Context) error
	close   func() error
	follow  func(ctx context.Context, service string, tail int, w io.Writer) error
}

// depsFactory builds the dependencies for a command run. It is a package
// variable so tests can replace it with a Docker-free implementation.
type depsFactory func(cfg *config.Config, log *zap.Logger) (*serviceDeps, error)

var newServiceDeps depsFactory = defaultServiceDeps

// defaultServiceDeps wires the real Docker client, plugin registry and mock
// manager together.
func defaultServiceDeps(cfg *config.Config, log *zap.Logger) (*serviceDeps, error) {
	dockerClient, err := docker.NewClient(log)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrDockerConnection, "failed to create Docker client")
	}
	dockerClient.SetRunID(docker.ResolveRunID(false))
	dockerClient.SetAllRuns(allRuns)

	registry := plugin.NewRegistry()
	if err := pluginServices.RegisterAll(registry, dockerClient, log); err != nil {
		_ = dockerClient.Close()
		return nil, gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to register plugins")
	}

	return &serviceDeps{
		manager: mock.NewManager(registry, log, cfg.Orchestration, dockerClient),
		ping:    dockerClient.Ping,
		close:   dockerClient.Close,
		follow: func(ctx context.Context, service string, tail int, w io.Writer) error {
			return followService(ctx, dockerClient, service, tail, w)
		},
	}, nil
}

// followService streams the logs of the service's container. The lookup goes
// through the scoped client, so it follows the same run rules as down/status.
func followService(ctx context.Context, c *docker.Client, service string, tail int, w io.Writer) error {
	containers, err := c.ListContainersByLabels(ctx, map[string]string{"managed-by": "gtool", "service": service})
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		return gtErrors.New(gtErrors.ErrServiceNotRunning, fmt.Sprintf("service %s is not running", service))
	}
	return c.FollowContainerLogs(ctx, containers[0].ID, tail, w)
}

func NewServicesCmd(configFile *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "services",
		Aliases: []string{"s"},
		Short:   "Manage mock services (PostgreSQL, Kafka, etc.)",
		Long: `Manage mock services lifecycle.

Services are Docker containers that provide dependencies for testing:
  ` + strings.Join(pluginServices.Names(), ", ") + `

Examples:
  gtool services up                  # Start all configured services
  gtool s up postgresql              # Start only PostgreSQL
  gtool s down                       # Stop all services
  gtool s status                     # Show services status
  gtool s logs postgresql            # View PostgreSQL logs`,
	}

	// Store reference to config file
	cfgFile = configFile

	// Create subcommands
	upCmd := newServicesUpCmd()
	downCmd := newServicesDownCmd()
	statusCmd := newServicesStatusCmd()
	logsCmd := newServicesLogsCmd()

	// Add subcommands
	cmd.AddCommand(upCmd, downCmd, statusCmd, logsCmd)
	cmd.PersistentFlags().BoolVar(&allRuns, "all-runs", false,
		"without GTOOL_RUN_ID, also act on containers of other gtool runs (e.g. concurrent gtool test pipelines)")

	return cmd
}

func newServicesUpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "up [service...]",
		Short: "Start mock services",
		Long: `Start one or more mock services.

If no services are specified, starts all services defined in the configuration file.

Examples:
  gtool services up                  # Start all services from config
  gtool s up postgresql              # Start only PostgreSQL
  gtool s up postgresql kafka        # Start PostgreSQL and Kafka
  gtool s up --config my-config.yml  # Use specific config file
  gtool s up --stable                # Start the fixed-contract mocks seeded from test/component/mocks-data`,
		RunE: runServicesUp,
	}
	cmd.Flags().BoolVar(&stableMode, "stable", false, "use the fixed docker contract (public images, ports, mounts, seeding) for the component mocks")
	return cmd
}

func newServicesDownCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "down [service...]",
		Short: "Stop mock services",
		Long: `Stop one or more running mock services.

If no services are specified, stops all running services.

Examples:
  gtool services down                # Stop all services
  gtool s down postgresql            # Stop only PostgreSQL
  gtool s down postgresql kafka      # Stop PostgreSQL and Kafka
  gtool s down --stable              # Stop the fixed-contract mock containers`,
		RunE: runServicesDown,
	}
	cmd.Flags().BoolVar(&stableMode, "stable", false, "stop the fixed-contract mock containers")
	return cmd
}

func newServicesStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Aliases: []string{"ps"},
		Short:   "Show services status",
		Long: `Display the status of all managed services.

Shows which services are running, stopped, or in error state.

Examples:
  gtool services status              # Show all services
  gtool s status                     # Short form
  gtool s ps                         # Docker-like alias`,
		RunE: runServicesStatus,
	}
}

func newServicesLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs [service]",
		Short: "View service logs",
		Long: `Display logs from a service.

Name a service, or pass --all for every service.

Examples:
  gtool services logs postgresql     # View PostgreSQL logs
  gtool s logs postgresql -f         # Follow PostgreSQL logs
  gtool s logs --all                 # View all services logs
  gtool s logs postgresql --tail 100 # Last 100 lines`,
		RunE: runServicesLogs,
	}

	// Add flags
	cmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "follow log output")
	cmd.Flags().BoolVar(&allLogs, "all", false, "show logs from all services")
	cmd.Flags().IntVar(&tailLines, "tail", 100, "number of lines to show from the end of the logs")

	return cmd
}

func runServicesUp(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	policy := cliconfig.Start
	if stableMode {
		policy = cliconfig.Compat
	}
	cfg, err := cliconfig.Load(configFilePath(), policy, os.Stderr)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to load configuration")
	}

	if stableMode {
		return runStableUp(ctx, log.Logger, args, cfg)
	}

	if err := ports.Allocate(cfg, ports.NoAuto); err != nil {
		return err
	}

	deps, err := newServiceDeps(cfg, log.Logger)
	if err != nil {
		return err
	}
	defer func() { _ = deps.close() }()

	if err := deps.ping(ctx); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerConnection, "docker daemon not available")
	}

	mockManager := deps.manager

	servicesToStart := args
	if len(servicesToStart) == 0 {
		servicesToStart = cfg.ThirdParty.Mocks
	}

	if len(servicesToStart) == 0 {
		return gtErrors.New(gtErrors.ErrInvalidArgument, "no services specified and no services configured")
	}

	log.Info("starting services", zap.Strings("services", servicesToStart))

	for _, serviceName := range servicesToStart {
		termout.Printf("🚀 Starting %s...\n", serviceName)
		serviceConfig := cfg.ThirdParty.MockConfig[serviceName]
		var configMap map[string]interface{}
		if serviceConfig == nil {
			configMap = make(map[string]interface{})
		} else {
			var ok bool
			configMap, ok = serviceConfig.(map[string]interface{})
			if !ok {
				return gtErrors.New(gtErrors.ErrConfigInvalid, fmt.Sprintf("invalid configuration for service %s", serviceName))
			}
		}

		if err := mockManager.Start(ctx, serviceName, configMap); err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, fmt.Sprintf("failed to start %s", serviceName))
		}

		termout.Printf("✅ %s started successfully\n", serviceName)
	}

	termout.Printf("\n✨ All services started!\n\n")
	fmt.Printf("Use 'gtool s status' to check services status\n")
	fmt.Printf("Use 'gtool s logs <service>' to view logs\n")
	fmt.Printf("Use 'gtool s down' to stop all services\n")

	return nil
}

func runServicesDown(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := cliconfig.Load(configFilePath(), cliconfig.Inspect, os.Stderr)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to load configuration")
	}

	if stableMode {
		return runStableDown(ctx, log.Logger, args)
	}

	deps, err := newServiceDeps(cfg, log.Logger)
	if err != nil {
		return err
	}
	defer func() { _ = deps.close() }()

	mockManager := deps.manager
	servicesToStop := args
	if len(servicesToStop) == 0 {
		servicesToStop = mockManager.ListRunning()
	}

	if len(servicesToStop) == 0 {
		fmt.Println("No services to stop")
		return nil
	}

	log.Info("stopping services", zap.Strings("services", servicesToStop))

	// Stop services
	for _, serviceName := range servicesToStop {
		termout.Printf("🛑 Stopping %s...\n", serviceName)

		if err := mockManager.Stop(ctx, serviceName); err != nil {
			termout.Printf("⚠️  Failed to stop %s: %v\n", serviceName, err)
			continue
		}

		termout.Printf("✅ %s stopped\n", serviceName)
	}

	termout.Printf("\n✨ Services stopped\n")

	return nil
}

func runServicesStatus(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := zap.NewNop() // Silent logger for status

	cfg, err := cliconfig.Load(configFilePath(), cliconfig.Inspect, os.Stderr)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to load configuration")
	}

	deps, err := newServiceDeps(cfg, log)
	if err != nil {
		return err
	}
	defer func() { _ = deps.close() }()

	mockManager := deps.manager

	statuses := mockManager.GetAllStatuses(ctx)

	if len(statuses) == 0 {
		fmt.Println("No services found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "SERVICE\tSTATUS\tUPTIME\tPORT")
	_, _ = fmt.Fprintln(w, "-------\t------\t------\t----")

	for _, status := range statuses {
		uptimeStr := "-"
		if status.Uptime > 0 {
			uptimeStr = formatDuration(status.Uptime)
		}

		portStr := "-"
		if status.Port > 0 {
			portStr = fmt.Sprintf("%d", status.Port)
		}

		statusIcon := termout.Text("●")
		statusColor := status.Status
		switch status.Status {
		case "running":
			statusColor = termout.Text("running ✓")
		case "stopped":
			statusColor = termout.Text("stopped ●")
		case "error":
			statusColor = termout.Text("error ✗")
		}

		_, _ = fmt.Fprintf(w, "%s\t%s %s\t%s\t%s\n",
			status.Name, statusIcon, statusColor, uptimeStr, portStr)
	}

	return w.Flush()
}

func runServicesLogs(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := zap.NewNop() // Silent logger for status

	if len(args) == 0 && !allLogs {
		return gtErrors.New(gtErrors.ErrInvalidArgument, "please specify a service or use --all flag")
	}
	if len(args) > 0 && allLogs {
		return gtErrors.New(gtErrors.ErrInvalidArgument, "use either service names or --all, not both")
	}
	if followLogs && (allLogs || len(args) != 1) {
		return gtErrors.New(gtErrors.ErrInvalidArgument, "--follow streams one service at a time")
	}

	cfg, err := cliconfig.Load(configFilePath(), cliconfig.Inspect, os.Stderr)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to load configuration")
	}

	deps, err := newServiceDeps(cfg, log)
	if err != nil {
		return err
	}
	defer func() { _ = deps.close() }()

	if followLogs {
		return deps.follow(ctx, args[0], tailLines, cmd.OutOrStdout())
	}

	mockManager := deps.manager

	servicesToLog := args
	if allLogs {
		servicesToLog = mockManager.ListRunning()
	}

	if len(servicesToLog) == 0 {
		return gtErrors.New(gtErrors.ErrServiceNotRunning, "no running services found")
	}

	for _, serviceName := range servicesToLog {
		logs, err := mockManager.GetLogs(ctx, serviceName, &plugin.LogOptions{
			Tail:   tailLines,
			Follow: followLogs,
		})

		if err != nil {
			fmt.Printf("Failed to get logs for %s: %v\n", serviceName, err)
			continue
		}

		if len(servicesToLog) > 1 {
			fmt.Printf("\n=== %s ===\n", serviceName)
		}

		for _, line := range logs {
			fmt.Println(line)
		}
	}

	return nil
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// newStableLauncher builds a Docker-backed fixed-contract launcher and verifies
// the Docker daemon is reachable. The caller is responsible for closing the
// returned client.
func newStableLauncher(ctx context.Context, log *zap.Logger) (*stablemocks.Launcher, *docker.Client, error) {
	dockerClient, err := docker.NewClient(log)
	if err != nil {
		return nil, nil, gtErrors.Wrap(err, gtErrors.ErrDockerConnection, "failed to create Docker client")
	}
	if err := dockerClient.Ping(ctx); err != nil {
		_ = dockerClient.Close()
		return nil, nil, gtErrors.Wrap(err, gtErrors.ErrDockerConnection, "docker daemon not available")
	}
	launcher, err := stablemocks.New(dockerClient, log)
	if err != nil {
		_ = dockerClient.Close()
		return nil, nil, err
	}
	return launcher, dockerClient, nil
}

func runStableUp(ctx context.Context, log *zap.Logger, args []string, cfg *config.Config) error {
	launcher, dockerClient, err := newStableLauncher(ctx, log)
	if err != nil {
		return err
	}
	defer func() { _ = dockerClient.Close() }()

	if err := launcher.Up(ctx, args, cfg); err != nil {
		return err
	}

	termout.Printf("\n✨ Fixed-contract mocks started! Use 'gtool s down --stable' to stop them.\n")
	return nil
}

func runStableDown(ctx context.Context, log *zap.Logger, args []string) error {
	launcher, dockerClient, err := newStableLauncher(ctx, log)
	if err != nil {
		return err
	}
	defer func() { _ = dockerClient.Close() }()

	if err := launcher.Down(ctx, args); err != nil {
		return err
	}

	termout.Println("✨ Fixed-contract mocks stopped")
	return nil
}
