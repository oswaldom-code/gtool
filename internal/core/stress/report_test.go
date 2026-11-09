package stress_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oswaldom-code/gtool/internal/core/stress"
	"github.com/oswaldom-code/gtool/pkg/config"
)

func sampleRun() (*config.StressConfig, *stress.Plan, []stress.StageMetrics) {
	sc := &config.StressConfig{
		Ramp:             config.StressRamp{StartRPS: 10, StepRPS: 10, StepDuration: 20 * time.Second, MaxRPS: 40},
		Thresholds:       config.StressThresholds{DegradationP95Ms: 800},
		MinAcceptableRPS: 30,
	}
	plan := &stress.Plan{
		Targets:  []stress.Target{{Name: "listUsers", Method: "GET", Path: "/users", Weight: 2, Source: "openapi"}},
		Warnings: []string{"plan warning"},
	}
	stages := []stress.StageMetrics{
		healthy(10, 10),
		healthy(20, 25),
		withErrors(healthy(30, 40), 0.3),
	}
	stages[0].P50, stages[0].P99 = 5*time.Millisecond, 20*time.Millisecond
	stages[0].Requests = 200
	stages[0].StatusCodes = map[string]int{"200": 200}
	stages[2].Errors = []string{"500 Internal Server Error"}
	return sc, plan, stages
}

var testTool = stress.ToolInfo{
	Name: "gtool", Version: "v1.2.3", Commit: "abc1234", BuildDate: "2026-10-31", Repository: "https://github.com/oswaldom-code/gtool",
}

func runInfo(url string) stress.RunInfo {
	return stress.RunInfo{TargetURL: url, Mode: stress.ModePipeline, StartedAt: time.Now().Add(-90 * time.Second), Tool: testTool}
}

var _ = Describe("Report", func() {
	var report *stress.Report

	BeforeEach(func() {
		sc, plan, stages := sampleRun()
		report = stress.NewReport(runInfo("http://localhost:8080"), sc, plan, stages, stress.Detect(stages, sc.Thresholds))
	})

	It("assembles points, gate verdict, effective thresholds and warnings", func() {
		Expect(report.SchemaVersion).To(Equal(stress.ReportSchemaVersion))
		Expect(report.Generator).To(Equal(testTool))
		Expect(report.Mode).To(Equal("pipeline"))
		Expect(report.DurationMs).To(BeNumerically("~", 90000, 1000))
		Expect(report.GeneratedAt).To(BeTemporally(">", report.StartedAt))
		Expect(report.Environment.OS).To(Equal(runtime.GOOS))
		Expect(report.Environment.Arch).To(Equal(runtime.GOARCH))
		Expect(report.Environment.CPUs).To(Equal(runtime.NumCPU()))
		Expect(report.Environment.GoVersion).To(Equal(runtime.Version()))
		Expect(report.Environment.Hostname).NotTo(BeEmpty())
		Expect(report.Degradation.RPS).To(Equal(20))
		Expect(report.Breaking.RPS).To(Equal(30))
		Expect(report.MaxSustainedRPS).To(Equal(20))
		Expect(report.Passed).To(BeFalse())
		Expect(report.Thresholds).To(Equal(stress.ThresholdsReport{
			DegradationLatencyFactor: 2, DegradationP95Ms: 800, BreakErrorRate: 0.05}))
		Expect(report.Ramp.StepDuration).To(Equal("20s"))
		Expect(report.Warnings).To(Equal([]string{"plan warning"}))
		Expect(report.Targets).To(Equal([]stress.TargetReport{
			{Name: "listUsers", Method: "GET", Path: "/users", Weight: 2, Source: "openapi"}}))
	})

	It("writes JSON with a stable schema", func() {
		var buf bytes.Buffer
		Expect(report.WriteJSON(&buf)).To(Succeed())

		var doc map[string]any
		Expect(json.Unmarshal(buf.Bytes(), &doc)).To(Succeed())
		Expect(doc).To(HaveKeyWithValue("schema-version", BeNumerically("==", 1)))
		Expect(doc).To(HaveKey("generated-at"))
		Expect(doc).To(HaveKey("started-at"))
		Expect(doc).To(HaveKey("duration-ms"))
		Expect(doc).To(HaveKeyWithValue("mode", "pipeline"))
		Expect(doc["generator"]).To(Equal(map[string]any{
			"name": "gtool", "version": "v1.2.3", "commit": "abc1234",
			"build-date": "2026-10-31", "repository": "https://github.com/oswaldom-code/gtool"}))
		Expect(doc["environment"]).To(HaveKey("hostname"))
		Expect(doc["environment"]).To(HaveKey("os"))
		Expect(doc["environment"]).To(HaveKey("arch"))
		Expect(doc["environment"]).To(HaveKey("cpus"))
		Expect(doc["environment"]).To(HaveKey("go-version"))
		Expect(doc).To(HaveKeyWithValue("target-url", "http://localhost:8080"))
		Expect(doc).To(HaveKeyWithValue("max-sustained-rps", BeNumerically("==", 20)))
		Expect(doc).To(HaveKeyWithValue("passed", false))
		Expect(doc).To(HaveKeyWithValue("min-acceptable-rps", BeNumerically("==", 30)))
		Expect(doc["breaking-point"]).To(HaveKeyWithValue("rps", BeNumerically("==", 30)))
		Expect(doc["degradation-point"]).To(HaveKeyWithValue("stage", BeNumerically("==", 1)))

		stages := doc["stages"].([]any)
		Expect(stages).To(HaveLen(3))
		first := stages[0].(map[string]any)
		for _, key := range []string{"requested-rps", "duration-ms", "requests", "achieved-rps", "throughput-rps",
			"latency-ms", "error-rate", "client-error-rate", "server-errors", "network-errors",
			"client-errors", "status-codes", "errors"} {
			Expect(first).To(HaveKey(key))
		}
		Expect(first["latency-ms"]).To(Equal(map[string]any{
			"mean": 0.0, "p50": 5.0, "p95": 10.0, "p99": 20.0, "max": 0.0}))
		Expect(first["errors"]).To(Equal([]any{}))
	})

	It("serializes unreached points as null", func() {
		sc, plan, stages := sampleRun()
		stages = stages[:1]
		r := stress.NewReport(runInfo("http://x"), sc, plan, stages, stress.Detect(stages, sc.Thresholds))
		var buf bytes.Buffer
		Expect(r.WriteJSON(&buf)).To(Succeed())
		Expect(buf.String()).To(ContainSubstring(`"breaking-point": null`))
		Expect(buf.String()).To(ContainSubstring(`"degradation-point": null`))
	})

	It("renders a self-contained HTML report with both points marked", func() {
		var buf bytes.Buffer
		Expect(report.WriteHTML(&buf)).To(Succeed())
		html := buf.String()
		Expect(html).To(HavePrefix("<!doctype html>"))
		Expect(html).NotTo(ContainSubstring("<script"))
		Expect(html).NotTo(MatchRegexp(`src="https?://`))
		Expect(html).NotTo(ContainSubstring("<link"))
		Expect(html).To(ContainSubstring(`Generated by <a href="https://github.com/oswaldom-code/gtool">gtool</a> v1.2.3 (commit <code>abc1234</code>)`))
		Expect(html).To(ContainSubstring("built 2026-10-31"))
		Expect(html).To(MatchRegexp(`<dt>Duration</dt><dd>1m3\ds</dd>`))
		Expect(html).To(ContainSubstring("<dt>Mode</dt><dd>pipeline</dd>"))
		Expect(html).To(ContainSubstring("<svg"))
		Expect(html).To(ContainSubstring(`<polyline class="p95"`))
		Expect(html).To(ContainSubstring(`<polyline class="errors"`))
		Expect(html).To(ContainSubstring(`class="marker degradation"`))
		Expect(html).To(ContainSubstring(`class="marker breaking"`))
		Expect(html).To(MatchRegexp(`<text class="breaking" x="704" dx="-4" y="40" text-anchor="end">breaking</text>`))
		Expect(html).To(MatchRegexp(`<text class="degradation" x="380" dx="-?4" y="26" text-anchor="(start|end)">degradation</text>`))
		Expect(html).To(ContainSubstring("p95 SLO"))
		Expect(html).To(ContainSubstring("FAIL"))
		Expect(html).To(ContainSubstring("listUsers"))
		Expect(html).To(ContainSubstring("plan warning"))
	})

	It("renders a single-stage run without points", func() {
		sc, plan, stages := sampleRun()
		sc.MinAcceptableRPS = 0
		stages = stages[:1]
		r := stress.NewReport(runInfo("http://x"), sc, plan, stages, stress.Detect(stages, sc.Thresholds))
		var buf bytes.Buffer
		Expect(r.WriteHTML(&buf)).To(Succeed())
		Expect(buf.String()).To(ContainSubstring("not reached"))
		Expect(buf.String()).NotTo(ContainSubstring(`class="marker`))
	})

	It("renders the tool name without a link when the repository is unknown", func() {
		sc, plan, stages := sampleRun()
		run := runInfo("http://x")
		run.Tool = stress.ToolInfo{Name: "gtool", Version: "dev"}
		r := stress.NewReport(run, sc, plan, stages, stress.Detect(stages, sc.Thresholds))
		var buf bytes.Buffer
		Expect(r.WriteHTML(&buf)).To(Succeed())
		Expect(buf.String()).To(ContainSubstring("Generated by gtool dev</div>"))
		Expect(buf.String()).NotTo(ContainSubstring("<dt>Repository</dt>"))
	})

	It("prints a terminal summary with the gate verdict", func() {
		var buf bytes.Buffer
		Expect(report.WriteSummary(&buf)).To(Succeed())
		out := buf.String()
		Expect(out).To(ContainSubstring("Stress test against http://localhost:8080 (1 targets)"))
		Expect(out).To(MatchRegexp(`gtool v1\.2\.3 \(commit abc1234\) on .+, ` + runtime.GOOS + "/" + runtime.GOARCH))
		Expect(out).To(MatchRegexp(`RPS\s+ACHIEVED\s+THROUGHPUT\s+P50 ms\s+P95 ms\s+P99 ms\s+ERRORS\s+4XX`))
		Expect(out).To(ContainSubstring("30.00%"))
		Expect(out).To(ContainSubstring("Degradation point: 20 rps — p95 25.0ms > 2.0x baseline (10.0ms)"))
		Expect(out).To(ContainSubstring("Breaking point:    30 rps — error rate 30.0%"))
		Expect(out).To(ContainSubstring("Max sustained:     20 rps (min acceptable 30 rps: FAIL)"))
		Expect(out).To(ContainSubstring("Warning: plan warning"))
	})

	It("prints PASS and unreached points", func() {
		sc, plan, stages := sampleRun()
		sc.MinAcceptableRPS = 10
		stages = stages[:1]
		r := stress.NewReport(runInfo("http://x"), sc, plan, stages, stress.Detect(stages, sc.Thresholds))
		var buf bytes.Buffer
		Expect(r.WriteSummary(&buf)).To(Succeed())
		Expect(buf.String()).To(ContainSubstring("Breaking point:    not reached"))
		Expect(buf.String()).To(ContainSubstring("(min acceptable 10 rps: PASS)"))
	})

	It("writes both report files into the reports dir", func() {
		dir := filepath.Join(GinkgoT().TempDir(), "nested", "reports")
		jsonPath, htmlPath, err := report.WriteFiles(dir)
		Expect(err).NotTo(HaveOccurred())
		Expect(jsonPath).To(Equal(filepath.Join(dir, "stress-report.json")))
		Expect(htmlPath).To(Equal(filepath.Join(dir, "stress-report.html")))
		Expect(jsonPath).To(BeAnExistingFile())
		Expect(htmlPath).To(BeAnExistingFile())
	})

	It("fails when the reports dir cannot be created", func() {
		file := filepath.Join(GinkgoT().TempDir(), "file")
		Expect(os.WriteFile(file, nil, 0o644)).To(Succeed())
		_, _, err := report.WriteFiles(filepath.Join(file, "reports"))
		Expect(err).To(HaveOccurred())
	})
})
