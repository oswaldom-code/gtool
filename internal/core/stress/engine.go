package stress

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	vegeta "github.com/tsenart/vegeta/v12/lib"
	"go.uber.org/zap"

	"github.com/oswaldom-code/gtool/pkg/config"
)

const (
	requestTimeout  = 30 * time.Second
	maxIdlePerHost  = 10000
	maxErrorSamples = 5
)

// StageMetrics is the outcome of one ramp stage. Network errors (no HTTP
// response: timeouts, refused connections) and 5xx count towards ErrorRate;
// 4xx are tracked apart so badly resolved targets do not fake a breaking point.
type StageMetrics struct {
	RequestedRPS    int
	Duration        time.Duration
	Requests        uint64
	AchievedRPS     float64
	Throughput      float64
	P50             time.Duration
	P95             time.Duration
	P99             time.Duration
	Max             time.Duration
	Mean            time.Duration
	ServerErrors    int
	NetworkErrors   int
	ClientErrors    int
	ErrorRate       float64
	ClientErrorRate float64
	StatusCodes     map[string]int
	Errors          []string
}

// Stages expands a ramp into the requested RPS of each stage. The last stage
// is always max-rps, even when the step does not land on it.
func Stages(r config.StressRamp) []int {
	var stages []int
	for rps := r.StartRPS; rps <= r.MaxRPS; rps += r.StepRPS {
		stages = append(stages, rps)
		if r.StepRPS <= 0 {
			break
		}
	}
	if len(stages) > 0 && stages[len(stages)-1] < r.MaxRPS {
		stages = append(stages, r.MaxRPS)
	}
	return stages
}

// weightedSchedule repeats each target index by its weight, so a round-robin
// over it distributes the load proportionally.
func weightedSchedule(targets []Target) []int {
	var schedule []int
	for i, t := range targets {
		for w := 0; w < weightOrDefault(t.Weight); w++ {
			schedule = append(schedule, i)
		}
	}
	return schedule
}

// newTargeter builds a concurrency-safe vegeta targeter that walks the
// weighted schedule and rotates each target's URL variants.
func newTargeter(baseURL string, targets []Target) vegeta.Targeter {
	baseURL = strings.TrimSuffix(baseURL, "/")
	schedule := weightedSchedule(targets)
	variants := make([]atomic.Uint64, len(targets))
	var next atomic.Uint64

	return func(tgt *vegeta.Target) error {
		idx := schedule[(next.Add(1)-1)%uint64(len(schedule))]
		t := targets[idx]
		v := variants[idx].Add(1) - 1
		tgt.Method = t.Method
		tgt.URL = baseURL + t.URLs[v%uint64(len(t.URLs))]
		tgt.Body = t.Body
		tgt.Header = t.Header
		return nil
	}
}

// classify tallies a result into the stage counters.
func (s *StageMetrics) classify(res *vegeta.Result) {
	switch {
	case res.Code == 0:
		s.NetworkErrors++
	case res.Code >= 500:
		s.ServerErrors++
	case res.Code >= 400:
		s.ClientErrors++
	}
}

// buildStageMetrics derives the stage view from vegeta's aggregated metrics
// and the gtool error classification.
func buildStageMetrics(rps int, m *vegeta.Metrics, counts StageMetrics) StageMetrics {
	counts.RequestedRPS = rps
	counts.Requests = m.Requests
	counts.Duration = m.Duration + m.Wait
	counts.AchievedRPS = m.Rate
	counts.P50 = m.Latencies.P50
	counts.P95 = m.Latencies.P95
	counts.P99 = m.Latencies.P99
	counts.Max = m.Latencies.Max
	counts.Mean = m.Latencies.Mean
	counts.StatusCodes = m.StatusCodes
	if len(m.Errors) > maxErrorSamples {
		counts.Errors = m.Errors[:maxErrorSamples]
	} else {
		counts.Errors = m.Errors
	}
	if m.Requests > 0 {
		failed := counts.ServerErrors + counts.NetworkErrors
		counts.ErrorRate = float64(failed) / float64(m.Requests)
		counts.ClientErrorRate = float64(counts.ClientErrors) / float64(m.Requests)
		if secs := counts.Duration.Seconds(); secs > 0 {
			counts.Throughput = float64(int(m.Requests)-failed) / secs
		}
	}
	return counts
}

// Engine generates HTTP load in-process with vegeta.
type Engine struct {
	client *http.Client
	logger *zap.Logger
}

// NewEngine creates an engine whose HTTP client (keep-alive, high idle
// connection limit) is shared by every stage, so connections survive between
// stages.
func NewEngine(logger *zap.Logger) *Engine {
	if logger == nil {
		logger = zap.NewNop()
	}
	transport := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: requestTimeout, KeepAlive: requestTimeout}).DialContext,
		MaxIdleConnsPerHost: maxIdlePerHost,
		IdleConnTimeout:     90 * time.Second,
	}
	return &Engine{
		client: &http.Client{Timeout: requestTimeout, Transport: transport},
		logger: logger,
	}
}

// RunStage attacks at a constant rate for the given duration. A cancelled
// context stops the attack; the metrics collected so far are returned with
// the context error.
func (e *Engine) RunStage(ctx context.Context, tr vegeta.Targeter, rps int, d time.Duration) (StageMetrics, error) {
	// A vegeta attacker stops itself when an attack ends, so each stage needs
	// a fresh one; the shared client keeps the connections.
	attacker := vegeta.NewAttacker(vegeta.Client(e.client))
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			attacker.Stop()
		case <-done:
		}
	}()

	var m vegeta.Metrics
	var counts StageMetrics
	pacer := vegeta.Rate{Freq: rps, Per: time.Second}
	for res := range attacker.Attack(tr, pacer, d, "stage-"+strconv.Itoa(rps)) {
		m.Add(res)
		counts.classify(res)
	}
	m.Close()

	return buildStageMetrics(rps, &m, counts), ctx.Err()
}

// Ramp runs the stages in order and stops early once shouldStop reports the
// breaking point. It returns the stages that completed.
func (e *Engine) Ramp(ctx context.Context, baseURL string, targets []Target, stages []int,
	stageDuration time.Duration, shouldStop func(StageMetrics) bool) ([]StageMetrics, error) {
	tr := newTargeter(baseURL, targets)
	results := make([]StageMetrics, 0, len(stages))
	for _, rps := range stages {
		e.logger.Info("stress stage started", zap.Int("rps", rps), zap.Duration("duration", stageDuration))
		stage, err := e.RunStage(ctx, tr, rps, stageDuration)
		if err != nil {
			return results, err
		}
		results = append(results, stage)
		e.logger.Info("stress stage finished",
			zap.Int("rps", rps),
			zap.Float64("achieved-rps", stage.AchievedRPS),
			zap.Duration("p95", stage.P95),
			zap.Float64("error-rate", stage.ErrorRate),
		)
		if shouldStop != nil && shouldStop(stage) {
			e.logger.Info("breaking point reached, stopping ramp", zap.Int("rps", rps))
			break
		}
	}
	return results, nil
}
