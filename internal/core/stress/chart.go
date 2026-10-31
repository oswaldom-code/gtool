package stress

import (
	"fmt"
	"math"
	"strings"
)

// Chart geometry of the HTML report, in SVG user units.
const (
	chartWidth   = 720.0
	chartHeight  = 260.0
	chartLeft    = 56.0
	chartRight   = 704.0
	chartTop     = 16.0
	chartBottom  = 224.0
	chartYTicks  = 4
	maxXTickRuns = 12
)

type chartPoint struct {
	X, Y float64
}

type chartSeries struct {
	Name   string
	Class  string
	Points string
	Dots   []chartPoint
}

type chartTick struct {
	Pos   float64
	Label string
}

type chartMarker struct {
	X      float64
	LabelY float64
	// LabelDX/Anchor keep the label inside the plot on the right half.
	LabelDX float64
	Anchor  string
	Label   string
	Class   string
}

// chart is a pre-computed line chart; the template only draws it.
type chart struct {
	Width, Height            float64
	Left, Right, Top, Bottom float64
	YLabel                   string
	Series                   []chartSeries
	XTicks                   []chartTick
	YTicks                   []chartTick
	Markers                  []chartMarker
	Threshold                *chartTick
}

// htmlView is the HTML template input.
type htmlView struct {
	*Report
	LatencyChart  chart
	ErrorChart    chart
	GeneratedText string
}

// niceMax rounds v up to 1, 2 or 5 times a power of ten.
func niceMax(v float64) float64 {
	if v <= 0 {
		return 1
	}
	exp := math.Pow(10, math.Floor(math.Log10(v)))
	for _, m := range []float64{1, 2, 5, 10} {
		if v <= m*exp {
			return m * exp
		}
	}
	return 10 * exp
}

// newChart sets up the axes for the stage RPS values and the y range.
func newChart(rps []int, yMax float64, yLabel string) (chart, func(int) float64, func(float64) float64) {
	c := chart{
		Width: chartWidth, Height: chartHeight,
		Left: chartLeft, Right: chartRight, Top: chartTop, Bottom: chartBottom,
		YLabel: yLabel,
	}
	minX, maxX := 0, 1
	if len(rps) > 0 {
		minX, maxX = rps[0], rps[len(rps)-1]
	}
	x := func(v int) float64 {
		if maxX == minX {
			return (chartLeft + chartRight) / 2
		}
		return chartLeft + float64(v-minX)/float64(maxX-minX)*(chartRight-chartLeft)
	}
	y := func(v float64) float64 {
		return chartBottom - v/yMax*(chartBottom-chartTop)
	}

	step := 1
	if len(rps) > maxXTickRuns {
		step = (len(rps) + maxXTickRuns - 1) / maxXTickRuns
	}
	for i := 0; i < len(rps); i += step {
		c.XTicks = append(c.XTicks, chartTick{Pos: x(rps[i]), Label: fmt.Sprint(rps[i])})
	}
	for i := 0; i <= chartYTicks; i++ {
		v := yMax * float64(i) / chartYTicks
		c.YTicks = append(c.YTicks, chartTick{Pos: y(v), Label: trimFloat(v)})
	}
	return c, x, y
}

func trimFloat(v float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}

func series(name, class string, rps []int, values []float64, x func(int) float64, y func(float64) float64) chartSeries {
	s := chartSeries{Name: name, Class: class}
	pts := make([]string, 0, len(values))
	for i, v := range values {
		p := chartPoint{X: x(rps[i]), Y: y(v)}
		s.Dots = append(s.Dots, p)
		pts = append(pts, fmt.Sprintf("%.1f,%.1f", p.X, p.Y))
	}
	s.Points = strings.Join(pts, " ")
	return s
}

func marker(xPos, labelY float64, label string) chartMarker {
	m := chartMarker{X: xPos, LabelY: labelY, LabelDX: 4, Anchor: "start", Label: label, Class: label}
	if xPos > (chartLeft+chartRight)/2 {
		m.LabelDX, m.Anchor = -4, "end"
	}
	return m
}

// addMarkers draws the degradation and breaking points as vertical lines.
func addMarkers(c *chart, r *Report, x func(int) float64) {
	if r.Degradation != nil {
		c.Markers = append(c.Markers, marker(x(r.Degradation.RPS), chartTop+10, "degradation"))
	}
	if r.Breaking != nil {
		c.Markers = append(c.Markers, marker(x(r.Breaking.RPS), chartTop+24, "breaking"))
	}
}

func stageRPS(r *Report) []int {
	rps := make([]int, len(r.Stages))
	for i, s := range r.Stages {
		rps[i] = s.RequestedRPS
	}
	return rps
}

// latencyChart plots p50/p95/p99 against the requested RPS.
func latencyChart(r *Report) chart {
	rps := stageRPS(r)
	p50 := make([]float64, len(r.Stages))
	p95 := make([]float64, len(r.Stages))
	p99 := make([]float64, len(r.Stages))
	top := float64(r.Thresholds.DegradationP95Ms)
	for i, s := range r.Stages {
		p50[i], p95[i], p99[i] = s.LatencyMs.P50, s.LatencyMs.P95, s.LatencyMs.P99
		top = math.Max(top, s.LatencyMs.P99)
	}
	c, x, y := newChart(rps, niceMax(top*1.1), "latency (ms)")
	c.Series = []chartSeries{
		series("p50", "p50", rps, p50, x, y),
		series("p95", "p95", rps, p95, x, y),
		series("p99", "p99", rps, p99, x, y),
	}
	if r.Thresholds.DegradationP95Ms > 0 {
		c.Threshold = &chartTick{Pos: y(float64(r.Thresholds.DegradationP95Ms)), Label: "p95 SLO"}
	}
	addMarkers(&c, r, x)
	return c
}

// errorChart plots the error rate and the 4xx rate (percent) against the
// requested RPS, with the breaking threshold.
func errorChart(r *Report) chart {
	rps := stageRPS(r)
	errs := make([]float64, len(r.Stages))
	client := make([]float64, len(r.Stages))
	top := r.Thresholds.BreakErrorRate * 100
	for i, s := range r.Stages {
		errs[i], client[i] = s.ErrorRate*100, s.ClientErrorRate*100
		top = math.Max(top, math.Max(errs[i], client[i]))
	}
	c, x, y := newChart(rps, math.Min(niceMax(top*1.1), 100), "rate (%)")
	c.Series = []chartSeries{
		series("errors (5xx + network)", "errors", rps, errs, x, y),
		series("4xx", "client", rps, client, x, y),
	}
	c.Threshold = &chartTick{Pos: y(r.Thresholds.BreakErrorRate * 100), Label: "break threshold"}
	addMarkers(&c, r, x)
	return c
}
