package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"

	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/internal/plugin"
	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
	"go.uber.org/zap"
)

const (
	// DefaultLauncherImage is the Karate backend test launcher image (see
	// test-launcher/). Build it locally with `make test-launcher-image`; override
	// it via NewKarateRunner when needed.
	DefaultLauncherImage = "gtool/test-launcher-back:latest"

	featuresTarget = "/app/features"
	reportsTarget  = "/app/target/karate-reports"
)

// DockerClient is the subset of *docker.Client used by the Karate runner.
type DockerClient interface {
	PullImage(ctx context.Context, image string) error
	CreateContainer(ctx context.Context, config *docker.ContainerConfig) (string, error)
	StartContainer(ctx context.Context, containerID string) error
	WaitForContainer(ctx context.Context, containerID string, condition container.WaitCondition) error
	InspectContainer(ctx context.Context, containerID string) (*dockertypes.ContainerJSON, error)
	GetContainerLogs(ctx context.Context, containerID string, tail int) (string, error)
	RemoveContainer(ctx context.Context, containerID string, force bool) error
}

// KarateRunner executes a Karate backend test suite by running the
// test-launcher-back container against the already-running app and mocks. It
// satisfies the orchestrator's TestRunner interface.
type KarateRunner struct {
	docker DockerClient
	image  string
	logger *zap.Logger
}

// NewKarateRunner creates a Karate runner. An empty image uses DefaultLauncherImage.
func NewKarateRunner(dockerClient DockerClient, image string, logger *zap.Logger) *KarateRunner {
	if image == "" {
		image = DefaultLauncherImage
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &KarateRunner{docker: dockerClient, image: image, logger: logger}
}

// Run mounts the feature files into the launcher container, runs it on the host
// network (so the tests can reach the app and mocks on localhost), and reports
// the outcome from the container exit code.
func (k *KarateRunner) Run(ctx context.Context, cfg *config.Config) (*plugin.TestResult, error) {
	tc := cfg.TestConfig
	if tc.FeaturesPath == "" {
		k.logger.Warn("no test-config.features-path configured; skipping tests")
		return &plugin.TestResult{}, nil
	}

	mounts, err := k.buildMounts(tc)
	if err != nil {
		return nil, err
	}

	var env []string
	if tc.Tags != "" {
		env = append(env, "TAGS="+tc.Tags)
	}

	k.logger.Info("pulling test launcher", zap.String("image", k.image))
	if err := k.docker.PullImage(ctx, k.image); err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "failed to pull test launcher image")
	}

	containerID, err := k.docker.CreateContainer(ctx, &docker.ContainerConfig{
		Image:       k.image,
		Name:        fmt.Sprintf("gtool-test-%d", time.Now().Unix()),
		Env:         env,
		Mounts:      mounts,
		NetworkMode: "host",
		Labels: map[string]string{
			"managed-by": "gtool",
			"gtool-role": "test",
		},
	})
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "failed to create test container")
	}
	defer func() {
		if err := k.docker.RemoveContainer(context.Background(), containerID, true); err != nil {
			k.logger.Warn("failed to remove test container", zap.Error(err))
		}
	}()

	start := time.Now()
	if err := k.docker.StartContainer(ctx, containerID); err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "failed to start test container")
	}

	k.logger.Info("running tests", zap.String("features", tc.FeaturesPath), zap.String("tags", tc.Tags))
	if err := k.docker.WaitForContainer(ctx, containerID, container.WaitConditionNotRunning); err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "failed while waiting for tests to finish")
	}

	exitCode := 0
	if inspect, err := k.docker.InspectContainer(ctx, containerID); err == nil && inspect.State != nil {
		exitCode = inspect.State.ExitCode
	}

	logs, _ := k.docker.GetContainerLogs(ctx, containerID, 1000)
	k.logger.Info("tests finished", zap.Int("exit-code", exitCode))

	return buildResult(exitCode, time.Since(start), tc.ReportsPath, logs), nil
}

// buildMounts builds the bind mounts for features (read-only) and reports.
func (k *KarateRunner) buildMounts(tc config.TestConfig) ([]docker.Mount, error) {
	featuresAbs, err := filepath.Abs(tc.FeaturesPath)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "invalid features-path")
	}
	mounts := []docker.Mount{
		{Type: "bind", Source: featuresAbs, Target: featuresTarget, ReadOnly: true},
	}

	if tc.ReportsPath != "" {
		reportsAbs, err := filepath.Abs(tc.ReportsPath)
		if err != nil {
			return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "invalid reports-path")
		}
		if err := os.MkdirAll(reportsAbs, 0o755); err != nil {
			return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "failed to create reports directory")
		}
		mounts = append(mounts, docker.Mount{Type: "bind", Source: reportsAbs, Target: reportsTarget})
	}

	return mounts, nil
}

// buildResult turns the container exit code into a TestResult. A zero exit code
// is a pass; any other value is a failure carrying the tail of the logs.
func buildResult(exitCode int, duration time.Duration, reportsPath, logs string) *plugin.TestResult {
	result := &plugin.TestResult{
		Total:     1,
		Duration:  duration,
		ReportURL: reportsPath,
	}
	if exitCode == 0 {
		result.Passed = 1
		return result
	}

	result.Failed = 1
	result.Failures = []plugin.TestFailure{{
		Name:    "karate suite",
		Message: fmt.Sprintf("test launcher exited with code %d", exitCode),
		Stack:   lastLines(logs, 20),
	}}
	return result
}

// lastLines returns at most n trailing lines of s.
func lastLines(s string, n int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
