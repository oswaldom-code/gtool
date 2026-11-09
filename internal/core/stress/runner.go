package stress

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/oswaldom-code/gtool/internal/core/ports"
	"github.com/oswaldom-code/gtool/internal/core/readiness"
	"github.com/oswaldom-code/gtool/internal/plugin"
	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

const (
	// DefaultReportsPath is used when stress-config.reports-path is empty.
	DefaultReportsPath = "test/stress/reports"
)

// Outcome is the result of the last stress run.
type Outcome struct {
	Report   *Report
	JSONPath string
	HTMLPath string
}

// Runner resolves the targets, ramps the load, detects the points and writes
// the reports. It satisfies the orchestrator TestRunner, so the pipeline runs
// it in place of the Karate suite.
type Runner struct {
	engine    *Engine
	targetURL string
	tool      ToolInfo
	logger    *zap.Logger
	outcome   *Outcome
}

// NewRunner creates a stress runner. targetURL overrides the app address
// (http://localhost:<app-config.port>) when set; tool identifies the gtool
// build in the reports.
func NewRunner(targetURL string, tool ToolInfo, logger *zap.Logger) *Runner {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Runner{engine: NewEngine(logger), targetURL: targetURL, tool: tool, logger: logger}
}

func (r *Runner) mode() string {
	if r.targetURL != "" {
		return ModeTargetURL
	}
	return ModePipeline
}

// Outcome returns the result of the last completed run, or nil.
func (r *Runner) Outcome() *Outcome {
	return r.outcome
}

func (r *Runner) baseURL(cfg *config.Config) string {
	if r.targetURL != "" {
		return strings.TrimSuffix(r.targetURL, "/")
	}
	return fmt.Sprintf("http://localhost:%d", ports.AppHostPort(cfg))
}

func reportsPath(sc *config.StressConfig) string {
	if sc.ReportsPath == "" {
		return DefaultReportsPath
	}
	return sc.ReportsPath
}

// Run executes the stress test. The returned result has one check, the
// min-acceptable-rps gate.
func (r *Runner) Run(ctx context.Context, cfg *config.Config) (*plugin.TestResult, error) {
	sc := cfg.StressConfig
	if sc == nil {
		return nil, gtErrors.New(gtErrors.ErrConfigInvalid, "stress-config section is required")
	}
	start := time.Now()

	plan, err := ResolveTargets(sc)
	if err != nil {
		return nil, err
	}

	baseURL := r.baseURL(cfg)
	r.logger.Info("waiting for target", zap.String("url", baseURL))
	if err := readiness.WaitHTTP(ctx, baseURL+"/", cfg.Orchestration.StartupTimeout); err != nil {
		return nil, err
	}

	r.logger.Info("starting stress ramp", zap.String("url", baseURL), zap.Int("targets", len(plan.Targets)))
	stages, err := r.engine.Ramp(ctx, baseURL, plan.Targets, Stages(sc.Ramp), sc.Ramp.StepDuration,
		func(s StageMetrics) bool { return BreakingReason(s, sc.Thresholds) != "" })
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "stress ramp interrupted")
	}

	run := RunInfo{TargetURL: baseURL, Mode: r.mode(), StartedAt: start, Tool: r.tool}
	report := NewReport(run, sc, plan, stages, Detect(stages, sc.Thresholds))
	jsonPath, htmlPath, err := report.WriteFiles(reportsPath(sc))
	if err != nil {
		return nil, err
	}
	r.outcome = &Outcome{Report: report, JSONPath: jsonPath, HTMLPath: htmlPath}

	result := &plugin.TestResult{Total: 1, Duration: time.Since(start), ReportURL: htmlPath}
	if report.Passed {
		result.Passed = 1
	} else {
		result.Failed = 1
		result.Failures = []plugin.TestFailure{{
			Name: "min-acceptable-rps",
			Message: fmt.Sprintf("max sustained load %d rps is below min-acceptable-rps %d",
				report.MaxSustainedRPS, sc.MinAcceptableRPS),
		}}
	}
	return result, nil
}
