package config

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

var (
	supportedVersions     = []string{"v1"}
	supportedTechnologies = []string{"golang", "nodejs", "generic"}
	supportedLaunchers    = []string{"test-launcher-back", "test-launcher-front"}
	supportedMocks        = []string{"mountebank", "couchbase", "postgresql", "kafka", "pubsub", "gcs", "redis", "mongodb", "mysql", "minio", "floci"}
	supportedHTTPMethods  = []string{
		http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions,
	}
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

	if cfg.StressConfig != nil {
		errors = append(errors, v.stressErrors(cfg.StressConfig)...)
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

func (v *Validator) validateStressSource(src config.StressSource) []string {
	var errs []string
	if v.checkPaths && src.OpenAPI != "" {
		if _, err := os.Stat(src.OpenAPI); os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("stress-config.source.openapi does not exist: %s", src.OpenAPI))
		}
	}
	for _, m := range src.Methods {
		if !contains(supportedHTTPMethods, strings.ToUpper(m)) {
			errs = append(errs, fmt.Sprintf("unsupported stress-config.source.methods value '%s'. Supported: %v", m, supportedHTTPMethods))
		}
	}
	if src.BasePath != "" && !strings.HasPrefix(src.BasePath, "/") {
		errs = append(errs, "stress-config.source.base-path must start with '/'")
	}
	for opID, o := range src.OperationOverrides {
		if o.Weight < 0 {
			errs = append(errs, fmt.Sprintf("stress-config.source.operation-overrides.%s.weight must not be negative", opID))
		}
		if v.checkPaths && o.BodyFile != "" {
			if _, err := os.Stat(o.BodyFile); os.IsNotExist(err) {
				errs = append(errs, fmt.Sprintf("stress-config.source.operation-overrides.%s.body-file does not exist: %s", opID, o.BodyFile))
			}
		}
	}
	return errs
}

func (v *Validator) validateStressTargets(targets []config.StressTarget) []string {
	var errs []string
	for i, t := range targets {
		prefix := fmt.Sprintf("stress-config.targets[%d]", i)
		if !contains(supportedHTTPMethods, strings.ToUpper(t.Method)) {
			errs = append(errs, fmt.Sprintf("%s.method '%s' is not supported. Supported: %v", prefix, t.Method, supportedHTTPMethods))
		}
		if !strings.HasPrefix(t.Path, "/") {
			errs = append(errs, fmt.Sprintf("%s.path must start with '/'", prefix))
		}
		if t.Body != "" && t.BodyFile != "" {
			errs = append(errs, fmt.Sprintf("%s: body and body-file are mutually exclusive", prefix))
		}
		if t.Weight < 0 {
			errs = append(errs, fmt.Sprintf("%s.weight must not be negative", prefix))
		}
		if v.checkPaths && t.BodyFile != "" {
			if _, err := os.Stat(t.BodyFile); os.IsNotExist(err) {
				errs = append(errs, fmt.Sprintf("%s.body-file does not exist: %s", prefix, t.BodyFile))
			}
		}
	}
	return errs
}

func validateStressRamp(r config.StressRamp) []string {
	var errs []string
	if r.StartRPS <= 0 {
		errs = append(errs, "stress-config.ramp.start-rps must be positive")
	}
	if r.MaxRPS < r.StartRPS {
		errs = append(errs, "stress-config.ramp.max-rps must be greater than or equal to start-rps")
	}
	if r.StepRPS < 0 || (r.StepRPS == 0 && r.MaxRPS > r.StartRPS) {
		errs = append(errs, "stress-config.ramp.step-rps must be positive when max-rps is greater than start-rps")
	}
	if r.StepDuration <= 0 {
		errs = append(errs, "stress-config.ramp.step-duration must be positive")
	}
	return errs
}

func validateStressThresholds(t config.StressThresholds) []string {
	var errs []string
	if t.DegradationLatencyFactor != 0 && t.DegradationLatencyFactor <= 1 {
		errs = append(errs, "stress-config.thresholds.degradation-latency-factor must be greater than 1")
	}
	if t.DegradationP95Ms < 0 {
		errs = append(errs, "stress-config.thresholds.degradation-p95-ms must not be negative")
	}
	if t.BreakErrorRate < 0 || t.BreakErrorRate > 1 {
		errs = append(errs, "stress-config.thresholds.break-error-rate must be between 0 and 1")
	}
	return errs
}

func (v *Validator) stressErrors(sc *config.StressConfig) []string {
	var errs []string
	if sc.Source.OpenAPI == "" && len(sc.Targets) == 0 {
		errs = append(errs, "stress-config requires source.openapi or at least one entry in targets")
	}
	errs = append(errs, v.validateStressSource(sc.Source)...)
	errs = append(errs, v.validateStressTargets(sc.Targets)...)
	errs = append(errs, validateStressRamp(sc.Ramp)...)
	errs = append(errs, validateStressThresholds(sc.Thresholds)...)
	if sc.MinAcceptableRPS < 0 {
		errs = append(errs, "stress-config.min-acceptable-rps must not be negative")
	}
	if sc.MinAcceptableRPS > sc.Ramp.MaxRPS {
		errs = append(errs, "stress-config.min-acceptable-rps must not exceed ramp.max-rps")
	}
	return errs
}

// ValidateStress validates only the stress-config block, so `gtool stress`
// does not require the component-test settings (test-launcher, etc.).
func (v *Validator) ValidateStress(cfg *config.Config) error {
	if cfg.StressConfig == nil {
		return gtErrors.New(gtErrors.ErrConfigInvalid, "stress-config section is required")
	}
	if errs := v.stressErrors(cfg.StressConfig); len(errs) > 0 {
		return gtErrors.New(gtErrors.ErrConfigInvalid,
			fmt.Sprintf("stress configuration validation failed:\n  - %s", joinErrors(errs)))
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
