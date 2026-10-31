package stress

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

	coreApp "github.com/oswaldom-code/gtool/internal/core/app"
	coreConfig "github.com/oswaldom-code/gtool/internal/core/config"
	"github.com/oswaldom-code/gtool/internal/core/mock"
	"github.com/oswaldom-code/gtool/internal/core/orchestrator"
	coreStress "github.com/oswaldom-code/gtool/internal/core/stress"
	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/internal/plugin"
	pluginServices "github.com/oswaldom-code/gtool/internal/plugin/services"
	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
	"github.com/oswaldom-code/gtool/pkg/logger"
)

// stressRunner is the stress runner surface used by the command.
// *coreStress.Runner satisfies it; tests inject a fake.
type stressRunner interface {
	orchestrator.TestRunner
	Outcome() *coreStress.Outcome
}

// pipeline is the orchestrator surface used by the command.
type pipeline interface {
	Run(ctx context.Context) (*orchestrator.Result, error)
}

type pipelineFactory func(cfg *config.Config, tests orchestrator.TestRunner, log *zap.Logger) (pipeline, func() error, error)

type runnerFactory func(targetURL string, log *zap.Logger) stressRunner

type deps struct {
	newPipeline pipelineFactory
	newRunner   runnerFactory
}

// defaultPipeline wires the real mock and app managers with the stress runner
// as the test phase, so mocks and app are always torn down.
func defaultPipeline(cfg *config.Config, tests orchestrator.TestRunner, log *zap.Logger) (pipeline, func() error, error) {
	dockerClient, err := docker.NewClient(log)
	if err != nil {
		return nil, nil, gtErrors.Wrap(err, gtErrors.ErrDockerConnection, "failed to create Docker client")
	}

	registry := plugin.NewRegistry()
	if err := pluginServices.RegisterAll(registry, dockerClient, log); err != nil {
		_ = dockerClient.Close()
		return nil, nil, gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to register plugins")
	}

	mockMgr := mock.NewManager(registry, log, cfg.Orchestration, dockerClient)
	appMgr := coreApp.NewDockerManager(dockerClient, log)
	return orchestrator.NewOrchestrator(cfg, mockMgr, appMgr, tests, log), dockerClient.Close, nil
}

var defaultDeps = deps{
	newPipeline: defaultPipeline,
	newRunner: func(targetURL string, log *zap.Logger) stressRunner {
		return coreStress.NewRunner(targetURL, log)
	},
}

func configPath(configFile *string) string {
	if configFile == nil {
		return ""
	}
	return *configFile
}

// loadStressConfig loads the config and validates only its stress-config.
func loadStressConfig(path string) (*config.Config, error) {
	cfg, err := coreConfig.LoadConfig(path)
	if err != nil {
		return nil, err
	}
	if err := coreConfig.NewValidator().ValidateStress(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func sampleURL(t coreStress.Target) string {
	if len(t.URLs) > 1 {
		return fmt.Sprintf("%s (+%d variants)", t.URLs[0], len(t.URLs)-1)
	}
	return t.URLs[0]
}

// printPlan writes the resolved targets, the ramp and the warnings.
func printPlan(w io.Writer, sc *config.StressConfig, plan *coreStress.Plan) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Targets (%d):\n", len(plan.Targets))
	// The tabwriter wraps a strings.Builder: writes cannot fail and Flush
	// reports any formatting error.
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "  NAME\tMETHOD\tPATH\tWEIGHT\tSOURCE\tREQUEST")
	for _, t := range plan.Targets {
		_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%d\t%s\t%s\n", t.Name, t.Method, t.Path, t.Weight, t.Source, sampleURL(t))
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	stages := coreStress.Stages(sc.Ramp)
	fmt.Fprintf(&b, "\nRamp: %d → %d rps, +%d every %s (%d stages, up to %s)\n",
		sc.Ramp.StartRPS, sc.Ramp.MaxRPS, sc.Ramp.StepRPS, sc.Ramp.StepDuration,
		len(stages), time.Duration(len(stages))*sc.Ramp.StepDuration)
	for _, warn := range plan.Warnings {
		fmt.Fprintf(&b, "Warning: %s\n", warn)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func runPlan(cmd *cobra.Command, configFile *string) error {
	cfg, err := loadStressConfig(configPath(configFile))
	if err != nil {
		return err
	}
	plan, err := coreStress.ResolveTargets(cfg.StressConfig)
	if err != nil {
		return err
	}
	return printPlan(cmd.OutOrStdout(), cfg.StressConfig, plan)
}

func newPlanCmd(configFile *string) *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "Print the resolved stress targets without running any load",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPlan(cmd, configFile)
		},
	}
}

// execute runs the stress test, through the full pipeline or directly
// against --target-url.
func execute(ctx context.Context, cfg *config.Config, targetURL string, d deps, log *zap.Logger) (stressRunner, *plugin.TestResult, error) {
	runner := d.newRunner(targetURL, log)
	if targetURL != "" {
		result, err := runner.Run(ctx, cfg)
		return runner, result, err
	}

	p, closeFn, err := d.newPipeline(cfg, runner, log)
	if err != nil {
		return runner, nil, err
	}
	defer func() { _ = closeFn() }()

	res, err := p.Run(ctx)
	if res == nil {
		return runner, nil, err
	}
	return runner, res.Test, err
}

func runStress(cmd *cobra.Command, configFile *string, targetURL string, d deps) error {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	// Cancel on Ctrl-C / SIGTERM; the orchestrator still tears everything down.
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := loadStressConfig(configPath(configFile))
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	runner, result, err := execute(ctx, cfg, targetURL, d, log.Logger)
	if outcome := runner.Outcome(); outcome != nil {
		var b strings.Builder
		b.WriteString("\n")
		if werr := outcome.Report.WriteSummary(&b); werr != nil {
			return werr
		}
		fmt.Fprintf(&b, "Reports: %s, %s\n", outcome.JSONPath, outcome.HTMLPath)
		if _, werr := io.WriteString(out, b.String()); werr != nil {
			return werr
		}
	}
	if err != nil {
		return err
	}
	if result != nil && result.Failed > 0 {
		msgs := make([]string, 0, len(result.Failures))
		for _, f := range result.Failures {
			msgs = append(msgs, f.Message)
		}
		return gtErrors.New(gtErrors.ErrTestFailed, "stress gate failed: "+strings.Join(msgs, "; "))
	}
	return nil
}

func newStressCmd(configFile *string, d deps) *cobra.Command {
	var targetURL string
	cmd := &cobra.Command{
		Use:   "stress",
		Short: "Measure the app under increasing load and find its degradation and breaking points",
		Long: `Run a stepped load ramp against the application and report where it
degrades (p95 latency or throughput) and where it breaks (error rate):

  1. Start the configured mock services
  2. Start the application under test
  3. Ramp the load over the targets resolved from stress-config
  4. Write JSON and HTML reports and tear everything down

With --target-url the mocks and app phases are skipped and the load hits that
URL instead. The command fails when the sustained load is below
stress-config.min-acceptable-rps.`,
		// A failed gate is not a usage error; keep CI logs clean.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStress(cmd, configFile, targetURL, d)
		},
	}
	cmd.Flags().StringVar(&targetURL, "target-url", "", "hit this base URL instead of starting mocks and app")
	cmd.AddCommand(newPlanCmd(configFile))
	return cmd
}

// NewStressCmd builds the `stress` command and its `plan` subcommand.
func NewStressCmd(configFile *string) *cobra.Command {
	return newStressCmd(configFile, defaultDeps)
}
