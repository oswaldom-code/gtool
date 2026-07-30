package orchestrator

import (
	"context"

	"github.com/oswaldom-code/gtool/internal/plugin"
	"github.com/oswaldom-code/gtool/pkg/config"
	"go.uber.org/zap"
)

// StubTestRunner is a placeholder TestRunner used until the Phase 4 test
// executors (Karate) are implemented. It runs no tests and reports an empty
// successful result so the rest of the pipeline can be exercised end to end.
type StubTestRunner struct {
	logger *zap.Logger
}

// NewStubTestRunner creates a stub test runner.
func NewStubTestRunner(logger *zap.Logger) *StubTestRunner {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &StubTestRunner{logger: logger}
}

// Run logs that test execution is not yet implemented and returns an empty result.
func (s *StubTestRunner) Run(_ context.Context, _ *config.Config) (*plugin.TestResult, error) {
	s.logger.Warn("test execution is not implemented yet (Phase 4); skipping tests")
	return &plugin.TestResult{}, nil
}
