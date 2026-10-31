package stress_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oswaldom-code/gtool/internal/core/stress"
	"github.com/oswaldom-code/gtool/pkg/config"
)

// healthy builds a stage that meets its requested rate with the given p95.
func healthy(rps int, p95Ms int) stress.StageMetrics {
	return stress.StageMetrics{
		RequestedRPS: rps,
		Throughput:   float64(rps),
		P95:          time.Duration(p95Ms) * time.Millisecond,
	}
}

func withErrors(s stress.StageMetrics, rate float64) stress.StageMetrics {
	s.ErrorRate = rate
	return s
}

func withThroughput(s stress.StageMetrics, tp float64) stress.StageMetrics {
	s.Throughput = tp
	return s
}

func pointRPS(p *stress.Point) int {
	if p == nil {
		return -1
	}
	return p.RPS
}

var _ = Describe("Detect", func() {
	DescribeTable("finds the degradation and breaking points",
		func(stages []stress.StageMetrics, th config.StressThresholds, degradation, breaking, sustained int, reason string) {
			a := stress.Detect(stages, th)
			Expect(pointRPS(a.Degradation)).To(Equal(degradation))
			Expect(pointRPS(a.Breaking)).To(Equal(breaking))
			Expect(a.MaxSustainedRPS).To(Equal(sustained))
			if reason != "" {
				Expect(a.Degradation.Reason).To(ContainSubstring(reason))
			}
		},
		Entry("neither point reached",
			[]stress.StageMetrics{healthy(10, 10), healthy(20, 12), healthy(30, 15)},
			config.StressThresholds{}, -1, -1, 30, ""),
		Entry("latency factor over baseline",
			[]stress.StageMetrics{healthy(10, 10), healthy(20, 15), healthy(30, 25), healthy(40, 30)},
			config.StressThresholds{}, 30, -1, 40, "2.0x baseline"),
		Entry("custom latency factor",
			[]stress.StageMetrics{healthy(10, 10), healthy(20, 15), healthy(30, 25)},
			config.StressThresholds{DegradationLatencyFactor: 1.4}, 20, -1, 30, "1.4x baseline"),
		Entry("absolute p95 SLO, including the baseline",
			[]stress.StageMetrics{healthy(10, 900), healthy(20, 950)},
			config.StressThresholds{DegradationP95Ms: 800}, 10, -1, 20, "SLO 800ms"),
		Entry("throughput below 90% of the requested rate",
			[]stress.StageMetrics{healthy(10, 10), withThroughput(healthy(20, 10), 17.9)},
			config.StressThresholds{}, 20, -1, 20, "throughput 17.9 rps"),
		Entry("breaking point after degradation",
			[]stress.StageMetrics{healthy(10, 10), healthy(20, 30), withErrors(healthy(30, 40), 0.2)},
			config.StressThresholds{}, 20, 30, 20, "baseline"),
		Entry("breaking without prior degradation",
			[]stress.StageMetrics{healthy(10, 10), withErrors(healthy(20, 10), 0.06)},
			config.StressThresholds{}, 20, 20, 10, "together with the breaking point"),
		Entry("error rate at the threshold does not break",
			[]stress.StageMetrics{healthy(10, 10), withErrors(healthy(20, 10), 0.05)},
			config.StressThresholds{}, -1, -1, 20, ""),
		Entry("custom break error rate",
			[]stress.StageMetrics{healthy(10, 10), withErrors(healthy(20, 10), 0.05)},
			config.StressThresholds{BreakErrorRate: 0.01}, 20, 20, 10, ""),
		Entry("baseline already broken",
			[]stress.StageMetrics{withErrors(healthy(10, 10), 1)},
			config.StressThresholds{}, 10, 10, 0, ""),
		Entry("no stages",
			[]stress.StageMetrics{},
			config.StressThresholds{}, -1, -1, 0, ""),
	)

	It("uses the first stage as baseline", func() {
		stages := []stress.StageMetrics{healthy(10, 10), healthy(20, 12)}
		a := stress.Detect(stages, config.StressThresholds{})
		Expect(a.Baseline).To(Equal(&stages[0]))
	})

	It("warns when the baseline has many 4xx responses", func() {
		baseline := healthy(10, 10)
		baseline.ClientErrorRate = 0.5
		a := stress.Detect([]stress.StageMetrics{baseline}, config.StressThresholds{})
		Expect(a.Warnings).To(ContainElement(ContainSubstring("50.0% of baseline responses were 4xx")))
	})

	It("records the breaking reason", func() {
		s := withErrors(healthy(10, 10), 0.5)
		s.ServerErrors, s.NetworkErrors = 3, 2
		a := stress.Detect([]stress.StageMetrics{s}, config.StressThresholds{})
		Expect(a.Breaking.Reason).To(Equal("error rate 50.0% > 5.0% (5xx: 3, network: 2)"))
		Expect(a.Breaking.Stage).To(Equal(0))
	})
})

var _ = Describe("BreakingReason", func() {
	It("is empty below the threshold", func() {
		Expect(stress.BreakingReason(withErrors(healthy(10, 10), 0.01), config.StressThresholds{})).To(BeEmpty())
	})
})

var _ = Describe("Analysis.MeetsMinimum", func() {
	DescribeTable("gates on the sustained load",
		func(sustained, minRPS int, expected bool) {
			Expect(stress.Analysis{MaxSustainedRPS: sustained}.MeetsMinimum(minRPS)).To(Equal(expected))
		},
		Entry("no minimum configured", 0, 0, true),
		Entry("sustained above minimum", 120, 100, true),
		Entry("sustained equal to minimum", 100, 100, true),
		Entry("sustained below minimum", 90, 100, false),
	)
})

var _ = Describe("WithDefaults", func() {
	It("fills only zero values", func() {
		Expect(stress.WithDefaults(config.StressThresholds{})).To(Equal(config.StressThresholds{
			DegradationLatencyFactor: 2.0, BreakErrorRate: 0.05}))
		custom := config.StressThresholds{DegradationLatencyFactor: 3, DegradationP95Ms: 500, BreakErrorRate: 0.1}
		Expect(stress.WithDefaults(custom)).To(Equal(custom))
	})
})
