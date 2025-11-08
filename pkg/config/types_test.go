package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.NotNil(t, cfg)
	assert.Equal(t, "v1", cfg.Version)
	assert.True(t, cfg.Orchestration.ParallelMocks)
	assert.Equal(t, 180*time.Second, cfg.Orchestration.StartupTimeout)
	assert.Equal(t, 3*time.Second, cfg.Orchestration.HealthCheckInterval)
	assert.Equal(t, 60, cfg.Orchestration.HealthCheckRetries)
	assert.True(t, cfg.Orchestration.CleanupOnFailure)
	assert.True(t, cfg.Orchestration.PreserveLogs)
	assert.False(t, cfg.Observability.StructuredLogs)
	assert.Equal(t, "info", cfg.Observability.LogLevel)
	assert.True(t, cfg.Observability.MetricsEnabled)
	assert.Equal(t, "text", cfg.Observability.ReportFormat)
}

func TestConfigStructure(t *testing.T) {
	cfg := &Config{
		Version:       "v1",
		AppTechnology: "golang",
		AppConfig: AppConfig{
			BinaryName: "myapp",
			BinaryPath: "./bin",
			Port:       8080,
			Environment: map[string]string{
				"LOG_LEVEL": "debug",
			},
		},
		TestLauncher: "test-launcher-back",
		TestConfig: TestConfig{
			Tags:         "@smoke",
			FeaturesPath: "./features",
			ReportsPath:  "./reports",
			Parallel:     true,
		},
		ThirdParty: ThirdPartyConfig{
			Mocks: []string{"couchbase", "pubsub"},
			MockConfig: map[string]interface{}{
				"couchbase": map[string]interface{}{
					"bucket": "test",
				},
			},
		},
		Orchestration: OrchestrationConfig{
			ParallelMocks:       true,
			StartupTimeout:      180 * time.Second,
			HealthCheckInterval: 3 * time.Second,
			HealthCheckRetries:  60,
			CleanupOnFailure:    true,
			PreserveLogs:        true,
		},
		Observability: ObservabilityConfig{
			StructuredLogs: true,
			LogLevel:       "debug",
			MetricsEnabled: true,
			ReportFormat:   "json",
		},
	}

	// Test basic fields
	assert.Equal(t, "v1", cfg.Version)
	assert.Equal(t, "golang", cfg.AppTechnology)
	assert.Equal(t, "test-launcher-back", cfg.TestLauncher)

	// Test app config
	assert.Equal(t, "myapp", cfg.AppConfig.BinaryName)
	assert.Equal(t, "./bin", cfg.AppConfig.BinaryPath)
	assert.Equal(t, 8080, cfg.AppConfig.Port)
	assert.Equal(t, "debug", cfg.AppConfig.Environment["LOG_LEVEL"])

	// Test test config
	assert.Equal(t, "@smoke", cfg.TestConfig.Tags)
	assert.Equal(t, "./features", cfg.TestConfig.FeaturesPath)
	assert.Equal(t, "./reports", cfg.TestConfig.ReportsPath)
	assert.True(t, cfg.TestConfig.Parallel)

	// Test third party
	assert.Len(t, cfg.ThirdParty.Mocks, 2)
	assert.Contains(t, cfg.ThirdParty.Mocks, "couchbase")
	assert.Contains(t, cfg.ThirdParty.Mocks, "pubsub")

	// Test orchestration
	assert.True(t, cfg.Orchestration.ParallelMocks)
	assert.Equal(t, 180*time.Second, cfg.Orchestration.StartupTimeout)
	assert.Equal(t, 3*time.Second, cfg.Orchestration.HealthCheckInterval)
	assert.Equal(t, 60, cfg.Orchestration.HealthCheckRetries)

	// Test observability
	assert.True(t, cfg.Observability.StructuredLogs)
	assert.Equal(t, "debug", cfg.Observability.LogLevel)
	assert.True(t, cfg.Observability.MetricsEnabled)
	assert.Equal(t, "json", cfg.Observability.ReportFormat)
}

func TestAppConfig(t *testing.T) {
	appCfg := AppConfig{
		BinaryName:  "test-app",
		BinaryPath:  "/usr/local/bin",
		DockerImage: "myapp:latest",
		Port:        3000,
		Environment: map[string]string{
			"ENV":       "test",
			"LOG_LEVEL": "info",
		},
	}

	assert.Equal(t, "test-app", appCfg.BinaryName)
	assert.Equal(t, "/usr/local/bin", appCfg.BinaryPath)
	assert.Equal(t, "myapp:latest", appCfg.DockerImage)
	assert.Equal(t, 3000, appCfg.Port)
	assert.Len(t, appCfg.Environment, 2)
}

func TestTestConfig(t *testing.T) {
	testCfg := TestConfig{
		Tags:         "@integration",
		FeaturesPath: "/app/features",
		ReportsPath:  "/app/reports",
		Parallel:     false,
	}

	assert.Equal(t, "@integration", testCfg.Tags)
	assert.Equal(t, "/app/features", testCfg.FeaturesPath)
	assert.Equal(t, "/app/reports", testCfg.ReportsPath)
	assert.False(t, testCfg.Parallel)
}

func TestThirdPartyConfig(t *testing.T) {
	thirdParty := ThirdPartyConfig{
		Mocks: []string{"kafka", "postgresql"},
		MockConfig: map[string]interface{}{
			"kafka": map[string]interface{}{
				"topics": []string{"events", "logs"},
			},
			"postgresql": map[string]interface{}{
				"database": "testdb",
			},
		},
	}

	assert.Len(t, thirdParty.Mocks, 2)
	assert.Len(t, thirdParty.MockConfig, 2)
	assert.Contains(t, thirdParty.MockConfig, "kafka")
	assert.Contains(t, thirdParty.MockConfig, "postgresql")
}

func TestOrchestrationConfig(t *testing.T) {
	orch := OrchestrationConfig{
		ParallelMocks:       false,
		StartupTimeout:      300 * time.Second,
		HealthCheckInterval: 5 * time.Second,
		HealthCheckRetries:  30,
		CleanupOnFailure:    false,
		PreserveLogs:        false,
	}

	assert.False(t, orch.ParallelMocks)
	assert.Equal(t, 300*time.Second, orch.StartupTimeout)
	assert.Equal(t, 5*time.Second, orch.HealthCheckInterval)
	assert.Equal(t, 30, orch.HealthCheckRetries)
	assert.False(t, orch.CleanupOnFailure)
	assert.False(t, orch.PreserveLogs)
}

func TestObservabilityConfig(t *testing.T) {
	obs := ObservabilityConfig{
		StructuredLogs: true,
		LogLevel:       "warn",
		MetricsEnabled: false,
		ReportFormat:   "html",
	}

	assert.True(t, obs.StructuredLogs)
	assert.Equal(t, "warn", obs.LogLevel)
	assert.False(t, obs.MetricsEnabled)
	assert.Equal(t, "html", obs.ReportFormat)
}

func BenchmarkDefaultConfig(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = DefaultConfig()
	}
}
