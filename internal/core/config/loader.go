package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/oswaldo-montano/gtool/pkg/config"
	gtErrors "github.com/oswaldo-montano/gtool/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Loader handles configuration loading from files
type Loader struct {
	expandEnv bool
}

// NewLoader creates a new config loader
func NewLoader() *Loader {
	return &Loader{
		expandEnv: true,
	}
}

// Load loads configuration from a file
func (l *Loader) Load(path string) (*config.Config, error) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigNotFound,
			fmt.Sprintf("configuration file not found: %s", path))
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigInvalid,
			"failed to read configuration file")
	}

	// Expand environment variables if enabled
	if l.expandEnv {
		data = []byte(l.expandEnvVars(string(data)))
	}

	// Parse YAML
	cfg := config.DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigInvalid,
			"failed to parse YAML configuration")
	}

	// Apply environment variable overrides
	l.applyEnvOverrides(cfg)

	return cfg, nil
}

// LoadFromPath loads configuration from default paths
func (l *Loader) LoadFromPath() (*config.Config, error) {
	// Try default paths
	paths := []string{
		"./component-config.yml",
		"./component-config.yaml",
		"./gtool-config.yml",
		"./gtool-config.yaml",
	}

	var lastErr error
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			cfg, err := l.Load(path)
			if err != nil {
				lastErr = err
				continue
			}
			return cfg, nil
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, gtErrors.New(gtErrors.ErrConfigNotFound,
		"no configuration file found in current directory. Expected: component-config.yml or gtool-config.yml")
}

// LoadFromPathOrDefault loads configuration or returns default
func (l *Loader) LoadFromPathOrDefault() *config.Config {
	cfg, err := l.LoadFromPath()
	if err != nil {
		return config.DefaultConfig()
	}
	return cfg
}

// expandEnvVars expands environment variables in the format ${VAR} or $VAR
func (l *Loader) expandEnvVars(content string) string {
	// Pattern matches ${VAR} or $VAR
	re := regexp.MustCompile(`\$\{([^}]+)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

	return re.ReplaceAllStringFunc(content, func(match string) string {
		// Extract variable name
		varName := strings.TrimPrefix(match, "$")
		varName = strings.TrimPrefix(varName, "{")
		varName = strings.TrimSuffix(varName, "}")

		// Get value from environment
		if value := os.Getenv(varName); value != "" {
			return value
		}

		// Keep original if not found
		return match
	})
}

// applyEnvOverrides applies environment variable overrides to configuration
func (l *Loader) applyEnvOverrides(cfg *config.Config) {
	// GTOOL_LOG_LEVEL overrides log level
	if logLevel := os.Getenv("GTOOL_LOG_LEVEL"); logLevel != "" {
		cfg.Observability.LogLevel = logLevel
	}

	// GTOOL_PARALLEL_MOCKS overrides parallel mocks
	if parallelMocks := os.Getenv("GTOOL_PARALLEL_MOCKS"); parallelMocks != "" {
		cfg.Orchestration.ParallelMocks = parallelMocks == "true"
	}

	// GTOOL_DOCKER_IMAGE overrides docker image
	if dockerImage := os.Getenv("GTOOL_DOCKER_IMAGE"); dockerImage != "" {
		cfg.AppConfig.DockerImage = dockerImage
	}
}

// SetExpandEnv enables or disables environment variable expansion
func (l *Loader) SetExpandEnv(expand bool) {
	l.expandEnv = expand
}

// LoadConfig is a convenience function to load configuration from a file
// If path is empty, it tries to load from default paths
func LoadConfig(path string) (*config.Config, error) {
	loader := NewLoader()

	if path == "" {
		return loader.LoadFromPath()
	}

	return loader.Load(path)
}
