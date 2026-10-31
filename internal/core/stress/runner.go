package stress

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/oswaldom-code/gtool/internal/plugin"
	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

const (
	// DefaultReportsPath is used when stress-config.reports-path is empty.
	DefaultReportsPath = "test/stress/reports"

	readinessPollInterval = 500 * time.Millisecond
	readinessProbeTimeout = 2 * time.Second
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
	logger    *zap.Logger
	outcome   *Outcome
}

// NewRunner creates a stress runner. targetURL overrides the app address
// (http://localhost:<app-config.port>) when set.
func NewRunner(targetURL string, logger *zap.Logger) *Runner {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Runner{engine: NewEngine(logger), targetURL: targetURL, logger: logger}
}

// Outcome returns the result of the last completed run, or nil.
func (r *Runner) Outcome() *Outcome {
	return r.outcome
}

func (r *Runner) baseURL(cfg *config.Config) string {
	if r.targetURL != "" {
		return strings.TrimSuffix(r.targetURL, "/")
	}
	return fmt.Sprintf("http://localhost:%d", cfg.AppConfig.Port)
}

// waitReady polls the base URL until it answers with any HTTP status. A
// published container port accepts connections before the app listens, so
// a TCP check is not enough.
func waitReady(ctx context.Context, baseURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: readinessProbeTimeout}
	deadline := time.Now().Add(timeout)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/", nil)
		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, fmt.Sprintf("invalid target URL %s", baseURL))
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return gtErrors.Wrap(err, gtErrors.ErrServiceNotReady, fmt.Sprintf("target %s not reachable after %s", baseURL, timeout))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(readinessPollInterval):
		}
	}
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
	if err := waitReady(ctx, baseURL, cfg.Orchestration.StartupTimeout); err != nil {
		return nil, err
	}

	r.logger.Info("starting stress ramp", zap.String("url", baseURL), zap.Int("targets", len(plan.Targets)))
	stages, err := r.engine.Ramp(ctx, baseURL, plan.Targets, Stages(sc.Ramp), sc.Ramp.StepDuration,
		func(s StageMetrics) bool { return BreakingReason(s, sc.Thresholds) != "" })
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "stress ramp interrupted")
	}

	report := NewReport(baseURL, sc, plan, stages, Detect(stages, sc.Thresholds))
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
