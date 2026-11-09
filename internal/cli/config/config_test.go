package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

func writeConfig(t *testing.T, content string) *string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "component-config.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return &path
}

const minimal = "version: v1\napp-technology: generic\ntest-launcher: test-launcher-back\n"

func runShow(t *testing.T, path *string, format string) (string, error) {
	t.Helper()
	cmd := newConfigShowCmd(path)
	var out bytes.Buffer
	cmd.SetOut(&out)
	require.NoError(t, cmd.Flags().Set("format", format))
	err := cmd.RunE(cmd, nil)
	return out.String(), err
}

func TestConfigShow_Formats(t *testing.T) {
	path := writeConfig(t, minimal)

	for _, format := range []string{"yml", "yaml"} {
		out, err := runShow(t, path, format)
		require.NoError(t, err, format)
		assert.Contains(t, out, "app-technology: generic", format)
	}

	out, err := runShow(t, path, "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"app-technology": "generic"`)

	_, err = runShow(t, path, "toml")
	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrInvalidArgument))
}

func TestConfigValidate_ReturnsValidationError(t *testing.T) {
	cmd := newConfigValidateCmd(writeConfig(t, "version: v9\n"))

	err := cmd.RunE(cmd, nil)

	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrConfigInvalid))
}

func TestConfigValidate_Valid(t *testing.T) {
	cmd := newConfigValidateCmd(writeConfig(t, minimal))
	assert.NoError(t, cmd.RunE(cmd, nil))
}
