package stress_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oswaldom-code/gtool/internal/core/stress"
	"github.com/oswaldom-code/gtool/pkg/config"
)

// hitCounter is a test server that records hits per path and answers with a
// status chosen by path, after an optional delay.
type hitCounter struct {
	mu    sync.Mutex
	hits  map[string]int
	delay time.Duration
}

func (h *hitCounter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.hits[r.URL.RequestURI()]++
	h.mu.Unlock()
	time.Sleep(h.delay)
	switch r.URL.Path {
	case "/fail":
		w.WriteHeader(http.StatusInternalServerError)
	case "/missing":
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

func (h *hitCounter) count(uri string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.hits[uri]
}

func target(name, url string, weight int) stress.Target {
	return stress.Target{Name: name, Method: http.MethodGet, Path: url, URLs: []string{url}, Header: http.Header{}, Weight: weight}
}

var _ = Describe("Stages", func() {
	DescribeTable("expands the ramp",
		func(r config.StressRamp, expected []int) {
			Expect(stress.Stages(r)).To(Equal(expected))
		},
		Entry("aligned steps", config.StressRamp{StartRPS: 10, StepRPS: 10, MaxRPS: 40}, []int{10, 20, 30, 40}),
		Entry("misaligned last step is clamped to max", config.StressRamp{StartRPS: 10, StepRPS: 20, MaxRPS: 60}, []int{10, 30, 50, 60}),
		Entry("single stage", config.StressRamp{StartRPS: 50, MaxRPS: 50}, []int{50}),
		Entry("single stage with step", config.StressRamp{StartRPS: 50, StepRPS: 10, MaxRPS: 50}, []int{50}),
		Entry("invalid ramp yields nothing", config.StressRamp{StartRPS: 50, StepRPS: 10, MaxRPS: 10}, nil),
	)
})

var _ = Describe("Engine", func() {
	var (
		server  *httptest.Server
		handler *hitCounter
		engine  *stress.Engine
	)

	BeforeEach(func() {
		handler = &hitCounter{hits: map[string]int{}}
		server = httptest.NewServer(handler)
		engine = stress.NewEngine(nil)
	})

	AfterEach(func() {
		server.Close()
	})

	It("advances through every stage and collects per-stage metrics", func() {
		handler.delay = 5 * time.Millisecond
		stages, err := engine.Ramp(context.Background(), server.URL, []stress.Target{target("ok", "/ok", 1)},
			[]int{20, 40}, 500*time.Millisecond, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(stages).To(HaveLen(2))

		Expect(stages[0].RequestedRPS).To(Equal(20))
		Expect(stages[1].RequestedRPS).To(Equal(40))
		Expect(stages[1].Requests).To(BeNumerically(">", stages[0].Requests))
		for _, s := range stages {
			Expect(s.ErrorRate).To(BeZero())
			Expect(s.StatusCodes).To(HaveKey("200"))
			Expect(s.P50).To(BeNumerically(">=", 5*time.Millisecond))
			Expect(s.P95).To(BeNumerically(">=", s.P50))
			Expect(s.P99).To(BeNumerically(">=", s.P95))
			Expect(s.Max).To(BeNumerically(">=", s.P99))
			Expect(s.Throughput).To(BeNumerically(">", 0))
			Expect(s.AchievedRPS).To(BeNumerically(">", 0))
			Expect(s.Duration).To(BeNumerically(">", 0))
		}
	})

	It("distributes the load by weight and rotates URL variants", func() {
		heavy := target("heavy", "/a", 3)
		heavy.URLs = []string{"/a?v=1", "/a?v=2", "/a?v=3"}
		_, err := engine.Ramp(context.Background(), server.URL+"/",
			[]stress.Target{heavy, target("light", "/b", 1)}, []int{80}, 500*time.Millisecond, nil)
		Expect(err).NotTo(HaveOccurred())

		a := handler.count("/a?v=1") + handler.count("/a?v=2") + handler.count("/a?v=3")
		b := handler.count("/b")
		Expect(b).To(BeNumerically(">", 0))
		Expect(float64(a) / float64(b)).To(BeNumerically("~", 3.0, 0.3))
		Expect(handler.count("/a?v=1")).To(BeNumerically("~", handler.count("/a?v=3"), 1))
	})

	It("counts 5xx as errors and 4xx apart", func() {
		stages, err := engine.Ramp(context.Background(), server.URL, []stress.Target{
			target("ok", "/ok", 2), target("fail", "/fail", 1), target("missing", "/missing", 1),
		}, []int{40}, 500*time.Millisecond, nil)
		Expect(err).NotTo(HaveOccurred())

		s := stages[0]
		Expect(s.ServerErrors).To(BeNumerically(">", 0))
		Expect(s.ClientErrors).To(BeNumerically(">", 0))
		Expect(s.NetworkErrors).To(BeZero())
		Expect(s.ErrorRate).To(BeNumerically("~", 0.25, 0.1))
		Expect(s.ClientErrorRate).To(BeNumerically("~", 0.25, 0.1))
		Expect(s.Errors).NotTo(BeEmpty())
	})

	It("counts unreachable targets as network errors", func() {
		url := server.URL
		server.Close()
		stages, err := engine.Ramp(context.Background(), url, []stress.Target{target("ok", "/ok", 1)},
			[]int{20}, 300*time.Millisecond, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(stages[0].NetworkErrors).To(BeNumerically(">", 0))
		Expect(stages[0].ErrorRate).To(Equal(1.0))
		Expect(stages[0].Throughput).To(BeZero())
	})

	It("stops early once the breaking point is reached", func() {
		stages, err := engine.Ramp(context.Background(), server.URL, []stress.Target{target("fail", "/fail", 1)},
			[]int{20, 40, 60}, 300*time.Millisecond, func(s stress.StageMetrics) bool { return s.ErrorRate > 0.5 })
		Expect(err).NotTo(HaveOccurred())
		Expect(stages).To(HaveLen(1))
	})

	It("aborts when the context is cancelled", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		start := time.Now()
		stages, err := engine.Ramp(ctx, server.URL, []stress.Target{target("ok", "/ok", 1)},
			[]int{20, 40}, 5*time.Second, nil)
		Expect(err).To(MatchError(context.DeadlineExceeded))
		Expect(stages).To(BeEmpty())
		Expect(time.Since(start)).To(BeNumerically("<", 2*time.Second))
	})
})
