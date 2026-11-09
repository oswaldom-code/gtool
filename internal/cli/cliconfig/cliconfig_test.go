package cliconfig

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

const validConfig = `version: v1
app-technology: generic
test-launcher: test-launcher-back
app-config:
  docker-image: app:1
  port: 8080
test-config:
  features-path: ./missing-features
`

const invalidConfig = `version: v9
app-technology: generic
test-launcher: test-launcher-back
`

func writeFile(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "component-config.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// chdir moves into dir for the test so default-location lookup is isolated.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

func TestLoad_MissingDefaultFile(t *testing.T) {
	chdir(t, t.TempDir())

	t.Run("required policies fail", func(t *testing.T) {
		for name, p := range map[string]Policy{"pipeline": Pipeline, "compat": Compat} {
			_, err := Load("", p, nil)
			require.Error(t, err, name)
			assert.True(t, gtErrors.Is(err, gtErrors.ErrConfigNotFound), name)
		}
	})

	t.Run("optional policies fall back to defaults with a notice", func(t *testing.T) {
		for name, p := range map[string]Policy{"start": Start, "inspect": Inspect} {
			var notice bytes.Buffer
			cfg, err := Load("", p, &notice)
			require.NoError(t, err, name)
			assert.NotNil(t, cfg, name)
			assert.Contains(t, notice.String(), "No configuration file found", name)
		}
	})

	t.Run("nil notice writer is allowed", func(t *testing.T) {
		_, err := Load("", Inspect, nil)
		assert.NoError(t, err)
	})
}

func TestLoad_ExplicitMissingFileAlwaysFails(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.yml")
	for name, p := range map[string]Policy{"start": Start, "inspect": Inspect} {
		_, err := Load(missing, p, nil)
		require.Error(t, err, name)
	}
}

func TestLoad_BrokenYAMLIsNeverReplacedByDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "version: [\n")
	chdir(t, dir)

	_, err := Load("", Inspect, nil)
	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrConfigInvalid))
}

func TestLoad_Validation(t *testing.T) {
	dir := t.TempDir()
	valid := writeFile(t, dir, validConfig)
	invalid := writeFile(t, t.TempDir(), invalidConfig)

	t.Run("pipeline checks paths", func(t *testing.T) {
		_, err := Load(valid, Pipeline, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "features-path does not exist")
	})

	t.Run("start validates but ignores paths", func(t *testing.T) {
		cfg, err := Load(valid, Start, nil)
		require.NoError(t, err)
		assert.Equal(t, "app:1", cfg.AppConfig.DockerImage)

		_, err = Load(invalid, Start, nil)
		require.Error(t, err)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrConfigInvalid))
	})

	t.Run("compat and inspect never validate", func(t *testing.T) {
		for name, p := range map[string]Policy{"compat": Compat, "inspect": Inspect} {
			_, err := Load(invalid, p, nil)
			assert.NoError(t, err, name)
		}
	})
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	path := writeFile(t, t.TempDir(), validConfig+"observability:\n  log-level: loud\n")

	_, err := Load(path, Inspect, nil)

	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrConfigInvalid))
	assert.Contains(t, err.Error(), "observability.log-level")
}
