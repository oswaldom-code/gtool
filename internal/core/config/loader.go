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

// envRef matches $$ (a literal $), ${NAME} and $NAME. Only valid variable
// names match, so gtool placeholders such as ${mocks.postgresql.port} are left
// for ports.Interpolate.
var envRef = regexp.MustCompile(`\$\$|\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// expandValue replaces environment references in one scalar value. Unset
// variables keep their reference; set-but-empty ones expand to "".
func expandValue(value string) string {
	return envRef.ReplaceAllStringFunc(value, func(match string) string {
		if match == "$$" {
			return "$"
		}
		name := strings.Trim(match, "${}")
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return match
	})
}

// expandNode expands scalar values (never mapping keys) after parsing, so a
// variable can never change the document structure. A plain scalar that
// changed is re-typed, so port: ${PORT} still decodes into an int.
func expandNode(n *yaml.Node) {
	switch n.Kind {
	case yaml.ScalarNode:
		if v := expandValue(n.Value); v != n.Value {
			n.Value = v
			if n.Style == 0 {
				n.Tag = ""
			}
		}
	case yaml.MappingNode:
		for i := 1; i < len(n.Content); i += 2 {
			expandNode(n.Content[i])
		}
	default:
		for _, c := range n.Content {
			expandNode(c)
		}
	}
}

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

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigInvalid,
			"failed to parse YAML configuration")
	}
	if l.expandEnv {
		expandNode(&root)
	}

	cfg := config.DefaultConfig()
	if len(root.Content) > 0 {
		if err := root.Decode(cfg); err != nil {
			return nil, gtErrors.Wrap(err, gtErrors.ErrConfigInvalid,
				"failed to parse YAML configuration")
		}
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
