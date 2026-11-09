package generate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreConfig "github.com/oswaldom-code/gtool/internal/core/config"
	"github.com/oswaldom-code/gtool/internal/core/ports"
)

// generateIn runs "generate config" into dir and returns the written path.
func generateIn(t *testing.T, dir string) string {
	t.Helper()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		_ = os.Chdir(orig)
		outputFile, force = "component-config.yml", false
	})
	outputFile = "component-config.yml"
	require.NoError(t, runGenerateConfig(nil, nil))
	return filepath.Join(dir, outputFile)
}

func TestGenerateConfig_ValidatesInFreshDirectory(t *testing.T) {
	dir := t.TempDir()
	path := generateIn(t, dir)

	cfg, err := coreConfig.LoadConfig(path)
	require.NoError(t, err)
	assert.NoError(t, coreConfig.NewValidator().Validate(cfg), "generated config must pass gtool config validate")

	for _, d := range scaffoldDirs {
		assert.DirExists(t, filepath.Join(dir, d))
	}
}

func TestGenerateConfig_ResolvesAsAPipelineConfig(t *testing.T) {
	cfg, err := coreConfig.LoadConfig(generateIn(t, t.TempDir()))
	require.NoError(t, err)

	_, err = ports.Resolve(cfg, ports.FreePortAllocator())
	require.NoError(t, err, "placeholders in the template must resolve")
	assert.Equal(t, cfg.ThirdParty.MockConfig["postgresql"].(map[string]interface{})["port"], cfg.AppConfig.Environment["DB_PORT"])
}

func TestGenerateConfig_RefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	generateIn(t, dir)

	require.NoError(t, os.Chdir(dir))
	assert.Error(t, runGenerateConfig(nil, nil))

	force = true
	assert.NoError(t, runGenerateConfig(nil, nil))
}
