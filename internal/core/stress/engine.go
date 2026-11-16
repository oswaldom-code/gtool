package stress

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	vegeta "github.com/tsenart/vegeta/v12/lib"
	"go.uber.org/zap"

	"github.com/oswaldom-code/gtool/pkg/config"
)

const (
	requestTimeout  = 30 * time.Second
	maxIdlePerHost  = config.MaxStressWorkers
	maxErrorSamples = 5
)

// StageMetrics is the outcome of one ramp stage. Network errors (no HTTP
// response: timeouts, refused connections) and 5xx count towards ErrorRate;
// 4xx are tracked apart so badly resolved targets do not fake a breaking point.
type StageMetrics struct {
	// RequestedRPS is the stage rate; 0 in a concurrency stage.
	RequestedRPS int
	// Workers is the closed-loop concurrency; 0 in a rate stage.
	Workers         int
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

// Load is the load of one stage: a constant request rate (RPS) or a fixed
// number of closed-loop workers (Workers). Exactly one is set.
type Load struct {
	RPS     int
	Workers int
}

func (l Load) String() string {
	if l.Workers > 0 {
		return fmt.Sprintf("%d workers", l.Workers)
	}
	return fmt.Sprintf("%d rps", l.RPS)
}

// steps expands start, start+step, ... up to limit. The last value is always
// limit, even when the step does not land on it.
func steps(start, step, limit int) []int {
	var values []int
	for v := start; v <= limit; v += step {
		values = append(values, v)
		if step <= 0 {
			break
		}
	}
	if len(values) > 0 && values[len(values)-1] < limit {
		values = append(values, limit)
	}
	return values
}

// Stages expands the configured ramp into the load of each stage.
func Stages(sc *config.StressConfig) []Load {
	var loads []Load
	if sc.Concurrent() {
		r := sc.ConcurrencyRamp
		for _, workers := range steps(r.StartWorkers, r.StepWorkers, r.MaxWorkers) {
			loads = append(loads, Load{Workers: workers})
		}
		return loads
	}
	for _, rps := range steps(sc.Ramp.StartRPS, sc.Ramp.StepRPS, sc.Ramp.MaxRPS) {
		loads = append(loads, Load{RPS: rps})
	}
	return loads
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
func buildStageMetrics(load Load, m *vegeta.Metrics, counts StageMetrics) StageMetrics {
	counts.RequestedRPS = load.RPS
	counts.Workers = load.Workers
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

// RunStage attacks with the given load for the given duration. A cancelled
// context stops the attack; the metrics collected so far are returned with
// the context error.
func (e *Engine) RunStage(ctx context.Context, tr vegeta.Targeter, load Load, d time.Duration) (StageMetrics, error) {
	opts := []func(*vegeta.Attacker){vegeta.Client(e.client)}
	pacer := vegeta.Rate{Freq: load.RPS, Per: time.Second}
	if load.Workers > 0 {
		// A zero rate is unbounded, so a fixed worker pool makes the attack
		// closed-loop: each worker sends again as soon as it gets a response.
		workers := uint64(load.Workers)
		opts = append(opts, vegeta.Workers(workers), vegeta.MaxWorkers(workers))
		pacer = vegeta.Rate{}
	}
	// A vegeta attacker stops itself when an attack ends, so each stage needs
	// a fresh one; the shared client keeps the connections.
	attacker := vegeta.NewAttacker(opts...)
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
	for res := range attacker.Attack(tr, pacer, d, "stage-"+load.String()) {
		m.Add(res)
		counts.classify(res)
	}
	m.Close()

	return buildStageMetrics(load, &m, counts), ctx.Err()
}

// Ramp runs the stages in order and stops early once shouldStop reports the
// breaking point. It returns the stages that completed.
func (e *Engine) Ramp(ctx context.Context, baseURL string, targets []Target, stages []Load,
	stageDuration time.Duration, shouldStop func(StageMetrics) bool) ([]StageMetrics, error) {
	tr := newTargeter(baseURL, targets)
	results := make([]StageMetrics, 0, len(stages))
	for _, load := range stages {
		e.logger.Info("stress stage started", zap.Stringer("load", load), zap.Duration("duration", stageDuration))
		stage, err := e.RunStage(ctx, tr, load, stageDuration)
		if err != nil {
			return results, err
		}
		results = append(results, stage)
		e.logger.Info("stress stage finished",
			zap.Stringer("load", load),
			zap.Float64("achieved-rps", stage.AchievedRPS),
			zap.Duration("p95", stage.P95),
			zap.Float64("error-rate", stage.ErrorRate),
		)
		if shouldStop != nil && shouldStop(stage) {
			e.logger.Info("breaking point reached, stopping ramp", zap.Stringer("load", load))
			break
		}
	}
	return results, nil
}
