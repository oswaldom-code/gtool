package config

import "time"

type Config struct {
	Version       string              `yaml:"version" json:"version"`
	AppTechnology string              `yaml:"app-technology" json:"app-technology"`
	AppConfig     AppConfig           `yaml:"app-config" json:"app-config"`
	TestLauncher  string              `yaml:"test-launcher" json:"test-launcher"`
	TestConfig    TestConfig          `yaml:"test-config" json:"test-config"`
	ThirdParty    ThirdPartyConfig    `yaml:"third-party" json:"third-party"`
	Orchestration OrchestrationConfig `yaml:"orchestration" json:"orchestration"`
	Observability ObservabilityConfig `yaml:"observability" json:"observability"`
	StressConfig  *StressConfig       `yaml:"stress-config,omitempty" json:"stress-config,omitempty"`
}

type AppConfig struct {
	BinaryName  string `yaml:"binary-name" json:"binary-name"`
	BinaryPath  string `yaml:"binary-path" json:"binary-path"`
	DockerImage string `yaml:"docker-image" json:"docker-image"`
	Port        int    `yaml:"port" json:"port"`
	HostPort    string `yaml:"host-port" json:"host-port"`
	// HealthPath is the HTTP path polled until the app answers before tests
	// run (any status counts as ready). Defaults to "/".
	HealthPath  string            `yaml:"health-path" json:"health-path"`
	Environment map[string]string `yaml:"environment" json:"environment"`
}

type TestConfig struct {
	Tags         string `yaml:"tags" json:"tags"`
	FeaturesPath string `yaml:"features-path" json:"features-path"`
	ReportsPath  string `yaml:"reports-path" json:"reports-path"`
	Parallel     bool   `yaml:"parallel" json:"parallel"`
}

type ThirdPartyConfig struct {
	Mocks      []string               `yaml:"mocks" json:"mocks"`
	MockConfig map[string]interface{} `yaml:"mock-config" json:"mock-config"`
}

type OrchestrationConfig struct {
	ParallelMocks       bool          `yaml:"parallel-mocks" json:"parallel-mocks"`
	StartupTimeout      time.Duration `yaml:"startup-timeout" json:"startup-timeout"`
	HealthCheckInterval time.Duration `yaml:"health-check-interval" json:"health-check-interval"`
	HealthCheckRetries  int           `yaml:"health-check-retries" json:"health-check-retries"`
	CleanupOnFailure    bool          `yaml:"cleanup-on-failure" json:"cleanup-on-failure"`
}

type ObservabilityConfig struct {
	LogLevel string `yaml:"log-level" json:"log-level"`
}

// StressConfig drives `gtool stress`: which targets to hit, how to ramp the
// load and which thresholds define the degradation and breaking points.
type StressConfig struct {
	Source           StressSource     `yaml:"source" json:"source"`
	Targets          []StressTarget   `yaml:"targets" json:"targets"`
	Ramp             StressRamp       `yaml:"ramp" json:"ramp"`
	Thresholds       StressThresholds `yaml:"thresholds" json:"thresholds"`
	MinAcceptableRPS int              `yaml:"min-acceptable-rps" json:"min-acceptable-rps"`
	ReportsPath      string           `yaml:"reports-path" json:"reports-path"`
}

// StressSource resolves targets from an OpenAPI contract (Swagger 2.0 or OpenAPI 3.x).
type StressSource struct {
	OpenAPI            string                             `yaml:"openapi" json:"openapi"`
	BasePath           string                             `yaml:"base-path" json:"base-path"`
	Methods            []string                           `yaml:"methods" json:"methods"`
	IncludeTags        []string                           `yaml:"include-tags" json:"include-tags"`
	ExcludeTags        []string                           `yaml:"exclude-tags" json:"exclude-tags"`
	IncludePaths       []string                           `yaml:"include-paths" json:"include-paths"`
	ExcludePaths       []string                           `yaml:"exclude-paths" json:"exclude-paths"`
	Headers            map[string]string                  `yaml:"headers" json:"headers"`
	ParamValues        map[string][]string                `yaml:"param-values" json:"param-values"`
	OperationOverrides map[string]StressOperationOverride `yaml:"operation-overrides" json:"operation-overrides"`
}

// StressOperationOverride customizes a spec operation, keyed by operationId.
type StressOperationOverride struct {
	BodyFile string `yaml:"body-file" json:"body-file"`
	Weight   int    `yaml:"weight" json:"weight"`
}

// StressTarget is an explicitly declared target. It takes precedence over a
// spec operation with the same method and path.
type StressTarget struct {
	Name     string            `yaml:"name" json:"name"`
	Method   string            `yaml:"method" json:"method"`
	Path     string            `yaml:"path" json:"path"`
	Headers  map[string]string `yaml:"headers" json:"headers"`
	Body     string            `yaml:"body" json:"body"`
	BodyFile string            `yaml:"body-file" json:"body-file"`
	Weight   int               `yaml:"weight" json:"weight"`
}

// StressRamp is a stepped load ramp: start-rps, then +step-rps every
// step-duration until max-rps.
type StressRamp struct {
	StartRPS     int           `yaml:"start-rps" json:"start-rps"`
	StepRPS      int           `yaml:"step-rps" json:"step-rps"`
	StepDuration time.Duration `yaml:"step-duration" json:"step-duration"`
	MaxRPS       int           `yaml:"max-rps" json:"max-rps"`
}

// StressThresholds define the degradation and breaking points. Zero values
// fall back to the defaults applied by the stress runner.
type StressThresholds struct {
	DegradationLatencyFactor float64 `yaml:"degradation-latency-factor" json:"degradation-latency-factor"`
	DegradationP95Ms         int     `yaml:"degradation-p95-ms" json:"degradation-p95-ms"`
	BreakErrorRate           float64 `yaml:"break-error-rate" json:"break-error-rate"`
}

func DefaultConfig() *Config {
	return &Config{
		Version: "v1",
		Orchestration: OrchestrationConfig{
			ParallelMocks:       true,
			StartupTimeout:      180 * time.Second,
			HealthCheckInterval: 3 * time.Second,
			HealthCheckRetries:  60,
			CleanupOnFailure:    true,
		},
		Observability: ObservabilityConfig{
			LogLevel: "info",
		},
	}
}
