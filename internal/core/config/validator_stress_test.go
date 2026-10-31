package config_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oswaldom-code/gtool/internal/core/config"
	pkgConfig "github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

func validStressConfig() *pkgConfig.StressConfig {
	return &pkgConfig.StressConfig{
		Targets: []pkgConfig.StressTarget{{Name: "users", Method: "GET", Path: "/users"}},
		Ramp: pkgConfig.StressRamp{
			StartRPS: 10, StepRPS: 10, StepDuration: 20 * time.Second, MaxRPS: 100,
		},
		MinAcceptableRPS: 50,
	}
}

var _ = Describe("Validator stress-config", func() {
	var (
		validator *config.Validator
		cfg       *pkgConfig.Config
	)

	BeforeEach(func() {
		validator = config.NewValidator()
		cfg = pkgConfig.DefaultConfig()
		cfg.StressConfig = validStressConfig()
	})

	Describe("ValidateStress", func() {
		It("accepts a valid stress config", func() {
			Expect(validator.ValidateStress(cfg)).To(Succeed())
		})

		It("accepts a single-stage ramp without step-rps", func() {
			cfg.StressConfig.Ramp = pkgConfig.StressRamp{StartRPS: 50, StepDuration: time.Second, MaxRPS: 50}
			cfg.StressConfig.MinAcceptableRPS = 0
			Expect(validator.ValidateStress(cfg)).To(Succeed())
		})

		It("requires the stress-config section", func() {
			cfg.StressConfig = nil
			err := validator.ValidateStress(cfg)
			Expect(err).To(HaveOccurred())
			Expect(gtErrors.Is(err, gtErrors.ErrConfigInvalid)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("stress-config section is required"))
		})

		It("accepts an existing openapi file and body files", func() {
			dir := GinkgoT().TempDir()
			spec := filepath.Join(dir, "swagger.yml")
			body := filepath.Join(dir, "body.json")
			Expect(os.WriteFile(spec, []byte("openapi: 3.0.0"), 0o644)).To(Succeed())
			Expect(os.WriteFile(body, []byte("{}"), 0o644)).To(Succeed())

			cfg.StressConfig.Targets = nil
			cfg.StressConfig.Source = pkgConfig.StressSource{
				OpenAPI:            spec,
				BasePath:           "/api",
				Methods:            []string{"get", "POST"},
				OperationOverrides: map[string]pkgConfig.StressOperationOverride{"createUser": {BodyFile: body, Weight: 3}},
			}
			Expect(validator.ValidateStress(cfg)).To(Succeed())
		})

		DescribeTable("rejects invalid values",
			func(mutate func(sc *pkgConfig.StressConfig), expected string) {
				mutate(cfg.StressConfig)
				err := validator.ValidateStress(cfg)
				Expect(err).To(HaveOccurred())
				Expect(gtErrors.Is(err, gtErrors.ErrConfigInvalid)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring(expected))
			},
			Entry("no source nor targets", func(sc *pkgConfig.StressConfig) {
				sc.Targets = nil
			}, "requires source.openapi or at least one entry in targets"),
			Entry("missing openapi file", func(sc *pkgConfig.StressConfig) {
				sc.Source.OpenAPI = "/does/not/exist.yml"
			}, "source.openapi does not exist"),
			Entry("unsupported source method", func(sc *pkgConfig.StressConfig) {
				sc.Source.Methods = []string{"FETCH"}
			}, "unsupported stress-config.source.methods value 'FETCH'"),
			Entry("relative base path", func(sc *pkgConfig.StressConfig) {
				sc.Source.BasePath = "api"
			}, "base-path must start with '/'"),
			Entry("negative override weight", func(sc *pkgConfig.StressConfig) {
				sc.Source.OperationOverrides = map[string]pkgConfig.StressOperationOverride{"op": {Weight: -1}}
			}, "operation-overrides.op.weight must not be negative"),
			Entry("missing override body file", func(sc *pkgConfig.StressConfig) {
				sc.Source.OperationOverrides = map[string]pkgConfig.StressOperationOverride{"op": {BodyFile: "/nope.json"}}
			}, "operation-overrides.op.body-file does not exist"),
			Entry("unsupported target method", func(sc *pkgConfig.StressConfig) {
				sc.Targets[0].Method = "BREW"
			}, "targets[0].method 'BREW' is not supported"),
			Entry("relative target path", func(sc *pkgConfig.StressConfig) {
				sc.Targets[0].Path = "users"
			}, "targets[0].path must start with '/'"),
			Entry("body and body-file together", func(sc *pkgConfig.StressConfig) {
				sc.Targets[0].Body = "{}"
				sc.Targets[0].BodyFile = "x.json"
			}, "body and body-file are mutually exclusive"),
			Entry("negative target weight", func(sc *pkgConfig.StressConfig) {
				sc.Targets[0].Weight = -2
			}, "targets[0].weight must not be negative"),
			Entry("missing target body file", func(sc *pkgConfig.StressConfig) {
				sc.Targets[0].BodyFile = "/nope.json"
			}, "targets[0].body-file does not exist"),
			Entry("zero start-rps", func(sc *pkgConfig.StressConfig) {
				sc.Ramp.StartRPS = 0
			}, "ramp.start-rps must be positive"),
			Entry("max below start", func(sc *pkgConfig.StressConfig) {
				sc.Ramp.MaxRPS = 5
				sc.MinAcceptableRPS = 0
			}, "max-rps must be greater than or equal to start-rps"),
			Entry("missing step-rps", func(sc *pkgConfig.StressConfig) {
				sc.Ramp.StepRPS = 0
			}, "step-rps must be positive"),
			Entry("missing step-duration", func(sc *pkgConfig.StressConfig) {
				sc.Ramp.StepDuration = 0
			}, "step-duration must be positive"),
			Entry("latency factor not above 1", func(sc *pkgConfig.StressConfig) {
				sc.Thresholds.DegradationLatencyFactor = 1
			}, "degradation-latency-factor must be greater than 1"),
			Entry("negative p95 SLO", func(sc *pkgConfig.StressConfig) {
				sc.Thresholds.DegradationP95Ms = -1
			}, "degradation-p95-ms must not be negative"),
			Entry("error rate above 1", func(sc *pkgConfig.StressConfig) {
				sc.Thresholds.BreakErrorRate = 1.5
			}, "break-error-rate must be between 0 and 1"),
			Entry("negative min-acceptable-rps", func(sc *pkgConfig.StressConfig) {
				sc.MinAcceptableRPS = -1
			}, "min-acceptable-rps must not be negative"),
			Entry("min-acceptable-rps above max-rps", func(sc *pkgConfig.StressConfig) {
				sc.MinAcceptableRPS = 1000
			}, "min-acceptable-rps must not exceed ramp.max-rps"),
		)

		It("skips path checks when disabled", func() {
			validator.SetCheckPaths(false)
			cfg.StressConfig.Source.OpenAPI = "/does/not/exist.yml"
			Expect(validator.ValidateStress(cfg)).To(Succeed())
		})
	})

	Describe("loading from YAML", func() {
		It("maps every stress-config field", func() {
			cfg, err := config.NewLoader().Load(fixturesBasePath + "/stress.yml")
			Expect(err).NotTo(HaveOccurred())

			sc := cfg.StressConfig
			Expect(sc).NotTo(BeNil())
			Expect(sc.Source.OpenAPI).To(Equal("api/swagger.yml"))
			Expect(sc.Source.BasePath).To(Equal("/api/v1"))
			Expect(sc.Source.Methods).To(Equal([]string{"GET"}))
			Expect(sc.Source.IncludeTags).To(Equal([]string{"products"}))
			Expect(sc.Source.ExcludePaths).To(Equal([]string{"/health"}))
			Expect(sc.Source.Headers).To(HaveKey("Authorization"))
			Expect(sc.Source.ParamValues["productId"]).To(Equal([]string{"1", "2", "3"}))
			Expect(sc.Source.OperationOverrides["createOrder"]).To(Equal(
				pkgConfig.StressOperationOverride{BodyFile: "test/stress/order.json", Weight: 3}))
			Expect(sc.Targets).To(HaveLen(1))
			Expect(sc.Targets[0].Weight).To(Equal(2))
			Expect(sc.Ramp).To(Equal(pkgConfig.StressRamp{
				StartRPS: 10, StepRPS: 10, StepDuration: 20 * time.Second, MaxRPS: 500}))
			Expect(sc.Thresholds).To(Equal(pkgConfig.StressThresholds{
				DegradationLatencyFactor: 2.0, DegradationP95Ms: 800, BreakErrorRate: 0.05}))
			Expect(sc.MinAcceptableRPS).To(Equal(100))
			Expect(sc.ReportsPath).To(Equal("test/stress/reports"))
		})

		It("leaves stress-config nil when absent", func() {
			cfg, err := config.NewLoader().Load(minimalConfigPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.StressConfig).To(BeNil())
		})
	})

	Describe("Validate", func() {
		BeforeEach(func() {
			cfg.AppTechnology = "generic"
			cfg.TestLauncher = "test-launcher-back"
		})

		It("passes when stress-config is valid", func() {
			Expect(validator.Validate(cfg)).To(Succeed())
		})

		It("passes when stress-config is absent", func() {
			cfg.StressConfig = nil
			Expect(validator.Validate(cfg)).To(Succeed())
		})

		It("reports stress-config errors alongside the rest", func() {
			cfg.StressConfig.Ramp.StartRPS = 0
			err := validator.Validate(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ramp.start-rps must be positive"))
		})
	})
})
