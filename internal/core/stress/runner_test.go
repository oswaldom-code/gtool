package stress_test

import (
	"context"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oswaldom-code/gtool/internal/core/orchestrator"
	"github.com/oswaldom-code/gtool/internal/core/stress"
	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

var _ orchestrator.TestRunner = (*stress.Runner)(nil)

var _ = Describe("Runner", func() {
	var (
		server  *httptest.Server
		handler *hitCounter
		cfg     *config.Config
	)

	BeforeEach(func() {
		handler = &hitCounter{hits: map[string]int{}}
		server = httptest.NewServer(handler)
		cfg = config.DefaultConfig()
		cfg.Orchestration.StartupTimeout = 2 * time.Second
		cfg.StressConfig = &config.StressConfig{
			Targets:     []config.StressTarget{{Method: "GET", Path: "/ok"}},
			Ramp:        config.StressRamp{StartRPS: 20, StepRPS: 20, StepDuration: 250 * time.Millisecond, MaxRPS: 40},
			ReportsPath: filepath.Join(GinkgoT().TempDir(), "reports"),
		}
	})

	AfterEach(func() {
		server.Close()
	})

	It("ramps against the target URL, writes the reports and passes the gate", func() {
		cfg.StressConfig.MinAcceptableRPS = 40
		runner := stress.NewRunner(server.URL+"/", testTool, nil)
		Expect(runner.Outcome()).To(BeNil())

		result, err := runner.Run(context.Background(), cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Total).To(Equal(1))
		Expect(result.Passed).To(Equal(1))
		Expect(result.Failed).To(BeZero())

		outcome := runner.Outcome()
		Expect(outcome).NotTo(BeNil())
		Expect(outcome.Report.Stages).To(HaveLen(2))
		Expect(outcome.Report.TargetURL).To(Equal(server.URL))
		Expect(outcome.Report.Mode).To(Equal(stress.ModeTargetURL))
		Expect(outcome.Report.Generator).To(Equal(testTool))
		Expect(outcome.Report.DurationMs).To(BeNumerically(">", 0))
		Expect(outcome.Report.Breaking).To(BeNil())
		Expect(outcome.JSONPath).To(BeAnExistingFile())
		Expect(outcome.HTMLPath).To(BeAnExistingFile())
		Expect(result.ReportURL).To(Equal(outcome.HTMLPath))
		Expect(handler.count("/ok")).To(BeNumerically(">", 0))
	})

	It("defaults to the app port on localhost", func() {
		u, err := url.Parse(server.URL)
		Expect(err).NotTo(HaveOccurred())
		cfg.AppConfig.Port, err = strconv.Atoi(u.Port())
		Expect(err).NotTo(HaveOccurred())
		cfg.StressConfig.Ramp = config.StressRamp{StartRPS: 10, StepDuration: 100 * time.Millisecond, MaxRPS: 10}

		runner := stress.NewRunner("", testTool, nil)
		_, err = runner.Run(context.Background(), cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(runner.Outcome().Report.TargetURL).To(Equal("http://localhost:" + u.Port()))
		Expect(runner.Outcome().Report.Mode).To(Equal(stress.ModePipeline))
	})

	It("fails the gate and stops early when the target breaks below the minimum", func() {
		cfg.StressConfig.Targets[0].Path = "/fail"
		cfg.StressConfig.MinAcceptableRPS = 20
		runner := stress.NewRunner(server.URL, testTool, nil)

		result, err := runner.Run(context.Background(), cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Failed).To(Equal(1))
		Expect(result.Failures[0].Name).To(Equal("min-acceptable-rps"))
		Expect(result.Failures[0].Message).To(ContainSubstring("0 rps is below min-acceptable-rps 20"))
		Expect(runner.Outcome().Report.Stages).To(HaveLen(1))
		Expect(runner.Outcome().Report.Breaking.RPS).To(Equal(20))
	})

	It("requires the stress-config section", func() {
		cfg.StressConfig = nil
		_, err := stress.NewRunner(server.URL, testTool, nil).Run(context.Background(), cfg)
		Expect(gtErrors.Is(err, gtErrors.ErrConfigInvalid)).To(BeTrue())
	})

	It("propagates target resolution errors", func() {
		cfg.StressConfig.Targets = []config.StressTarget{{Method: "POST", Path: "/x", BodyFile: "/missing.json"}}
		_, err := stress.NewRunner(server.URL, testTool, nil).Run(context.Background(), cfg)
		Expect(err).To(HaveOccurred())
	})

	It("fails when the target never becomes reachable", func() {
		url := server.URL
		server.Close()
		cfg.Orchestration.StartupTimeout = 600 * time.Millisecond
		_, err := stress.NewRunner(url, testTool, nil).Run(context.Background(), cfg)
		Expect(gtErrors.Is(err, gtErrors.ErrServiceNotReady)).To(BeTrue())
	})

	It("rejects an invalid target URL", func() {
		_, err := stress.NewRunner("://bad", testTool, nil).Run(context.Background(), cfg)
		Expect(gtErrors.Is(err, gtErrors.ErrInvalidArgument)).To(BeTrue())
	})

	It("returns an error when cancelled during the ramp", func() {
		cfg.StressConfig.Ramp.StepDuration = 5 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		runner := stress.NewRunner(server.URL, testTool, nil)
		_, err := runner.Run(ctx, cfg)
		Expect(gtErrors.Is(err, gtErrors.ErrTestFailed)).To(BeTrue())
		Expect(runner.Outcome()).To(BeNil())
	})

	It("returns when cancelled while waiting for the target", func() {
		url := server.URL
		server.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_, err := stress.NewRunner(url, testTool, nil).Run(ctx, cfg)
		Expect(err).To(MatchError(context.DeadlineExceeded))
	})

	It("fails when the reports cannot be written", func() {
		cfg.StressConfig.Ramp = config.StressRamp{StartRPS: 10, StepDuration: 100 * time.Millisecond, MaxRPS: 10}
		cfg.StressConfig.ReportsPath = "/proc/gtool-stress-reports"
		_, err := stress.NewRunner(server.URL, testTool, nil).Run(context.Background(), cfg)
		Expect(err).To(HaveOccurred())
	})
})
