package stress

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/oswaldom-code/gtool/internal/core/orchestrator"
	coreStress "github.com/oswaldom-code/gtool/internal/core/stress"
	"github.com/oswaldom-code/gtool/internal/plugin"
	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

func TestStressCmd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Stress CLI Suite")
}

const stressConfigYAML = `version: v1
app-technology: golang
app-config:
  port: 8080
stress-config:
  source:
    openapi: ../../../test/fixtures/stress/openapi3.yml
    param-values:
      productId: ["1", "2"]
  targets:
    - name: extra
      method: POST
      path: /extra
      body: '{"a":1}'
  ramp:
    start-rps: 10
    step-rps: 10
    step-duration: 1s
    max-rps: 30
  min-acceptable-rps: 20
`

type fakeRunner struct {
	targetURL string
	result    *plugin.TestResult
	err       error
	outcome   *coreStress.Outcome
	ran       bool
}

func (f *fakeRunner) Run(_ context.Context, _ *config.Config) (*plugin.TestResult, error) {
	f.ran = true
	return f.result, f.err
}

func (f *fakeRunner) Outcome() *coreStress.Outcome {
	if !f.ran {
		return nil
	}
	return f.outcome
}

// fakePipeline runs the test phase like the orchestrator does.
type fakePipeline struct {
	cfg    *config.Config
	tests  orchestrator.TestRunner
	err    error
	called bool
}

func (f *fakePipeline) Run(ctx context.Context) (*orchestrator.Result, error) {
	f.called = true
	if f.err != nil {
		return nil, f.err
	}
	res, err := f.tests.Run(ctx, f.cfg)
	return &orchestrator.Result{MocksStarted: true, AppStarted: true, Test: res}, err
}

func sampleOutcome(passed bool) *coreStress.Outcome {
	sc := &config.StressConfig{Ramp: config.StressRamp{StartRPS: 10, MaxRPS: 10}, MinAcceptableRPS: 10}
	stages := []coreStress.StageMetrics{{RequestedRPS: 10, Throughput: 10}}
	if !passed {
		stages[0].ErrorRate = 1
	}
	plan := &coreStress.Plan{Targets: []coreStress.Target{{Name: "t", Method: "GET", Path: "/"}}}
	report := coreStress.NewReport("http://app", sc, plan, stages, coreStress.Detect(stages, sc.Thresholds))
	return &coreStress.Outcome{Report: report, JSONPath: "r/stress-report.json", HTMLPath: "r/stress-report.html"}
}

var _ = Describe("stress command", func() {
	var (
		cfgPath  string
		out      *bytes.Buffer
		runner   *fakeRunner
		pipe     *fakePipeline
		d        deps
		pipeErr  error
		runCmd   func(args ...string) error
		closed   bool
		usedPipe bool
	)

	BeforeEach(func() {
		cfgPath = filepath.Join(GinkgoT().TempDir(), "component-config.yml")
		Expect(os.WriteFile(cfgPath, []byte(stressConfigYAML), 0o644)).To(Succeed())
		out = &bytes.Buffer{}
		runner = &fakeRunner{result: &plugin.TestResult{Total: 1, Passed: 1}, outcome: sampleOutcome(true)}
		pipe = &fakePipeline{}
		pipeErr, closed, usedPipe = nil, false, false

		d = deps{
			newRunner: func(targetURL string, _ *zap.Logger) stressRunner {
				runner.targetURL = targetURL
				return runner
			},
			newPipeline: func(cfg *config.Config, tests orchestrator.TestRunner, _ *zap.Logger) (pipeline, func() error, error) {
				usedPipe = true
				if pipeErr != nil {
					return nil, nil, pipeErr
				}
				pipe.cfg, pipe.tests = cfg, tests
				return pipe, func() error { closed = true; return nil }, nil
			},
		}
		runCmd = func(args ...string) error {
			cmd := newStressCmd(&cfgPath, d)
			cmd.SetOut(out)
			cmd.SetErr(out)
			cmd.SetArgs(args)
			return cmd.Execute()
		}
	})

	It("is exposed as `stress` with a `plan` subcommand", func() {
		cmd := NewStressCmd(nil)
		Expect(cmd.Name()).To(Equal("stress"))
		Expect(cmd.Flags().Lookup("target-url")).NotTo(BeNil())
		sub, _, err := cmd.Find([]string{"plan"})
		Expect(err).NotTo(HaveOccurred())
		Expect(sub.Name()).To(Equal("plan"))
	})

	Describe("plan", func() {
		It("prints the resolved targets, the ramp and the warnings without running load", func() {
			Expect(runCmd("plan")).To(Succeed())
			text := out.String()
			Expect(text).To(ContainSubstring("Targets (5):"))
			Expect(text).To(MatchRegexp(`listProducts\s+GET\s+/api/v1/products\s+1\s+openapi\s+/api/v1/products\?limit=20`))
			Expect(text).To(ContainSubstring("/api/v1/products/1 (+1 variants)"))
			Expect(text).To(MatchRegexp(`extra\s+POST\s+/extra\s+1\s+config`))
			Expect(text).To(ContainSubstring("Ramp: 10 → 30 rps, +10 every 1s (3 stages, up to 3s)"))
			Expect(text).To(ContainSubstring("Warning: mutating methods enabled (extra)"))
			Expect(runner.ran).To(BeFalse())
			Expect(usedPipe).To(BeFalse())
		})

		It("fails on a config without stress-config", func() {
			Expect(os.WriteFile(cfgPath, []byte("version: v1\n"), 0o644)).To(Succeed())
			err := runCmd("plan")
			Expect(gtErrors.Is(err, gtErrors.ErrConfigInvalid)).To(BeTrue())
		})

		It("fails when the config file does not exist", func() {
			cfgPath = "/does/not/exist.yml"
			Expect(runCmd("plan")).To(HaveOccurred())
		})

		It("fails when no target is resolved", func() {
			Expect(os.WriteFile(cfgPath, []byte(`version: v1
stress-config:
  source:
    openapi: ../../../test/fixtures/stress/openapi3.yml
    include-tags: [none]
  ramp: {start-rps: 1, step-duration: 1s, max-rps: 1}
`), 0o644)).To(Succeed())
			err := runCmd("plan")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no stress targets resolved"))
		})
	})

	Describe("with --target-url", func() {
		It("skips the pipeline and prints the summary and report paths", func() {
			Expect(runCmd("--target-url", "http://remote:9000")).To(Succeed())
			Expect(runner.targetURL).To(Equal("http://remote:9000"))
			Expect(usedPipe).To(BeFalse())
			Expect(out.String()).To(ContainSubstring("Stress test against http://app"))
			Expect(out.String()).To(ContainSubstring("Reports: r/stress-report.json, r/stress-report.html"))
		})

		It("returns a non-zero exit error when the gate fails", func() {
			runner.outcome = sampleOutcome(false)
			runner.result = &plugin.TestResult{Total: 1, Failed: 1, Failures: []plugin.TestFailure{
				{Name: "min-acceptable-rps", Message: "max sustained load 0 rps is below min-acceptable-rps 10"}}}
			err := runCmd("--target-url", "http://remote:9000")
			Expect(gtErrors.Is(err, gtErrors.ErrTestFailed)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("stress gate failed: max sustained load 0 rps"))
			Expect(out.String()).To(ContainSubstring("FAIL"))
		})

		It("propagates runner errors", func() {
			runner.err = errors.New("unreachable")
			Expect(runCmd("--target-url", "http://remote:9000")).To(MatchError("unreachable"))
		})
	})

	Describe("through the pipeline", func() {
		It("runs the stress runner as the test phase and closes the deps", func() {
			Expect(runCmd()).To(Succeed())
			Expect(pipe.called).To(BeTrue())
			Expect(pipe.tests).To(BeIdenticalTo(runner))
			Expect(pipe.cfg.StressConfig.MinAcceptableRPS).To(Equal(20))
			Expect(runner.targetURL).To(BeEmpty())
			Expect(closed).To(BeTrue())
			Expect(out.String()).To(ContainSubstring("Reports:"))
		})

		It("fails when the pipeline cannot be built", func() {
			pipeErr = errors.New("docker down")
			Expect(runCmd()).To(MatchError("docker down"))
		})

		It("fails when the pipeline fails before the stress phase", func() {
			pipe.err = errors.New("mocks failed")
			Expect(runCmd()).To(MatchError("mocks failed"))
			Expect(closed).To(BeTrue())
			Expect(out.String()).NotTo(ContainSubstring("Reports:"))
		})
	})
})
