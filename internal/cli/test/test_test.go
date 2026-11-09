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

func TestRunTest_Success(t *testing.T) {
	cfgFile = nil
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
	cfgFile = nil
	injectPipeline(t, &fakePipeline{
		result: &orchestrator.Result{MocksStarted: true},
		err:    errors.New("boom"),
	})

	err := runTest(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline failed")
}

func TestRunTest_FactoryFailure(t *testing.T) {
	cfgFile = nil
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
