package services

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	coreConfig "github.com/oswaldo-montano/gtool/internal/core/config"
	"github.com/oswaldo-montano/gtool/internal/core/mock"
	"github.com/oswaldo-montano/gtool/internal/infra/docker"
	"github.com/oswaldo-montano/gtool/internal/plugin"
	pluginServices "github.com/oswaldo-montano/gtool/internal/plugin/services"
	"github.com/oswaldo-montano/gtool/pkg/config"
	"github.com/oswaldo-montano/gtool/pkg/logger"
)

var (
	followLogs bool
	allLogs    bool
	tailLines  int
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
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	registry := plugin.NewRegistry()
	if err := pluginServices.RegisterAll(registry, dockerClient, log); err != nil {
		_ = dockerClient.Close()
		return nil, fmt.Errorf("failed to register plugins: %w", err)
	}

	return &serviceDeps{
		manager: mock.NewManager(registry, log, cfg.Orchestration, dockerClient),
		ping:    dockerClient.Ping,
		close:   dockerClient.Close,
	}, nil
}

func NewServicesCmd(configFile *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "services",
		Aliases: []string{"s"},
		Short:   "Manage mock services (PostgreSQL, Kafka, etc.)",
		Long: `Manage mock services lifecycle.

Services are Docker containers that provide dependencies for testing:
  - PostgreSQL
  - Couchbase
  - Kafka
  - Mountebank
  - Pub/Sub
  - GCS

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

	return cmd
}

func newServicesUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up [service...]",
		Short: "Start mock services",
		Long: `Start one or more mock services.

If no services are specified, starts all services defined in the configuration file.

Examples:
  gtool services up                  # Start all services from config
  gtool s up postgresql              # Start only PostgreSQL
  gtool s up postgresql kafka        # Start PostgreSQL and Kafka
  gtool s up --config my-config.yml  # Use specific config file`,
		RunE: runServicesUp,
	}
}

func newServicesDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down [service...]",
		Short: "Stop mock services",
		Long: `Stop one or more running mock services.

If no services are specified, stops all running services.

Examples:
  gtool services down                # Stop all services
  gtool s down postgresql            # Stop only PostgreSQL
  gtool s down postgresql kafka      # Stop PostgreSQL and Kafka`,
		RunE: runServicesDown,
	}
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

If no service is specified, shows logs from all services.

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
	ctx := context.Background()
	log := logger.Default()
	defer log.Sync()

	cfg, err := loadConfigOrDefault(configFilePath())
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	deps, err := newServiceDeps(cfg, log.Logger)
	if err != nil {
		return err
	}
	defer deps.close()

	if err := deps.ping(ctx); err != nil {
		return fmt.Errorf("Docker daemon not available: %w", err)
	}

	mockManager := deps.manager

	servicesToStart := args
	if len(servicesToStart) == 0 {
		servicesToStart = cfg.ThirdParty.Mocks
	}

	if len(servicesToStart) == 0 {
		return fmt.Errorf("no services specified and no services configured")
	}

	log.Info("starting services", zap.Strings("services", servicesToStart))

	for _, serviceName := range servicesToStart {
		fmt.Printf("🚀 Starting %s...\n", serviceName)
		serviceConfig := cfg.ThirdParty.MockConfig[serviceName]
		var configMap map[string]interface{}
		if serviceConfig == nil {
			configMap = make(map[string]interface{})
		} else {
			var ok bool
			configMap, ok = serviceConfig.(map[string]interface{})
			if !ok {
				return fmt.Errorf("invalid configuration for service %s", serviceName)
			}
		}

		if err := mockManager.Start(ctx, serviceName, configMap); err != nil {
			return fmt.Errorf("failed to start %s: %w", serviceName, err)
		}

		fmt.Printf("✅ %s started successfully\n", serviceName)
	}

	fmt.Printf("\n✨ All services started!\n\n")
	fmt.Printf("Use 'gtool s status' to check services status\n")
	fmt.Printf("Use 'gtool s logs <service>' to view logs\n")
	fmt.Printf("Use 'gtool s down' to stop all services\n")

	return nil
}

func runServicesDown(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	log := logger.Default()
	defer log.Sync()

	cfg, err := loadConfigOrDefault(configFilePath())
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	deps, err := newServiceDeps(cfg, log.Logger)
	if err != nil {
		return err
	}
	defer deps.close()

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
		fmt.Printf("🛑 Stopping %s...\n", serviceName)

		if err := mockManager.Stop(ctx, serviceName); err != nil {
			fmt.Printf("⚠️  Failed to stop %s: %v\n", serviceName, err)
			continue
		}

		fmt.Printf("✅ %s stopped\n", serviceName)
	}

	fmt.Printf("\n✨ Services stopped\n")

	return nil
}

func runServicesStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	log := zap.NewNop() // Silent logger for status

	cfg, err := loadConfigOrDefault(configFilePath())
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	deps, err := newServiceDeps(cfg, log)
	if err != nil {
		return err
	}
	defer deps.close()

	mockManager := deps.manager

	statuses := mockManager.GetAllStatuses(ctx)

	if len(statuses) == 0 {
		fmt.Println("No services found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "SERVICE\tSTATUS\tUPTIME\tPORT")
	fmt.Fprintln(w, "-------\t------\t------\t----")

	for _, status := range statuses {
		uptimeStr := "-"
		if status.Uptime > 0 {
			uptimeStr = formatDuration(status.Uptime)
		}

		portStr := "-"
		if status.Port > 0 {
			portStr = fmt.Sprintf("%d", status.Port)
		}

		statusIcon := "●"
		statusColor := status.Status
		if status.Status == "running" {
			statusColor = "running ✓"
		} else if status.Status == "stopped" {
			statusColor = "stopped ●"
		} else if status.Status == "error" {
			statusColor = "error ✗"
		}

		fmt.Fprintf(w, "%s\t%s %s\t%s\t%s\n",
			status.Name, statusIcon, statusColor, uptimeStr, portStr)
	}

	w.Flush()

	return nil
}

func runServicesLogs(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	log := zap.NewNop() // Silent logger for status

	if len(args) == 0 && !allLogs {
		return fmt.Errorf("please specify a service or use --all flag")
	}

	cfg, err := loadConfigOrDefault(configFilePath())
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	deps, err := newServiceDeps(cfg, log)
	if err != nil {
		return err
	}
	defer deps.close()

	mockManager := deps.manager

	servicesToLog := args
	if allLogs {
		servicesToLog = mockManager.ListRunning()
	}

	if len(servicesToLog) == 0 {
		return fmt.Errorf("no running services found")
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

func loadConfigOrDefault(cfgFile string) (*config.Config, error) {
	if cfgFile != "" {
		cfg, err := coreConfig.LoadConfig(cfgFile)
		if err != nil {
			return nil, err
		}
		return cfg, nil
	}

	cfg, err := coreConfig.LoadConfig("")
	if err != nil {
		fmt.Println("ℹ️  No configuration file found, using defaults")
		return config.DefaultConfig(), nil
	}

	return cfg, nil
}
