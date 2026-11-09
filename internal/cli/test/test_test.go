package test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/oswaldom-code/gtool/internal/core/orchestrator"
	"github.com/oswaldom-code/gtool/internal/plugin"
	"github.com/oswaldom-code/gtool/pkg/config"
)

type fakePipeline struct {
	result *orchestrator.Result
	err    error
	called bool
}

func (f *fakePipeline) Run(_ context.Context) (*orchestrator.Result, error) {
	f.called = true
	return f.result, f.err
}

func injectPipeline(t *testing.T, p pipeline) {
	t.Helper()
	orig := newPipeline
	t.Cleanup(func() {
		newPipeline = orig
		cfgFile = nil
	})
	newPipeline = func(_ *config.Config, _ *zap.Logger) (*pipelineDeps, error) {
		return &pipelineDeps{pipeline: p, close: func() error { return nil }}, nil
	}
}

func TestNewTestCmd(t *testing.T) {
	cmd := NewTestCmd(nil)
	assert.Equal(t, "test", cmd.Name())
	assert.NotNil(t, cmd.RunE)
}

// useValidConfig points cfgFile at a minimal config that passes validation.
func useValidConfig(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "component-config.yml")
	require.NoError(t, os.WriteFile(path, []byte("version: v1\n"+
		"app-technology: generic\n"+
		"test-launcher: test-launcher-back\n"), 0o600))
	cfgFile = &path
	t.Cleanup(func() { cfgFile = nil })
}

func TestRunTest_NoConfigFails(t *testing.T) {
	cfgFile = nil
	dir := t.TempDir()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(orig) })
	fp := &fakePipeline{}
	injectPipeline(t, fp)

	err = runTest(nil, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no configuration file found")
	assert.False(t, fp.called, "a missing config must never run (and pass) an empty pipeline")
}

func TestRunTest_InvalidConfigFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "component-config.yml")
	require.NoError(t, os.WriteFile(path, []byte("version: v9\n"), 0o600))
	cfgFile = &path
	fp := &fakePipeline{}
	injectPipeline(t, fp)

	err := runTest(nil, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ERR_CONFIG_INVALID")
	assert.False(t, fp.called)
}

func TestRunTest_Success(t *testing.T) {
	useValidConfig(t)
	fp := &fakePipeline{result: &orchestrator.Result{
		MocksStarted: true,
		AppStarted:   true,
		Test:         &plugin.TestResult{Total: 3, Passed: 3},
	}}
	injectPipeline(t, fp)

	require.NoError(t, runTest(nil, nil))
	assert.True(t, fp.called)
}

func TestRunTest_PipelineFailure(t *testing.T) {
	useValidConfig(t)
	injectPipeline(t, &fakePipeline{
		result: &orchestrator.Result{MocksStarted: true},
		err:    errors.New("boom"),
	})

	err := runTest(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline failed")
}

func TestRunTest_FactoryFailure(t *testing.T) {
	useValidConfig(t)
	orig := newPipeline
	t.Cleanup(func() { newPipeline = orig })
	newPipeline = func(_ *config.Config, _ *zap.Logger) (*pipelineDeps, error) {
		return nil, errors.New("docker down")
	}

	err := runTest(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "docker down")
}

func TestPrintResult_NilSafe(t *testing.T) {
	printResult(nil) // must not panic
}

func writePortsConfig(t *testing.T, envValue string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "component-config.yml")
	content := `version: v1
app-technology: generic
test-launcher: test-launcher-back
app-config:
  docker-image: app:1
  port: 8080
  host-port: auto
  environment:
    DB_PORT: "` + envValue + `"
third-party:
  mocks: [postgresql]
  mock-config:
    postgresql:
      port: auto
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	cfgFile = &path
}

func TestRunTest_ResolvesDynamicPorts(t *testing.T) {
	writePortsConfig(t, "${mocks.postgresql.port}")
	fp := &fakePipeline{result: &orchestrator.Result{}}
	orig := newPipeline
	t.Cleanup(func() { newPipeline = orig; cfgFile = nil })
	var got *config.Config
	newPipeline = func(cfg *config.Config, _ *zap.Logger) (*pipelineDeps, error) {
		got = cfg
		return &pipelineDeps{pipeline: fp, close: func() error { return nil }}, nil
	}

	require.NoError(t, runTest(nil, nil))

	pgPort := got.ThirdParty.MockConfig["postgresql"].(map[string]interface{})["port"].(string)
	_, err := strconv.Atoi(pgPort)
	assert.NoError(t, err, "auto must be replaced by a numeric port")
	assert.Equal(t, pgPort, got.AppConfig.Environment["DB_PORT"])
	_, err = strconv.Atoi(got.AppConfig.HostPort)
	assert.NoError(t, err)
}

func TestRunTest_UnresolvedPlaceholder(t *testing.T) {
	writePortsConfig(t, "${mocks.redis.port}")
	fp := &fakePipeline{}
	injectPipeline(t, fp)

	err := runTest(nil, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve ports")
	assert.False(t, fp.called, "no phase may start with an unresolved address")
}

func TestRunTest_StableOnlyFlagsNeedStable(t *testing.T) {
	cmd := NewTestCmd(nil)
	useValidConfig(t)
	t.Cleanup(func() { stableMode, stableNoOpen = false, false })
	require.NoError(t, cmd.Flags().Set("no-open", "true"))
	fp := &fakePipeline{}
	injectPipeline(t, fp)

	err := runTest(cmd, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--no-open only applies with --stable")
	assert.False(t, fp.called)
}

func TestRunTest_TagsOverrideConfig(t *testing.T) {
	cmd := NewTestCmd(nil)
	useValidConfig(t)
	t.Cleanup(func() { stableTags = "" })
	require.NoError(t, cmd.Flags().Set("tags", "@smoke"))
	var got *config.Config
	orig := newPipeline
	t.Cleanup(func() { newPipeline = orig })
	newPipeline = func(cfg *config.Config, _ *zap.Logger) (*pipelineDeps, error) {
		got = cfg
		return &pipelineDeps{pipeline: &fakePipeline{result: &orchestrator.Result{}}, close: func() error { return nil }}, nil
	}

	require.NoError(t, runTest(cmd, nil))
	assert.Equal(t, "@smoke", got.TestConfig.Tags)
}
