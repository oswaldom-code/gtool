package test

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	coreApp "github.com/oswaldo-montano/gtool/internal/core/app"
	coreConfig "github.com/oswaldo-montano/gtool/internal/core/config"
	"github.com/oswaldo-montano/gtool/internal/core/mock"
	"github.com/oswaldo-montano/gtool/internal/core/orchestrator"
	"github.com/oswaldo-montano/gtool/internal/infra/docker"
	"github.com/oswaldo-montano/gtool/internal/plugin"
	pluginServices "github.com/oswaldo-montano/gtool/internal/plugin/services"
	"github.com/oswaldo-montano/gtool/pkg/config"
	"github.com/oswaldo-montano/gtool/pkg/logger"
)

// cfgFile points at the root --config flag value; it is dereferenced at run
// time because the persistent flag is parsed after the command is constructed.
var cfgFile *string

// pipeline is the orchestrator surface used by the command. *orchestrator.Orchestrator
// satisfies it; tests inject a fake.
type pipeline interface {
	Run(ctx context.Context) (*orchestrator.Result, error)
}

type pipelineDeps struct {
	pipeline pipeline
	close    func() error
}

type depsFactory func(cfg *config.Config, log *zap.Logger) (*pipelineDeps, error)

var newPipeline depsFactory = defaultPipeline

// defaultPipeline wires the real mock manager, app manager and (stub) test
// runner into an orchestrator.
func defaultPipeline(cfg *config.Config, log *zap.Logger) (*pipelineDeps, error) {
	dockerClient, err := docker.NewClient(log)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	registry := plugin.NewRegistry()
	if err := pluginServices.RegisterAll(registry, dockerClient, log); err != nil {
		_ = dockerClient.Close()
		return nil, fmt.Errorf("failed to register plugins: %w", err)
	}

	mockMgr := mock.NewManager(registry, log, cfg.Orchestration, dockerClient)
	appMgr := coreApp.NewDockerManager(dockerClient, log)
	tests := orchestrator.NewStubTestRunner(log)

	return &pipelineDeps{
		pipeline: orchestrator.NewOrchestrator(cfg, mockMgr, appMgr, tests, log),
		close:    dockerClient.Close,
	}, nil
}

// NewTestCmd builds the `test` command, which runs the full pipeline.
func NewTestCmd(configFile *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run the full component test pipeline",
		Long: `Run the complete component test pipeline with a single command:

  1. Start the configured mock services
  2. Start the application under test
  3. Run the tests
  4. Tear everything down

Whatever is started is always cleaned up, including on Ctrl-C.`,
		RunE: runTest,
	}

	cfgFile = configFile
	return cmd
}

// configFilePath returns the current value of the --config flag, or "".
func configFilePath() string {
	if cfgFile == nil {
		return ""
	}
	return *cfgFile
}

func runTest(_ *cobra.Command, _ []string) error {
	log := logger.Default()
	defer log.Sync()

	// Cancel the pipeline on Ctrl-C / SIGTERM; the orchestrator still tears down.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := loadConfigOrDefault(configFilePath())
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	deps, err := newPipeline(cfg, log.Logger)
	if err != nil {
		return err
	}
	defer deps.close()

	fmt.Println("🧪 Running component test pipeline...")
	result, runErr := deps.pipeline.Run(ctx)
	printResult(result)

	if runErr != nil {
		return fmt.Errorf("pipeline failed: %w", runErr)
	}

	fmt.Println("\n✅ Pipeline completed")
	return nil
}

func printResult(result *orchestrator.Result) {
	if result == nil {
		return
	}
	fmt.Printf("  mocks started: %v\n", result.MocksStarted)
	fmt.Printf("  app started:   %v\n", result.AppStarted)
	if result.Test != nil {
		fmt.Printf("  tests:         %d total, %d passed, %d failed\n",
			result.Test.Total, result.Test.Passed, result.Test.Failed)
	}
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
