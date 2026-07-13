package config

import (
	"fmt"
	"os"

	"github.com/oswaldo-montano/gtool/pkg/config"
	gtErrors "github.com/oswaldo-montano/gtool/pkg/errors"
)

var (
	supportedVersions     = []string{"v1"}
	supportedTechnologies = []string{"golang", "nodejs", "generic"}
	supportedLaunchers    = []string{"test-launcher-back", "test-launcher-front"}
	supportedMocks        = []string{"mountebank", "couchbase", "postgresql", "kafka", "pubsub", "gcs", "redis", "mongodb", "mysql", "minio"}
)

type Validator struct {
	checkPaths bool
}

func NewValidator() *Validator {
	return &Validator{
		checkPaths: true,
	}
}

func (v *Validator) Validate(cfg *config.Config) error {
	var errors []string

	if err := v.validateVersion(cfg); err != nil {
		errors = append(errors, err.Error())
	}

	if err := v.validateAppTechnology(cfg); err != nil {
		errors = append(errors, err.Error())
	}

	if err := v.validateTestLauncher(cfg); err != nil {
		errors = append(errors, err.Error())
	}

	if err := v.validateAppConfig(cfg); err != nil {
		errors = append(errors, err.Error())
	}

	if err := v.validateTestConfig(cfg); err != nil {
		errors = append(errors, err.Error())
	}

	if err := v.validateMockServices(cfg); err != nil {
		errors = append(errors, err.Error())
	}

	if err := v.validateOrchestration(cfg); err != nil {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		return gtErrors.New(gtErrors.ErrConfigInvalid,
			fmt.Sprintf("configuration validation failed:\n  - %s", joinErrors(errors)))
	}

	return nil
}

func (v *Validator) validateVersion(cfg *config.Config) error {
	if cfg.Version == "" {
		return fmt.Errorf("version is required")
	}

	if !contains(supportedVersions, cfg.Version) {
		return fmt.Errorf("unsupported version '%s'. Supported: %v", cfg.Version, supportedVersions)
	}

	return nil
}

func (v *Validator) validateAppTechnology(cfg *config.Config) error {
	if cfg.AppTechnology == "" {
		return fmt.Errorf("app-technology is required")
	}

	if !contains(supportedTechnologies, cfg.AppTechnology) {
		return fmt.Errorf("unsupported app-technology '%s'. Supported: %v", cfg.AppTechnology, supportedTechnologies)
	}

	return nil
}

func (v *Validator) validateTestLauncher(cfg *config.Config) error {
	if cfg.TestLauncher == "" {
		return fmt.Errorf("test-launcher is required")
	}

	if !contains(supportedLaunchers, cfg.TestLauncher) {
		return fmt.Errorf("unsupported test-launcher '%s'. Supported: %v", cfg.TestLauncher, supportedLaunchers)
	}

	return nil
}

func (v *Validator) validateAppConfig(cfg *config.Config) error {
	appCfg := cfg.AppConfig

	if appCfg.DockerImage == "" {
		if cfg.AppTechnology == "golang" {
			if appCfg.BinaryName == "" {
				return fmt.Errorf("app-config.binary-name is required for golang technology")
			}
		}
	}

	if appCfg.Port < 0 || appCfg.Port > 65535 {
		return fmt.Errorf("app-config.port must be between 0 and 65535")
	}

	return nil
}

func (v *Validator) validateTestConfig(cfg *config.Config) error {
	testCfg := cfg.TestConfig

	if v.checkPaths {
		if testCfg.FeaturesPath != "" {
			if _, err := os.Stat(testCfg.FeaturesPath); os.IsNotExist(err) {
				return fmt.Errorf("test-config.features-path does not exist: %s", testCfg.FeaturesPath)
			}
		}
	}

	return nil
}

func (v *Validator) validateMockServices(cfg *config.Config) error {
	for _, mock := range cfg.ThirdParty.Mocks {
		if !contains(supportedMocks, mock) {
			return fmt.Errorf("unsupported mock service '%s'. Supported: %v", mock, supportedMocks)
		}
	}

	// Validate mock-specific configurations

	return nil
}

func (v *Validator) validateOrchestration(cfg *config.Config) error {
	orch := cfg.Orchestration

	if orch.StartupTimeout <= 0 {
		return fmt.Errorf("orchestration.startup-timeout must be positive")
	}

	if orch.HealthCheckInterval <= 0 {
		return fmt.Errorf("orchestration.health-check-interval must be positive")
	}

	if orch.HealthCheckRetries <= 0 {
		return fmt.Errorf("orchestration.health-check-retries must be positive")
	}

	return nil
}

func (v *Validator) SetCheckPaths(check bool) {
	v.checkPaths = check
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func joinErrors(errors []string) string {
	result := ""
	for i, err := range errors {
		if i > 0 {
			result += "\n  - "
		}
		result += err
	}
	return result
}
