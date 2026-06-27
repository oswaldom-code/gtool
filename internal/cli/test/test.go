package test

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	coreApp "github.com/oswaldo-montano/gtool/internal/core/app"
	"github.com/oswaldo-montano/gtool/internal/core/app/nativeapp"
	coreConfig "github.com/oswaldo-montano/gtool/internal/core/config"
	"github.com/oswaldo-montano/gtool/internal/core/mock"
	"github.com/oswaldo-montano/gtool/internal/core/mock/stablemocks"
	"github.com/oswaldo-montano/gtool/internal/core/orchestrator"
	coreTest "github.com/oswaldo-montano/gtool/internal/core/test"
	"github.com/oswaldo-montano/gtool/internal/core/test/stablekarate"
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
	tests := coreTest.NewKarateRunner(dockerClient, "", log)

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
	cmd.Flags().BoolVar(&stableMode, "stable", false, "reproduce legacy 'component t' using STABLE mocks + native app + Karate")
	cmd.Flags().StringVar(&stableTags, "tags", "", "Karate tags filter (with --stable)")
	cmd.Flags().StringVar(&stableBuildConfig, "build-config", "build-config.yml", "path to build-config.yml (with --stable)")
	cmd.Flags().BoolVar(&stableNoOpen, "no-open", false, "do not open the HTML report when finished (with --stable)")
	cmd.AddCommand(newKarateCmd())
	return cmd
}

var (
	stableMode        bool
	stableTags        string
	stableBuildConfig string
	stableNoOpen      bool
)

var (
	karateTags        string
	karateUrlsToBlock string
	karateFeatures    string
	karateReports     string
	karateImage       string
	karateNoOpen      bool
)

// newKarateCmd builds `gtool test karate`, reproducing the legacy
// "component e" (exec-only-tests): it runs the Karate launcher against the
// already-running mocks and app and opens the HTML report.
func newKarateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "karate",
		Short: "Run only the Karate component tests (mocks and app must already be running)",
		Long: `Run the Karate backend test launcher against the already-running mocks and
application, mounting test/component/features and writing the HTML report to
test/component/reports. The report is opened in the browser when it finishes.

Bring the environment up first with:
  gtool services up --stable
  gtool app start --native`,
		RunE: runKarate,
	}
	cmd.Flags().StringVar(&karateTags, "tags", "", "Karate tags filter (TAGS)")
	cmd.Flags().StringVar(&karateUrlsToBlock, "urls-to-block", "", "comma-separated URLs to block")
	cmd.Flags().StringVar(&karateFeatures, "features", "", "features path (default test/component/features)")
	cmd.Flags().StringVar(&karateReports, "reports", "", "reports path (default test/component/reports)")
	cmd.Flags().StringVar(&karateImage, "image", "", "override the test launcher image")
	cmd.Flags().BoolVar(&karateNoOpen, "no-open", false, "do not open the HTML report when finished")
	return cmd
}

func runKarate(_ *cobra.Command, _ []string) error {
	ctx := context.Background()
	log := logger.Default()
	defer log.Sync()

	dockerClient, err := docker.NewClient(log.Logger)
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer dockerClient.Close()
	if err := dockerClient.Ping(ctx); err != nil {
		return fmt.Errorf("Docker daemon not available: %w", err)
	}

	runner := stablekarate.New(dockerClient, log.Logger)
	result, err := runner.Run(ctx, stablekarate.Options{
		Image:        karateImage,
		Tags:         karateTags,
		UrlsToBlock:  karateUrlsToBlock,
		FeaturesPath: karateFeatures,
		ReportsPath:  karateReports,
		Open:         !karateNoOpen,
	})
	if err != nil {
		return err
	}
	if !result.Passed {
		return fmt.Errorf("karate tests failed (exit code %d)", result.ExitCode)
	}
	fmt.Printf("✅ Karate tests passed in %s\n", result.Duration.Round(time.Second))
	return nil
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

	if stableMode {
		return runStableTest(ctx, log.Logger, cfg)
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

	if result != nil && result.Test != nil && result.Test.Failed > 0 {
		return fmt.Errorf("tests failed: %d of %d failed", result.Test.Failed, result.Test.Total)
	}

	fmt.Println("\n✅ Pipeline completed")
	return nil
}

// runStableTest reproduces the legacy "component t": prepare STABLE mocks,
// launch the native app, run Karate, then always tear app and mocks down
// (LIFO defers run even on test failure or Ctrl-C).
func runStableTest(ctx context.Context, log *zap.Logger, cfg *config.Config) error {
	dockerClient, err := docker.NewClient(log)
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer dockerClient.Close()
	if err := dockerClient.Ping(ctx); err != nil {
		return fmt.Errorf("Docker daemon not available: %w", err)
	}

	mocks, err := stablemocks.New(dockerClient, log)
	if err != nil {
		return err
	}
	app, err := nativeapp.New(log, stableBuildConfig)
	if err != nil {
		return err
	}
	karate := stablekarate.New(dockerClient, log)

	fmt.Println("▶ Phase 1/3: starting STABLE mocks...")
	if err := mocks.Up(ctx, nil, cfg); err != nil {
		_ = mocks.Down(context.Background(), nil)
		return fmt.Errorf("failed to start mocks: %w", err)
	}
	defer func() {
		fmt.Println("🧹 Stopping mocks...")
		_ = mocks.Down(context.Background(), nil)
	}()

	fmt.Println("\n▶ Phase 2/3: starting native app...")
	if err := app.Start(ctx); err != nil {
		_ = app.Stop(context.Background())
		return fmt.Errorf("failed to start app: %w", err)
	}
	defer func() {
		fmt.Println("🧹 Stopping app...")
		_ = app.Stop(context.Background())
	}()

	fmt.Println("\n▶ Phase 3/3: running Karate tests...")
	result, err := karate.Run(ctx, stablekarate.Options{
		Tags: stableTags,
		Open: !stableNoOpen,
	})
	if err != nil {
		return fmt.Errorf("test run failed: %w", err)
	}
	if !result.Passed {
		return fmt.Errorf("tests failed (exit code %d)", result.ExitCode)
	}

	fmt.Printf("\n✅ Pipeline completed (Karate passed in %s)\n", result.Duration.Round(time.Second))
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
