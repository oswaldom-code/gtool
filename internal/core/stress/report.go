package stress

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

const (
	// ReportSchemaVersion is bumped on any breaking change to the JSON report.
	ReportSchemaVersion = 1

	jsonReportFile = "stress-report.json"
	htmlReportFile = "stress-report.html"

	// ModePipeline means gtool started the mocks and the app; ModeTargetURL
	// means the load hit an external --target-url.
	ModePipeline  = "pipeline"
	ModeTargetURL = "target-url"
)

//go:embed report.html.tmpl
var htmlTemplate string

// ToolInfo identifies the gtool build that produced a report.
type ToolInfo struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	BuildDate  string `json:"build-date"`
	Repository string `json:"repository"`
}

// EnvironmentInfo describes the host that generated the load; results are
// relative to it.
type EnvironmentInfo struct {
	Hostname  string `json:"hostname"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	CPUs      int    `json:"cpus"`
	GoVersion string `json:"go-version"`
}

// RunInfo is the run context recorded in the report.
type RunInfo struct {
	TargetURL string
	Mode      string
	StartedAt time.Time
	Tool      ToolInfo
}

func currentEnvironment() EnvironmentInfo {
	hostname, _ := os.Hostname()
	return EnvironmentInfo{
		Hostname:  hostname,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		CPUs:      runtime.NumCPU(),
		GoVersion: runtime.Version(),
	}
}

// Report is the stable, serialized outcome of a stress run.
type Report struct {
	SchemaVersion    int              `json:"schema-version"`
	Generator        ToolInfo         `json:"generator"`
	Environment      EnvironmentInfo  `json:"environment"`
	Mode             string           `json:"mode"`
	StartedAt        time.Time        `json:"started-at"`
	GeneratedAt      time.Time        `json:"generated-at"`
	DurationMs       float64          `json:"duration-ms"`
	TargetURL        string           `json:"target-url"`
	Ramp             RampReport       `json:"ramp"`
	Thresholds       ThresholdsReport `json:"thresholds"`
	MinAcceptableRPS int              `json:"min-acceptable-rps"`
	Targets          []TargetReport   `json:"targets"`
	Stages           []StageReport    `json:"stages"`
	Degradation      *PointReport     `json:"degradation-point"`
	Breaking         *PointReport     `json:"breaking-point"`
	MaxSustainedRPS  int              `json:"max-sustained-rps"`
	Passed           bool             `json:"passed"`
	Warnings         []string         `json:"warnings"`
}

// RampReport mirrors the configured ramp.
type RampReport struct {
	StartRPS     int    `json:"start-rps"`
	StepRPS      int    `json:"step-rps"`
	StepDuration string `json:"step-duration"`
	MaxRPS       int    `json:"max-rps"`
}

// ThresholdsReport holds the effective thresholds (defaults applied).
type ThresholdsReport struct {
	DegradationLatencyFactor float64 `json:"degradation-latency-factor"`
	DegradationP95Ms         int     `json:"degradation-p95-ms"`
	BreakErrorRate           float64 `json:"break-error-rate"`
}

// TargetReport describes one target of the run.
type TargetReport struct {
	Name   string `json:"name"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Weight int    `json:"weight"`
	Source string `json:"source"`
}

// LatencyReport holds latencies in milliseconds.
type LatencyReport struct {
	Mean float64 `json:"mean"`
	P50  float64 `json:"p50"`
	P95  float64 `json:"p95"`
	P99  float64 `json:"p99"`
	Max  float64 `json:"max"`
}

// StageReport is the serialized form of StageMetrics.
type StageReport struct {
	RequestedRPS    int            `json:"requested-rps"`
	DurationMs      float64        `json:"duration-ms"`
	Requests        uint64         `json:"requests"`
	AchievedRPS     float64        `json:"achieved-rps"`
	ThroughputRPS   float64        `json:"throughput-rps"`
	LatencyMs       LatencyReport  `json:"latency-ms"`
	ErrorRate       float64        `json:"error-rate"`
	ClientErrorRate float64        `json:"client-error-rate"`
	ServerErrors    int            `json:"server-errors"`
	NetworkErrors   int            `json:"network-errors"`
	ClientErrors    int            `json:"client-errors"`
	StatusCodes     map[string]int `json:"status-codes"`
	Errors          []string       `json:"errors"`
}

// PointReport is a degradation or breaking point.
type PointReport struct {
	RPS    int    `json:"rps"`
	Stage  int    `json:"stage"`
	Reason string `json:"reason"`
}

func pointReport(p *Point) *PointReport {
	if p == nil {
		return nil
	}
	return &PointReport{RPS: p.RPS, Stage: p.Stage, Reason: p.Reason}
}

func stageReport(s StageMetrics) StageReport {
	codes := s.StatusCodes
	if codes == nil {
		codes = map[string]int{}
	}
	errs := s.Errors
	if errs == nil {
		errs = []string{}
	}
	return StageReport{
		RequestedRPS:  s.RequestedRPS,
		DurationMs:    ms(s.Duration),
		Requests:      s.Requests,
		AchievedRPS:   s.AchievedRPS,
		ThroughputRPS: s.Throughput,
		LatencyMs: LatencyReport{
			Mean: ms(s.Mean), P50: ms(s.P50), P95: ms(s.P95), P99: ms(s.P99), Max: ms(s.Max),
		},
		ErrorRate:       s.ErrorRate,
		ClientErrorRate: s.ClientErrorRate,
		ServerErrors:    s.ServerErrors,
		NetworkErrors:   s.NetworkErrors,
		ClientErrors:    s.ClientErrors,
		StatusCodes:     codes,
		Errors:          errs,
	}
}

// NewReport assembles the report of a run.
func NewReport(run RunInfo, sc *config.StressConfig, plan *Plan, stages []StageMetrics, a Analysis) *Report {
	th := WithDefaults(sc.Thresholds)
	now := time.Now().UTC()
	r := &Report{
		SchemaVersion: ReportSchemaVersion,
		Generator:     run.Tool,
		Environment:   currentEnvironment(),
		Mode:          run.Mode,
		StartedAt:     run.StartedAt.UTC(),
		GeneratedAt:   now,
		DurationMs:    ms(now.Sub(run.StartedAt)),
		TargetURL:     run.TargetURL,
		Ramp: RampReport{
			StartRPS:     sc.Ramp.StartRPS,
			StepRPS:      sc.Ramp.StepRPS,
			StepDuration: sc.Ramp.StepDuration.String(),
			MaxRPS:       sc.Ramp.MaxRPS,
		},
		Thresholds: ThresholdsReport{
			DegradationLatencyFactor: th.DegradationLatencyFactor,
			DegradationP95Ms:         th.DegradationP95Ms,
			BreakErrorRate:           th.BreakErrorRate,
		},
		MinAcceptableRPS: sc.MinAcceptableRPS,
		Targets:          make([]TargetReport, 0, len(plan.Targets)),
		Stages:           make([]StageReport, 0, len(stages)),
		Degradation:      pointReport(a.Degradation),
		Breaking:         pointReport(a.Breaking),
		MaxSustainedRPS:  a.MaxSustainedRPS,
		Passed:           a.MeetsMinimum(sc.MinAcceptableRPS),
		Warnings:         append(append([]string{}, plan.Warnings...), a.Warnings...),
	}
	for _, t := range plan.Targets {
		r.Targets = append(r.Targets, TargetReport{Name: t.Name, Method: t.Method, Path: t.Path, Weight: t.Weight, Source: t.Source})
	}
	for _, s := range stages {
		r.Stages = append(r.Stages, stageReport(s))
	}
	return r
}

// WriteJSON writes the report as indented JSON.
func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteHTML renders the self-contained HTML report.
func (r *Report) WriteHTML(w io.Writer) error {
	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"pct": func(v float64) string { return fmt.Sprintf("%.2f%%", v*100) },
		"f1":  func(v float64) string { return fmt.Sprintf("%.1f", v) },
	}).Parse(htmlTemplate)
	if err != nil {
		return err
	}
	return tmpl.Execute(w, htmlView{
		Report:        r,
		LatencyChart:  latencyChart(r),
		ErrorChart:    errorChart(r),
		GeneratedText: r.GeneratedAt.Format(time.RFC1123),
		StartedText:   r.StartedAt.Format(time.RFC1123),
		DurationText:  time.Duration(r.DurationMs * float64(time.Millisecond)).Round(time.Second).String(),
	})
}

func pointText(p *PointReport) string {
	if p == nil {
		return "not reached"
	}
	return fmt.Sprintf("%d rps — %s", p.RPS, p.Reason)
}

// WriteSummary prints the per-stage table, both points and the gate verdict.
func (r *Report) WriteSummary(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Stress test against %s (%d targets)\n", r.TargetURL, len(r.Targets))
	fmt.Fprintf(&b, "%s %s (commit %s) on %s, %s/%s, %d CPUs\n\n", r.Generator.Name, r.Generator.Version,
		r.Generator.Commit, r.Environment.Hostname, r.Environment.OS, r.Environment.Arch, r.Environment.CPUs)

	// The tabwriter wraps a strings.Builder: writes cannot fail and Flush
	// reports any formatting error.
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', tabwriter.AlignRight)
	_, _ = fmt.Fprintln(tw, "RPS\tACHIEVED\tTHROUGHPUT\tP50 ms\tP95 ms\tP99 ms\tERRORS\t4XX\t")
	for _, s := range r.Stages {
		_, _ = fmt.Fprintf(tw, "%d\t%.1f\t%.1f\t%.1f\t%.1f\t%.1f\t%.2f%%\t%.2f%%\t\n",
			s.RequestedRPS, s.AchievedRPS, s.ThroughputRPS, s.LatencyMs.P50, s.LatencyMs.P95, s.LatencyMs.P99,
			s.ErrorRate*100, s.ClientErrorRate*100)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	fmt.Fprintf(&b, "\nDegradation point: %s\n", pointText(r.Degradation))
	fmt.Fprintf(&b, "Breaking point:    %s\n", pointText(r.Breaking))
	fmt.Fprintf(&b, "Max sustained:     %d rps", r.MaxSustainedRPS)
	if r.MinAcceptableRPS > 0 {
		verdict := "PASS"
		if !r.Passed {
			verdict = "FAIL"
		}
		fmt.Fprintf(&b, " (min acceptable %d rps: %s)", r.MinAcceptableRPS, verdict)
	}
	b.WriteString("\n")
	for _, warn := range r.Warnings {
		fmt.Fprintf(&b, "Warning: %s\n", warn)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeFile(path string, render func(io.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := render(f); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// WriteFiles writes stress-report.json and stress-report.html into dir and
// returns their paths.
func (r *Report) WriteFiles(dir string) (string, string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, fmt.Sprintf("failed to create reports dir %s", dir))
	}
	jsonPath := filepath.Join(dir, jsonReportFile)
	htmlPath := filepath.Join(dir, htmlReportFile)
	if err := writeFile(jsonPath, r.WriteJSON); err != nil {
		return "", "", gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to write JSON report")
	}
	if err := writeFile(htmlPath, r.WriteHTML); err != nil {
		return "", "", gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to write HTML report")
	}
	return jsonPath, htmlPath, nil
}
