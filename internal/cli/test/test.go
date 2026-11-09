package test

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/oswaldom-code/gtool/internal/cli/cliconfig"
	coreApp "github.com/oswaldom-code/gtool/internal/core/app"
	"github.com/oswaldom-code/gtool/internal/core/app/nativeapp"
	"github.com/oswaldom-code/gtool/internal/core/mock"
	"github.com/oswaldom-code/gtool/internal/core/mock/stablemocks"
	"github.com/oswaldom-code/gtool/internal/core/orchestrator"
	"github.com/oswaldom-code/gtool/internal/core/ports"
	coreTest "github.com/oswaldom-code/gtool/internal/core/test"
	"github.com/oswaldom-code/gtool/internal/core/test/stablekarate"
	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/internal/infra/termout"
	"github.com/oswaldom-code/gtool/internal/plugin"
	pluginServices "github.com/oswaldom-code/gtool/internal/plugin/services"
	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
	"github.com/oswaldom-code/gtool/pkg/logger"
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
	dockerClient, err := docker.NewRunClient(log)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrDockerConnection, "failed to create Docker client")
	}

	registry := plugin.NewRegistry()
	if err := pluginServices.RegisterAll(registry, dockerClient, log); err != nil {
		_ = dockerClient.Close()
		return nil, gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to register plugins")
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
	cmd.Flags().BoolVar(&stableMode, "stable", false, "fixed-contract mode: fixed mocks + native app + Karate")
	cmd.Flags().StringVar(&stableTags, "tags", "", "Karate tags filter (overrides test-config.tags)")
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
	karateStable      bool
)

// karateRunner is the test-runner surface used by `gtool test karate`.
// *coreTest.KarateRunner satisfies it; tests inject a fake.
type karateRunner interface {
	Run(ctx context.Context, cfg *config.Config) (*plugin.TestResult, error)
}

type karateFactory func(image string, log *zap.Logger) (karateRunner, func() error, error)

var newKarateRunner karateFactory = defaultKarateRunner

func defaultKarateRunner(image string, log *zap.Logger) (karateRunner, func() error, error) {
	dockerClient, err := docker.NewClient(log)
	if err != nil {
		return nil, nil, gtErrors.Wrap(err, gtErrors.ErrDockerConnection, "failed to create Docker client")
	}
	return coreTest.NewKarateRunner(dockerClient, image, log), dockerClient.Close, nil
}

// newKarateCmd builds `gtool test karate`: it runs the Karate launcher against
// mocks and app that are already running. With --stable it uses the
// fixed-contract runner.
func newKarateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "karate",
		Short: "Run only the Karate component tests (mocks and app must already be running)",
		Long: `Run the Karate backend test launcher against the already-running mocks and
application, using test-config (features-path, reports-path, tags) and the
ports in the configuration. Ports must be explicit: auto is only supported
by gtool test and gtool stress.

Bring the environment up first with:
  gtool services up
  gtool app start

With --stable it uses the fixed-contract runner instead: it mounts
test/component/features, writes test/component/reports and opens the report.
Pair it with:
  gtool services up --stable
  gtool app start --native`,
		RunE: runKarate,
	}
	cmd.Flags().StringVar(&karateTags, "tags", "", "Karate tags filter (overrides test-config.tags)")
	cmd.Flags().StringVar(&karateFeatures, "features", "", "features path (overrides test-config.features-path)")
	cmd.Flags().StringVar(&karateReports, "reports", "", "reports path (overrides test-config.reports-path)")
	cmd.Flags().StringVar(&karateImage, "image", "", "override the test launcher image")
	cmd.Flags().BoolVar(&karateStable, "stable", false, "fixed-contract runner, paired with services up --stable + app start --native")
	cmd.Flags().StringVar(&karateUrlsToBlock, "urls-to-block", "", "comma-separated URLs to block (with --stable)")
	cmd.Flags().BoolVar(&karateNoOpen, "no-open", false, "do not open the HTML report when finished (with --stable)")
	return cmd
}

// karateStableOnlyFlags only change the --stable flow of `test karate`.
var karateStableOnlyFlags = []string{"urls-to-block", "no-open"}

func rejectStableOnlyFlags(cmd *cobra.Command, names []string) error {
	if cmd == nil {
		return nil
	}
	for _, name := range names {
		if f := cmd.Flag(name); f != nil && f.Changed {
			return gtErrors.New(gtErrors.ErrInvalidArgument, fmt.Sprintf("--%s only applies with --stable", name))
		}
	}
	return nil
}

func runStableKarate(ctx context.Context, log *zap.Logger) error {
	dockerClient, err := docker.NewClient(log)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerConnection, "failed to create Docker client")
	}
	defer func() { _ = dockerClient.Close() }()
	if err := dockerClient.Ping(ctx); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerConnection, "docker daemon not available")
	}

	runner := stablekarate.New(dockerClient, log)
	result, err := runner.Run(ctx, stablekarate.Options{
		Image:        karateImage,
		Tags:         karateTags,
		UrlsToBlock:  karateUrlsToBlock,
		FeaturesPath: karateFeatures,
		ReportsPath:  karateReports,
		Open:         !karateNoOpen && !termout.Plain(),
	})
	if err != nil {
		return err
	}
	if !result.Passed {
		return gtErrors.New(gtErrors.ErrTestFailed, fmt.Sprintf("karate tests failed (exit code %d)", result.ExitCode))
	}
	termout.Printf("✅ Karate tests passed in %s\n", result.Duration.Round(time.Second))
	return nil
}

// applyKarateOverrides applies the --tags, --features and --reports flags.
func applyKarateOverrides(cfg *config.Config) {
	if karateTags != "" {
		cfg.TestConfig.Tags = karateTags
	}
	if karateFeatures != "" {
		cfg.TestConfig.FeaturesPath = karateFeatures
	}
	if karateReports != "" {
		cfg.TestConfig.ReportsPath = karateReports
	}
}

func runNativeKarate(ctx context.Context, log *zap.Logger) error {
	cfg, err := cliconfig.Load(configFilePath(), cliconfig.Pipeline, os.Stderr)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to load configuration")
	}
	applyKarateOverrides(cfg)
	if _, err := ports.Resolve(cfg, ports.NoAuto); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to resolve ports")
	}

	runner, closeRunner, err := newKarateRunner(karateImage, log)
	if err != nil {
		return err
	}
	defer func() { _ = closeRunner() }()

	termout.Println("🧪 Running Karate tests...")
	result, err := runner.Run(ctx, cfg)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrTestFailed, "test run failed")
	}
	fmt.Printf("  tests: %d total, %d passed, %d failed\n", result.Total, result.Passed, result.Failed)
	if result.ReportURL != "" {
		fmt.Printf("  report: %s\n", filepath.Join(result.ReportURL, "karate-summary.html"))
	}
	if result.Failed > 0 {
		return gtErrors.New(gtErrors.ErrTestFailed, fmt.Sprintf("tests failed: %d of %d failed", result.Failed, result.Total))
	}
	termout.Printf("✅ Karate tests passed in %s\n", result.Duration.Round(time.Second))
	return nil
}

func runKarate(cmd *cobra.Command, _ []string) error {
	if !karateStable {
		if err := rejectStableOnlyFlags(cmd, karateStableOnlyFlags); err != nil {
			return err
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	if karateStable {
		return runStableKarate(ctx, log.Logger)
	}
	return runNativeKarate(ctx, log.Logger)
}

// configFilePath returns the current value of the --config flag, or "".
func configFilePath() string {
	if cfgFile == nil {
		return ""
	}
	return *cfgFile
}

// stableOnlyFlags only change the --stable flow; passing them without it was
// silently ignored before.
var stableOnlyFlags = []string{"build-config", "no-open"}

func runTest(cmd *cobra.Command, _ []string) error {
	if !stableMode {
		if err := rejectStableOnlyFlags(cmd, stableOnlyFlags); err != nil {
			return err
		}
	}

	log := logger.Default()
	defer func() { _ = log.Sync() }()

	// Cancel the pipeline on Ctrl-C / SIGTERM; the orchestrator still tears down.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	policy := cliconfig.Pipeline
	if stableMode {
		policy = cliconfig.Compat
	}
	cfg, err := cliconfig.Load(configFilePath(), policy, os.Stderr)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to load configuration")
	}

	if stableMode {
		return runStableTest(ctx, log.Logger, cfg)
	}
	if stableTags != "" {
		cfg.TestConfig.Tags = stableTags
	}

	values, err := ports.Resolve(cfg, ports.FreePortAllocator())
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to resolve ports")
	}
	log.Info("resolved ports", zap.Any("ports", values))

	deps, err := newPipeline(cfg, log.Logger)
	if err != nil {
		return err
	}
	defer func() { _ = deps.close() }()

	termout.Println("🧪 Running component test pipeline...")
	result, runErr := deps.pipeline.Run(ctx)
	printResult(result)

	if runErr != nil {
		return gtErrors.Wrap(runErr, gtErrors.ErrTestFailed, "pipeline failed")
	}

	if result != nil && result.Test != nil && result.Test.Failed > 0 {
		return gtErrors.New(gtErrors.ErrTestFailed, fmt.Sprintf("tests failed: %d of %d failed", result.Test.Failed, result.Test.Total))
	}

	termout.Println("\n✅ Pipeline completed")
	return nil
}

// runStableTest runs the fixed-contract mode: prepare the fixed mocks,
// launch the native app, run Karate, then always tear app and mocks down
// (LIFO defers run even on test failure or Ctrl-C).
func runStableTest(ctx context.Context, log *zap.Logger, cfg *config.Config) error {
	dockerClient, err := docker.NewClient(log)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerConnection, "failed to create Docker client")
	}
	defer func() { _ = dockerClient.Close() }()
	if err := dockerClient.Ping(ctx); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerConnection, "docker daemon not available")
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

	termout.Println("▶ Phase 1/3: starting fixed-contract mocks...")
	if err := mocks.Up(ctx, nil, cfg); err != nil {
		_ = mocks.Down(context.Background(), nil)
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to start mocks")
	}
	defer func() {
		termout.Println("🧹 Stopping mocks...")
		_ = mocks.Down(context.Background(), nil)
	}()

	termout.Println("\n▶ Phase 2/3: starting native app...")
	if err := app.Start(ctx); err != nil {
		_ = app.Stop(context.Background())
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to start app")
	}
	defer func() {
		termout.Println("🧹 Stopping app...")
		_ = app.Stop(context.Background())
	}()

	termout.Println("\n▶ Phase 3/3: running Karate tests...")
	result, err := karate.Run(ctx, stablekarate.Options{
		Tags: stableTags,
		Open: !stableNoOpen && !termout.Plain(),
	})
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrTestFailed, "test run failed")
	}
	if !result.Passed {
		return gtErrors.New(gtErrors.ErrTestFailed, fmt.Sprintf("tests failed (exit code %d)", result.ExitCode))
	}

	termout.Printf("\n✅ Pipeline completed (Karate passed in %s)\n", result.Duration.Round(time.Second))
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
