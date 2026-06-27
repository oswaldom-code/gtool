package test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/oswaldo-montano/gtool/internal/core/orchestrator"
	"github.com/oswaldo-montano/gtool/internal/plugin"
	"github.com/oswaldo-montano/gtool/pkg/config"
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
