package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oswaldo-montano/gtool/internal/plugin"
	"github.com/oswaldo-montano/gtool/pkg/config"
	gtErrors "github.com/oswaldo-montano/gtool/pkg/errors"
)

type fakeMocks struct {
	startErr   error
	startCalls int
	stopCalls  int
}

func (f *fakeMocks) StartAll(_ context.Context, _ map[string]map[string]interface{}) error {
	f.startCalls++
	return f.startErr
}
func (f *fakeMocks) StopAll(_ context.Context) error {
	f.stopCalls++
	return nil
}

type fakeApp struct {
	startErr   error
	startCalls int
	stopCalls  int
}

func (f *fakeApp) Start(_ context.Context, _ *plugin.AppConfig) error {
	f.startCalls++
	return f.startErr
}
func (f *fakeApp) Stop(_ context.Context) error {
	f.stopCalls++
	return nil
}

type fakeTests struct {
	err    error
	called bool
}

func (f *fakeTests) Run(_ context.Context, _ *config.Config) (*plugin.TestResult, error) {
	f.called = true
	return &plugin.TestResult{Total: 1, Passed: 1}, f.err
}

// cfgWith builds a config with the given mocks and docker image.
func cfgWith(mocks []string, image string) *config.Config {
	cfg := config.DefaultConfig()
	cfg.ThirdParty.Mocks = mocks
	cfg.AppConfig.DockerImage = image
	return cfg
}

func TestOrchestrator_Run_FullPipeline(t *testing.T) {
	mocks := &fakeMocks{}
	app := &fakeApp{}
	tests := &fakeTests{}
	o := NewOrchestrator(cfgWith([]string{"postgresql"}, "myapp:latest"), mocks, app, tests, nil)

	res, err := o.Run(context.Background())

	require.NoError(t, err)
	assert.True(t, res.MocksStarted)
	assert.True(t, res.AppStarted)
	assert.True(t, tests.called)
	// Everything started must be torn down.
	assert.Equal(t, 1, mocks.startCalls)
	assert.Equal(t, 1, mocks.stopCalls)
	assert.Equal(t, 1, app.startCalls)
	assert.Equal(t, 1, app.stopCalls)
}

func TestOrchestrator_Run_NoMocksNoApp(t *testing.T) {
	mocks := &fakeMocks{}
	app := &fakeApp{}
	tests := &fakeTests{}
	o := NewOrchestrator(cfgWith(nil, ""), mocks, app, tests, nil)

	res, err := o.Run(context.Background())

	require.NoError(t, err)
	assert.False(t, res.MocksStarted)
	assert.False(t, res.AppStarted)
	assert.True(t, tests.called, "tests run even with no mocks/app")
	assert.Equal(t, 0, mocks.startCalls)
	assert.Equal(t, 0, app.startCalls)
}

func TestOrchestrator_Run_MockFailure(t *testing.T) {
	mocks := &fakeMocks{startErr: errors.New("boom")}
	app := &fakeApp{}
	tests := &fakeTests{}
	o := NewOrchestrator(cfgWith([]string{"postgresql"}, "myapp:latest"), mocks, app, tests, nil)

	res, err := o.Run(context.Background())

	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrServiceFailed))
	assert.False(t, res.AppStarted)
	assert.False(t, tests.called, "tests must not run if mocks failed")
	assert.Equal(t, 0, app.startCalls, "app must not start if mocks failed")
	// Mocks were not marked started, so StopAll is not called.
	assert.Equal(t, 0, mocks.stopCalls)
}

func TestOrchestrator_Run_AppFailure_CleansUpMocks(t *testing.T) {
	mocks := &fakeMocks{}
	app := &fakeApp{startErr: errors.New("boom")}
	tests := &fakeTests{}
	o := NewOrchestrator(cfgWith([]string{"postgresql"}, "myapp:latest"), mocks, app, tests, nil)

	res, err := o.Run(context.Background())

	require.Error(t, err)
	assert.False(t, tests.called, "tests must not run if app failed")
	assert.True(t, res.MocksStarted)
	assert.Equal(t, 1, mocks.stopCalls, "mocks started earlier must be cleaned up")
	assert.Equal(t, 0, app.stopCalls, "app never started, so not stopped")
}

func TestOrchestrator_Run_TestFailure_CleansUpEverything(t *testing.T) {
	mocks := &fakeMocks{}
	app := &fakeApp{}
	tests := &fakeTests{err: errors.New("tests failed")}
	o := NewOrchestrator(cfgWith([]string{"postgresql"}, "myapp:latest"), mocks, app, tests, nil)

	_, err := o.Run(context.Background())

	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrTestFailed))
	assert.Equal(t, 1, app.stopCalls)
	assert.Equal(t, 1, mocks.stopCalls)
}

func TestStubTestRunner(t *testing.T) {
	res, err := NewStubTestRunner(nil).Run(context.Background(), config.DefaultConfig())

	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestBuildServiceConfigs(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ThirdParty.Mocks = []string{"postgresql", "kafka"}
	cfg.ThirdParty.MockConfig = map[string]interface{}{
		"postgresql": map[string]interface{}{"port": "5432"},
	}

	got := buildServiceConfigs(cfg)

	require.Len(t, got, 2)
	assert.Equal(t, "5432", got["postgresql"]["port"])
	assert.NotNil(t, got["kafka"], "mock without config still gets an empty map")
}
