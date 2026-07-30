package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
	"gopkg.in/yaml.v3"
)

type Loader struct {
	expandEnv bool
}

func NewLoader() *Loader {
	return &Loader{
		expandEnv: true,
	}
}

func (l *Loader) Load(path string) (*config.Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigNotFound,
			fmt.Sprintf("configuration file not found: %s", path))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigInvalid,
			"failed to read configuration file")
	}

	if l.expandEnv {
		data = []byte(l.expandEnvVars(string(data)))
	}

	cfg := config.DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigInvalid,
			"failed to parse YAML configuration")
	}

	l.applyEnvOverrides(cfg)

	return cfg, nil
}

func (l *Loader) LoadFromPath() (*config.Config, error) {
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

func (l *Loader) LoadFromPathOrDefault() *config.Config {
	cfg, err := l.LoadFromPath()
	if err != nil {
		return config.DefaultConfig()
	}
	return cfg
}

func (l *Loader) expandEnvVars(content string) string {
	re := regexp.MustCompile(`\$\{([^}]+)}|\$([A-Za-z_][A-Za-z0-9_]*)`)

	return re.ReplaceAllStringFunc(content, func(match string) string {
		varName := strings.TrimPrefix(match, "$")
		varName = strings.TrimPrefix(varName, "{")
		varName = strings.TrimSuffix(varName, "}")

		if value := os.Getenv(varName); value != "" {
			return value
		}

		return match
	})
}

func (l *Loader) applyEnvOverrides(cfg *config.Config) {
	if logLevel := os.Getenv("GTOOL_LOG_LEVEL"); logLevel != "" {
		cfg.Observability.LogLevel = logLevel
	}

	if parallelMocks := os.Getenv("GTOOL_PARALLEL_MOCKS"); parallelMocks != "" {
		cfg.Orchestration.ParallelMocks = parallelMocks == "true"
	}

	if dockerImage := os.Getenv("GTOOL_DOCKER_IMAGE"); dockerImage != "" {
		cfg.AppConfig.DockerImage = dockerImage
	}
}

func (l *Loader) SetExpandEnv(expand bool) {
	l.expandEnv = expand
}

func LoadConfig(path string) (*config.Config, error) {
	loader := NewLoader()

	if path == "" {
		return loader.LoadFromPath()
	}

	return loader.Load(path)
}
