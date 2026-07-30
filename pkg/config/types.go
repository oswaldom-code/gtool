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
}

type AppConfig struct {
	BinaryName  string            `yaml:"binary-name" json:"binary-name"`
	BinaryPath  string            `yaml:"binary-path" json:"binary-path"`
	DockerImage string            `yaml:"docker-image" json:"docker-image"`
	Port        int               `yaml:"port" json:"port"`
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
	PreserveLogs        bool          `yaml:"preserve-logs" json:"preserve-logs"`
}

type ObservabilityConfig struct {
	StructuredLogs bool   `yaml:"structured-logs" json:"structured-logs"`
	LogLevel       string `yaml:"log-level" json:"log-level"`
	MetricsEnabled bool   `yaml:"metrics-enabled" json:"metrics-enabled"`
	ReportFormat   string `yaml:"report-format" json:"report-format"`
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
			PreserveLogs:        true,
		},
		Observability: ObservabilityConfig{
			StructuredLogs: false,
			LogLevel:       "info",
			MetricsEnabled: true,
			ReportFormat:   "text",
		},
	}
}
