// Package cliconfig loads the configuration for CLI commands with an explicit
// policy, so every command treats a missing or invalid file the same way.
package cliconfig

import (
	"fmt"
	"io"

	coreConfig "github.com/oswaldom-code/gtool/internal/core/config"
	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
	"github.com/oswaldom-code/gtool/pkg/logger"
)

// Policy states whether a command needs a config file and how strictly it is
// checked before anything starts.
type Policy struct {
	// Required fails when no file is given or found in the default locations.
	Required bool
	// Validate runs the validator on a loaded file (never on built-in defaults).
	Validate bool
	// CheckPaths makes the validator require configured paths to exist.
	CheckPaths bool
}

var (
	// Pipeline is for commands that run tests: a silent fallback to defaults
	// would report success without testing anything.
	Pipeline = Policy{Required: true, Validate: true, CheckPaths: true}
	// Compat is for the legacy --stable flow, whose configs follow the fixed
	// contract and do not satisfy the gtool validator.
	Compat = Policy{Required: true}
	// Start is for commands that start containers from an optional config.
	Start = Policy{Validate: true}
	// Inspect is for read-only and teardown commands, which must work even
	// with a config that would not validate.
	Inspect = Policy{}
)

// Load reads the config at path, or from the default locations when path is
// empty, applying p. When an optional config is absent it returns the
// built-in defaults and writes a notice to notice (if not nil).
func Load(path string, p Policy, notice io.Writer) (*config.Config, error) {
	cfg, err := coreConfig.LoadConfig(path)
	if err != nil {
		if path == "" && !p.Required && gtErrors.Is(err, gtErrors.ErrConfigNotFound) {
			if notice != nil {
				fmt.Fprintln(notice, "No configuration file found, using defaults")
			}
			return config.DefaultConfig(), nil
		}
		return nil, err
	}

	if err := logger.ApplyConfigLevel(cfg.Observability.LogLevel); err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "invalid observability.log-level")
	}

	if p.Validate {
		v := coreConfig.NewValidator()
		v.SetCheckPaths(p.CheckPaths)
		if err := v.Validate(cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}
