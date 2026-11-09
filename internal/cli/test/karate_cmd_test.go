package test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/oswaldom-code/gtool/internal/plugin"
	"github.com/oswaldom-code/gtool/pkg/config"
)

type fakeKarate struct {
	result *plugin.TestResult
	err    error
	got    *config.Config
	image  string
}

func (f *fakeKarate) Run(_ context.Context, cfg *config.Config) (*plugin.TestResult, error) {
	f.got = cfg
	return f.result, f.err
}

func karateCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := newKarateCmd()
	t.Cleanup(func() {
		karateTags, karateUrlsToBlock, karateFeatures, karateReports, karateImage = "", "", "", "", ""
		karateNoOpen, karateStable = false, false
	})
	return cmd
}

func injectKarate(t *testing.T, fk *fakeKarate) {
	t.Helper()
	orig := newKarateRunner
	t.Cleanup(func() { newKarateRunner = orig })
	newKarateRunner = func(image string, _ *zap.Logger) (karateRunner, func() error, error) {
		fk.image = image
		return fk, func() error { return nil }, nil
	}
}

func writeKarateConfig(t *testing.T, postgresPort string) string {
	t.Helper()
	dir := t.TempDir()
	features := filepath.Join(dir, "features")
	require.NoError(t, os.MkdirAll(features, 0o755))
	path := filepath.Join(dir, "component-config.yml")
	require.NoError(t, os.WriteFile(path, []byte("version: v1\n"+
		"app-technology: generic\n"+
		"app-config:\n  port: 8080\n  host-port: 18080\n"+
		"test-launcher: test-launcher-back\n"+
		"test-config:\n  features-path: "+features+"\n  tags: \"@all\"\n"+
		"third-party:\n  mocks: [postgresql]\n  mock-config:\n    postgresql:\n      port: "+postgresPort+"\n"), 0o600))
	cfgFile = &path
	t.Cleanup(func() { cfgFile = nil })
	return dir
}

func TestRunKarate_NativeUsesConfig(t *testing.T) {
	cmd := karateCmd(t)
	writeKarateConfig(t, "15432")
	fk := &fakeKarate{result: &plugin.TestResult{Total: 2, Passed: 2}}
	injectKarate(t, fk)

	require.NoError(t, runKarate(cmd, nil))

	require.NotNil(t, fk.got)
	assert.Equal(t, "@all", fk.got.TestConfig.Tags)
	mc := fk.got.ThirdParty.MockConfig["postgresql"].(map[string]interface{})
	assert.EqualValues(t, 15432, mc["port"])
}

func TestRunKarate_FlagsOverrideConfig(t *testing.T) {
	cmd := karateCmd(t)
	dir := writeKarateConfig(t, "15432")
	fk := &fakeKarate{result: &plugin.TestResult{Total: 1, Passed: 1}}
	injectKarate(t, fk)
	reports := filepath.Join(dir, "out")
	require.NoError(t, cmd.Flags().Set("tags", "@smoke"))
	require.NoError(t, cmd.Flags().Set("reports", reports))
	require.NoError(t, cmd.Flags().Set("image", "custom:1"))

	require.NoError(t, runKarate(cmd, nil))

	assert.Equal(t, "@smoke", fk.got.TestConfig.Tags)
	assert.Equal(t, reports, fk.got.TestConfig.ReportsPath)
	assert.Equal(t, "custom:1", fk.image)
}

func TestRunKarate_FailedScenariosFail(t *testing.T) {
	cmd := karateCmd(t)
	writeKarateConfig(t, "15432")
	injectKarate(t, &fakeKarate{result: &plugin.TestResult{Total: 3, Passed: 1, Failed: 2}})

	err := runKarate(cmd, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 of 3 failed")
}

func TestRunKarate_RunnerErrorFails(t *testing.T) {
	cmd := karateCmd(t)
	writeKarateConfig(t, "15432")
	injectKarate(t, &fakeKarate{err: assert.AnError})

	err := runKarate(cmd, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "test run failed")
}

func TestRunKarate_AutoPortRejected(t *testing.T) {
	cmd := karateCmd(t)
	writeKarateConfig(t, "auto")
	fk := &fakeKarate{}
	injectKarate(t, fk)

	err := runKarate(cmd, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "only supported by single-invocation pipelines")
	assert.Nil(t, fk.got)
}

func TestRunKarate_NoConfigFails(t *testing.T) {
	cmd := karateCmd(t)
	cfgFile = nil
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(t.TempDir()))
	t.Cleanup(func() { _ = os.Chdir(orig) })
	fk := &fakeKarate{}
	injectKarate(t, fk)

	err = runKarate(cmd, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no configuration file found")
	assert.Nil(t, fk.got)
}

func TestRunKarate_StableOnlyFlagsNeedStable(t *testing.T) {
	for _, flag := range []string{"no-open", "urls-to-block"} {
		t.Run(flag, func(t *testing.T) {
			cmd := karateCmd(t)
			writeKarateConfig(t, "15432")
			fk := &fakeKarate{}
			injectKarate(t, fk)
			value := "true"
			if flag == "urls-to-block" {
				value = "http://x"
			}
			require.NoError(t, cmd.Flags().Set(flag, value))

			err := runKarate(cmd, nil)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "--"+flag+" only applies with --stable")
			assert.Nil(t, fk.got)
		})
	}
}
