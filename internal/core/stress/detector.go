package stress

import (
	"fmt"
	"time"

	"github.com/oswaldom-code/gtool/pkg/config"
)

const (
	defaultLatencyFactor    = 2.0
	defaultBreakErrorRate   = 0.05
	defaultPlateauTolerance = 0.05
	// throughputFloor is the fraction of the requested rate below which the
	// achieved throughput counts as degraded.
	throughputFloor = 0.9
	// baselineClientErrorWarning is the baseline 4xx rate above which targets
	// are probably badly resolved.
	baselineClientErrorWarning = 0.1
)

// Point marks the first stage where a condition was observed. RPS and Workers
// are the load of that stage (see StageMetrics).
type Point struct {
	RPS     int
	Workers int
	Stage   int
	Reason  string
}

func pointAt(s StageMetrics, stage int, reason string) *Point {
	return &Point{RPS: s.RequestedRPS, Workers: s.Workers, Stage: stage, Reason: reason}
}

// Analysis is the outcome of evaluating the stages against the thresholds.
type Analysis struct {
	Baseline    *StageMetrics
	Degradation *Point
	Breaking    *Point
	// Plateau is the first concurrency stage where adding workers no longer
	// raised the throughput. It is informative: it does not stop the ramp.
	Plateau *Point
	// MaxSustainedRPS is the highest load completed without reaching the
	// breaking point (0 when the first stage already broke): the requested
	// RPS of a rate ramp, the throughput of a concurrency ramp.
	MaxSustainedRPS int
	Warnings        []string
}

// WithDefaults fills the zero-valued thresholds with the defaults.
func WithDefaults(t config.StressThresholds) config.StressThresholds {
	if t.DegradationLatencyFactor == 0 {
		t.DegradationLatencyFactor = defaultLatencyFactor
	}
	if t.BreakErrorRate == 0 {
		t.BreakErrorRate = defaultBreakErrorRate
	}
	if t.ThroughputPlateauTolerance == 0 {
		t.ThroughputPlateauTolerance = defaultPlateauTolerance
	}
	return t
}

func ms(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

// BreakingReason reports why a stage is past the breaking point, or "" when
// it is not.
func BreakingReason(s StageMetrics, t config.StressThresholds) string {
	t = WithDefaults(t)
	if s.ErrorRate > t.BreakErrorRate {
		return fmt.Sprintf("error rate %.1f%% > %.1f%% (5xx: %d, network: %d)",
			s.ErrorRate*100, t.BreakErrorRate*100, s.ServerErrors, s.NetworkErrors)
	}
	return ""
}

// degradationReason reports why a stage is degraded relative to the baseline,
// or "" when it is not.
func degradationReason(s, baseline StageMetrics, t config.StressThresholds) string {
	if baseline.P95 > 0 && float64(s.P95) > t.DegradationLatencyFactor*float64(baseline.P95) {
		return fmt.Sprintf("p95 %.1fms > %.1fx baseline (%.1fms)", ms(s.P95), t.DegradationLatencyFactor, ms(baseline.P95))
	}
	if t.DegradationP95Ms > 0 && ms(s.P95) > float64(t.DegradationP95Ms) {
		return fmt.Sprintf("p95 %.1fms > SLO %dms", ms(s.P95), t.DegradationP95Ms)
	}
	if s.Throughput < throughputFloor*float64(s.RequestedRPS) {
		return fmt.Sprintf("throughput %.1f rps < %.0f%% of requested %d rps", s.Throughput, throughputFloor*100, s.RequestedRPS)
	}
	return ""
}

// plateauReason reports why a concurrency stage did not raise the throughput
// of the previous one, or "" when it did.
func plateauReason(s, prev StageMetrics, t config.StressThresholds) string {
	if s.Workers == 0 || s.Throughput > prev.Throughput*(1+t.ThroughputPlateauTolerance) {
		return ""
	}
	return fmt.Sprintf("throughput %.1f rps is not more than %.0f%% above %.1f rps at %d workers",
		s.Throughput, t.ThroughputPlateauTolerance*100, prev.Throughput, prev.Workers)
}

// sustainedRPS is the load a completed stage proves the target can sustain.
func sustainedRPS(s StageMetrics) int {
	if s.Workers > 0 {
		return int(s.Throughput)
	}
	return s.RequestedRPS
}

// Detect finds the degradation and breaking points. The first stage is the
// baseline. A run that breaks without degrading first reports the breaking
// stage as the degradation point too.
func Detect(stages []StageMetrics, t config.StressThresholds) Analysis {
	t = WithDefaults(t)
	var a Analysis
	if len(stages) == 0 {
		return a
	}
	a.Baseline = &stages[0]
	if a.Baseline.ClientErrorRate > baselineClientErrorWarning {
		a.Warnings = append(a.Warnings, fmt.Sprintf(
			"%.1f%% of baseline responses were 4xx: check the resolved targets with `gtool stress plan`",
			a.Baseline.ClientErrorRate*100))
	}

	for i, s := range stages {
		if a.Degradation == nil {
			if reason := degradationReason(s, *a.Baseline, t); reason != "" {
				a.Degradation = pointAt(s, i, reason)
			}
		}
		if reason := BreakingReason(s, t); reason != "" {
			a.Breaking = pointAt(s, i, reason)
			if a.Degradation == nil {
				a.Degradation = pointAt(s, i, "reached together with the breaking point")
			}
			return a
		}
		if a.Plateau == nil && i > 0 {
			if reason := plateauReason(s, stages[i-1], t); reason != "" {
				a.Plateau = pointAt(s, i, reason)
			}
		}
		a.MaxSustainedRPS = max(a.MaxSustainedRPS, sustainedRPS(s))
	}
	return a
}

// MeetsMinimum reports whether the sustained load reaches min-acceptable-rps
// (always true when no minimum is configured).
func (a Analysis) MeetsMinimum(minRPS int) bool {
	return minRPS <= 0 || a.MaxSustainedRPS >= minRPS
}
