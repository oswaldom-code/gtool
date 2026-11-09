package test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"

	"github.com/oswaldom-code/gtool/internal/core/ports"
	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/internal/plugin"
	"github.com/oswaldom-code/gtool/internal/plugin/services/svcconfig"
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
	EnsureImage(ctx context.Context, image string) error
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

// pubsubEmulatorHost returns the address the launcher's Pub/Sub helpers must
// use when the pubsub mock is enabled, or "" otherwise. The launcher runs on
// the host network, so the emulator is reached on its host-published port.
func pubsubEmulatorHost(cfg *config.Config) string {
	if port, ok := ports.Values(cfg)["mocks.pubsub.port"]; ok {
		return "127.0.0.1:" + port
	}
	return ""
}

// runEnv exposes this run's app URL and mock ports (GTOOL_APP_URL,
// GTOOL_<SVC>_PORT, ...) so features and launcher helpers need no hardcoded
// ports. Sorted for a deterministic container spec.
// connectionKeys are the mock-config settings, besides ports, that launcher
// helpers need to connect (e.g. karate-config.js builds its Postgres client).
var connectionKeys = []string{"user", "username", "password", "database"}

// connectionEnv exports GTOOL_<SVC>_<KEY> for the connection settings set on
// each enabled mock.
func connectionEnv(cfg *config.Config, vars map[string]string) {
	for _, svc := range cfg.ThirdParty.Mocks {
		mc, _ := cfg.ThirdParty.MockConfig[svc].(map[string]interface{})
		for _, key := range connectionKeys {
			if v := svcconfig.Port(mc, key, ""); v != "" {
				vars["GTOOL_"+strings.ToUpper(svc)+"_"+strings.ToUpper(key)] = v
			}
		}
	}
}

func runEnv(cfg *config.Config) []string {
	vars := ports.TestEnv(ports.Values(cfg))
	connectionEnv(cfg, vars)
	out := make([]string, 0, len(vars))
	for k, v := range vars {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// chownToUser hands ownership of the reports tree back to the invoking user by
// running chown inside a short-lived root container (the launcher writes the
// reports as root). Best-effort: failures are logged, not fatal.
func (k *KarateRunner) chownToUser(ctx context.Context, reportsAbs string) {
	uid, gid := os.Getuid(), os.Getgid()
	if uid < 0 || gid < 0 {
		return // not a Unix host
	}

	id, err := k.docker.CreateContainer(ctx, &docker.ContainerConfig{
		Image:      k.image,
		Entrypoint: []string{"chown"},
		Cmd:        []string{"-R", fmt.Sprintf("%d:%d", uid, gid), reportsTarget},
		Mounts:     []docker.Mount{{Type: "bind", Source: reportsAbs, Target: reportsTarget}},
		Labels:     map[string]string{"managed-by": "gtool", "gtool-role": "chown"},
	})
	if err != nil {
		k.logger.Warn("could not create chown container", zap.Error(err))
		return
	}
	defer func() { _ = k.docker.RemoveContainer(context.Background(), id, true) }()

	if err := k.docker.StartContainer(ctx, id); err != nil {
		k.logger.Warn("could not start chown container", zap.Error(err))
		return
	}
	if err := k.docker.WaitForContainer(ctx, id, container.WaitConditionNotRunning); err != nil {
		k.logger.Warn("chown container did not finish cleanly", zap.Error(err))
	}
}

// Run mounts the feature files into the launcher container, runs it on the host
// network (so the tests can reach the app and mocks on localhost), and reports
// the outcome from the container exit code.
func (k *KarateRunner) Run(ctx context.Context, cfg *config.Config) (*plugin.TestResult, error) {
	tc := cfg.TestConfig
	if tc.FeaturesPath == "" {
		return nil, gtErrors.New(gtErrors.ErrConfigInvalid,
			"test-config.features-path is required: without features nothing would be tested")
	}

	mounts, err := k.buildMounts(tc)
	if err != nil {
		return nil, err
	}

	var env []string
	if tc.Tags != "" {
		env = append(env, "TAGS="+tc.Tags)
	}
	if host := pubsubEmulatorHost(cfg); host != "" {
		env = append(env, "PUBSUB_EMULATOR_HOST="+host)
	}
	env = append(env, runEnv(cfg)...)

	k.logger.Info("ensuring test launcher image", zap.String("image", k.image))
	if err := k.docker.EnsureImage(ctx, k.image); err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "failed to get test launcher image")
	}

	containerID, err := k.docker.CreateContainer(ctx, &docker.ContainerConfig{
		Image:       k.image,
		Name:        fmt.Sprintf("gtool-test-%d", time.Now().Unix()),
		Env:         env,
		Mounts:      mounts,
		NetworkMode: "host",
		User:        docker.HostUser(),
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

	exitCode, err := docker.ExitCode(k.docker.InspectContainer(ctx, containerID))
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "could not determine the test result")
	}

	logs, _ := k.docker.GetContainerLogs(ctx, containerID, 1000)
	k.logger.Info("tests finished", zap.Int("exit-code", exitCode))

	if tc.ReportsPath != "" {
		if reportsAbs, err := filepath.Abs(tc.ReportsPath); err == nil {
			k.chownToUser(ctx, reportsAbs)
		}
	}

	summary, err := readSummary(tc.ReportsPath)
	if err != nil {
		k.logger.Warn("no Karate summary; the result is based on the exit code only", zap.Error(err))
	}
	result := buildResult(exitCode, time.Since(start), tc.ReportsPath, logs, summary)
	if summary != nil && result.Total == 0 {
		return nil, gtErrors.New(gtErrors.ErrTestFailed,
			"no scenarios ran: check test-config.features-path and tags")
	}
	return result, nil
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

const summaryFile = "karate-summary-json.txt"

// karateSummary is the part of Karate's karate-summary-json.txt gtool reads.
type karateSummary struct {
	ScenariosPassed int `json:"scenariosPassed"`
	ScenariosFailed int `json:"scenariosfailed"`
	Features        []struct {
		Name         string `json:"name"`
		RelativePath string `json:"relativePath"`
		FailedCount  int    `json:"failedCount"`
	} `json:"featureSummary"`
}

// readSummary reads the Karate summary from the reports directory. It returns
// nil and an error when reports are not configured or the file is missing.
func readSummary(reportsPath string) (*karateSummary, error) {
	if reportsPath == "" {
		return nil, gtErrors.New(gtErrors.ErrTestFailed, "test-config.reports-path is not set")
	}
	data, err := os.ReadFile(filepath.Join(reportsPath, summaryFile))
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "failed to read the Karate summary")
	}
	var s karateSummary
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "failed to parse the Karate summary")
	}
	return &s, nil
}

// buildResult turns the run into a TestResult: scenario counts from the Karate
// summary when available, otherwise one pass/fail entry from the exit code. A
// non-zero exit code is always a failure, even when no scenario failed (e.g. a
// compilation error in the launcher).
func buildResult(exitCode int, duration time.Duration, reportsPath, logs string, summary *karateSummary) *plugin.TestResult {
	result := &plugin.TestResult{Duration: duration, ReportURL: reportsPath}

	if summary != nil {
		result.Total = summary.ScenariosPassed + summary.ScenariosFailed
		result.Passed = summary.ScenariosPassed
		result.Failed = summary.ScenariosFailed
		for _, f := range summary.Features {
			if f.FailedCount > 0 {
				result.Failures = append(result.Failures, plugin.TestFailure{
					Name:    f.Name,
					Feature: f.RelativePath,
					Message: fmt.Sprintf("%d scenario(s) failed", f.FailedCount),
				})
			}
		}
	} else if exitCode == 0 {
		result.Total, result.Passed = 1, 1
	}

	if exitCode != 0 && result.Failed == 0 {
		result.Failed = 1
		result.Total = result.Passed + result.Failed
	}
	if exitCode != 0 {
		result.Failures = append(result.Failures, plugin.TestFailure{
			Name:    "karate suite",
			Message: fmt.Sprintf("test launcher exited with code %d", exitCode),
			Stack:   lastLines(logs, 20),
		})
	}
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
