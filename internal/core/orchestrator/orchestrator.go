package orchestrator

import (
	"context"

	"github.com/oswaldo-montano/gtool/internal/plugin"
	"github.com/oswaldo-montano/gtool/pkg/config"
	gtErrors "github.com/oswaldo-montano/gtool/pkg/errors"
	"go.uber.org/zap"
)

// MockManager starts and stops the mock services. *mock.Manager satisfies it.
type MockManager interface {
	StartAll(ctx context.Context, serviceConfigs map[string]map[string]interface{}) error
	StopAll(ctx context.Context) error
}

// AppManager starts and stops the application under test. *app.DockerManager
// satisfies it.
type AppManager interface {
	Start(ctx context.Context, cfg *plugin.AppConfig) error
	Stop(ctx context.Context) error
}

// TestRunner executes the test suite. Until Phase 4 lands this is a stub.
type TestRunner interface {
	Run(ctx context.Context, cfg *config.Config) (*plugin.TestResult, error)
}

// Result summarizes a pipeline run.
type Result struct {
	MocksStarted bool
	AppStarted   bool
	Test         *plugin.TestResult
}

// Orchestrator coordinates the full component-test pipeline: start mocks, start
// the application, run tests, then tear everything down.
type Orchestrator struct {
	config *config.Config
	mocks  MockManager
	app    AppManager
	tests  TestRunner
	logger *zap.Logger
}

// NewOrchestrator wires the orchestrator with its phase managers.
func NewOrchestrator(cfg *config.Config, mocks MockManager, app AppManager, tests TestRunner, logger *zap.Logger) *Orchestrator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Orchestrator{
		config: cfg,
		mocks:  mocks,
		app:    app,
		tests:  tests,
		logger: logger,
	}
}

// Run executes the pipeline. Whatever was started is always torn down before
// Run returns, even on failure or context cancellation.
func (o *Orchestrator) Run(ctx context.Context) (*Result, error) {
	res := &Result{}

	defer o.cleanup(res)

	// Phase 1: mocks
	serviceConfigs := buildServiceConfigs(o.config)
	if len(serviceConfigs) > 0 {
		o.logger.Info("phase: starting mocks", zap.Int("count", len(serviceConfigs)))
		if err := o.mocks.StartAll(ctx, serviceConfigs); err != nil {
			return res, gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to start mocks")
		}
		res.MocksStarted = true
	}

	// Phase 2: application
	appConfig := buildAppConfig(o.config)
	if appConfig.DockerImage != "" {
		o.logger.Info("phase: starting application", zap.String("image", appConfig.DockerImage))
		if err := o.app.Start(ctx, appConfig); err != nil {
			return res, gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to start application")
		}
		res.AppStarted = true
	}

	// Phase 3: tests
	o.logger.Info("phase: running tests")
	testResult, err := o.tests.Run(ctx, o.config)
	if err != nil {
		return res, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "test execution failed")
	}
	res.Test = testResult

	o.logger.Info("pipeline finished")
	return res, nil
}

// cleanup tears down the application and mocks in reverse order. It uses a fresh
// context so teardown still runs when the pipeline was cancelled, and is
// best-effort: failures are logged, not returned.
func (o *Orchestrator) cleanup(res *Result) {
	cleanupCtx := context.Background()

	if res.AppStarted {
		o.logger.Info("cleanup: stopping application")
		if err := o.app.Stop(cleanupCtx); err != nil {
			o.logger.Error("failed to stop application during cleanup", zap.Error(err))
		}
	}
	if res.MocksStarted {
		o.logger.Info("cleanup: stopping mocks")
		if err := o.mocks.StopAll(cleanupCtx); err != nil {
			o.logger.Error("failed to stop mocks during cleanup", zap.Error(err))
		}
	}
}

// buildServiceConfigs turns the configured mocks into the per-service config map
// the mock manager expects.
func buildServiceConfigs(cfg *config.Config) map[string]map[string]interface{} {
	out := make(map[string]map[string]interface{}, len(cfg.ThirdParty.Mocks))
	for _, name := range cfg.ThirdParty.Mocks {
		m, _ := cfg.ThirdParty.MockConfig[name].(map[string]interface{})
		if m == nil {
			m = map[string]interface{}{}
		}
		out[name] = m
	}
	return out
}

// buildAppConfig maps the file configuration to a plugin.AppConfig.
func buildAppConfig(cfg *config.Config) *plugin.AppConfig {
	return &plugin.AppConfig{
		BinaryName:  cfg.AppConfig.BinaryName,
		BinaryPath:  cfg.AppConfig.BinaryPath,
		DockerImage: cfg.AppConfig.DockerImage,
		Port:        cfg.AppConfig.Port,
		Environment: cfg.AppConfig.Environment,
	}
}
