// Package stablekarate reproduces the legacy DIA component tool's launch_tests
// step: it runs the test-launcher-back (Karate) STABLE image on the host
// network against the already-running app and mocks, mounting the feature files
// and writing the HTML report to the host, and can open that report in a browser.
package stablekarate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"go.uber.org/zap"

	"github.com/oswaldo-montano/gtool/internal/infra/docker"
	gtErrors "github.com/oswaldo-montano/gtool/pkg/errors"
)

const (
	// defaultImage is the private test launcher image, referenced by its full
	// registry path so the locally present STABLE image resolves without a pull.
	defaultImage = "europe-southwest1-docker.pkg.dev/dia-com-cicd-pro/es-dia-ecom/test-launcher-back:STABLE"

	pubsubEmulatorHost = "127.0.0.1:9085"

	featuresTarget = "/app/features"
	reportsTarget  = "/app/target/karate-reports"

	defaultFeaturesPath = "test/component/features"
	defaultReportsPath  = "test/component/reports"

	summaryFile = "karate-summary.html"
)

// DockerClient is the subset of *docker.Client used by the runner.
type DockerClient interface {
	EnsureImage(ctx context.Context, image string) error
	CreateContainer(ctx context.Context, cfg *docker.ContainerConfig) (string, error)
	StartContainer(ctx context.Context, id string) error
	WaitForContainer(ctx context.Context, id string, condition container.WaitCondition) error
	InspectContainer(ctx context.Context, id string) (*dockertypes.ContainerJSON, error)
	GetContainerLogs(ctx context.Context, id string, tail int) (string, error)
	RemoveContainer(ctx context.Context, id string, force bool) error
}

// Options configures a Karate run.
type Options struct {
	Image        string // defaults to the STABLE test-launcher-back image
	Tags         string
	UrlsToBlock  string
	FeaturesPath string // defaults to test/component/features
	ReportsPath  string // defaults to test/component/reports
	Open         bool   // open the HTML report when the run finishes
}

// Result is the outcome of a Karate run.
type Result struct {
	Passed     bool
	ExitCode   int
	ReportPath string // path to the HTML summary, if generated
	Duration   time.Duration
}

// Runner runs the Karate launcher container.
type Runner struct {
	docker DockerClient
	logger *zap.Logger
	opener func(path string) error
}

// New builds a Runner.
func New(d DockerClient, logger *zap.Logger) *Runner {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Runner{docker: d, logger: logger, opener: openInBrowser}
}

// Run executes the Karate suite and returns the result. It assumes the mocks
// and app are already running (like the legacy "component e").
func (r *Runner) Run(ctx context.Context, opts Options) (*Result, error) {
	image := opts.Image
	if image == "" {
		image = defaultImage
	}
	featuresPath := orDefault(opts.FeaturesPath, defaultFeaturesPath)
	reportsPath := orDefault(opts.ReportsPath, defaultReportsPath)

	featuresAbs, err := filepath.Abs(featuresPath)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "invalid features path")
	}
	if info, statErr := os.Stat(featuresAbs); statErr != nil || !info.IsDir() {
		return nil, gtErrors.New(gtErrors.ErrTestFailed,
			fmt.Sprintf("features directory not found: %s", featuresAbs))
	}

	reportsAbs, err := filepath.Abs(reportsPath)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "invalid reports path")
	}

	if err := r.docker.EnsureImage(ctx, image); err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "failed to ensure test launcher image")
	}

	// Reset the reports dir, mirroring the legacy "rm -rf && mkdir -p". The
	// launcher runs as root, so a previous run leaves root-owned files our user
	// can't delete; if removal fails, chown them back (via a root container) and
	// retry, replacing the legacy "sudo rm -rf".
	if err := os.RemoveAll(reportsAbs); err != nil {
		r.chownToUser(ctx, image, reportsAbs)
		if err := os.RemoveAll(reportsAbs); err != nil {
			r.logger.Warn("could not clean reports dir", zap.String("path", reportsAbs), zap.Error(err))
		}
	}
	if err := os.MkdirAll(reportsAbs, 0o755); err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "failed to create reports directory")
	}

	containerID, err := r.docker.CreateContainer(ctx, &docker.ContainerConfig{
		Image: image,
		Name:  fmt.Sprintf("gtool-karate-%d", time.Now().Unix()),
		Env: []string{
			"PUBSUB_EMULATOR_HOST=" + pubsubEmulatorHost,
			"TAGS=" + opts.Tags,
			"URLS_TO_BLOCK=" + opts.UrlsToBlock,
		},
		Mounts: []docker.Mount{
			{Type: "bind", Source: featuresAbs, Target: featuresTarget, ReadOnly: true},
			{Type: "bind", Source: reportsAbs, Target: reportsTarget},
		},
		NetworkMode: "host",
		Init:        true,
		Labels:      map[string]string{"managed-by": "gtool", "gtool-role": "karate"},
	})
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "failed to create test container")
	}
	defer func() {
		if rmErr := r.docker.RemoveContainer(context.Background(), containerID, true); rmErr != nil {
			r.logger.Warn("failed to remove test container", zap.Error(rmErr))
		}
	}()

	fmt.Printf("🥋 Running Karate tests...\n")
	start := time.Now()
	if err := r.docker.StartContainer(ctx, containerID); err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "failed to start test container")
	}
	if err := r.docker.WaitForContainer(ctx, containerID, container.WaitConditionNotRunning); err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrTestFailed, "failed while waiting for tests to finish")
	}
	duration := time.Since(start)

	exitCode := 0
	if inspect, err := r.docker.InspectContainer(ctx, containerID); err == nil && inspect.State != nil {
		exitCode = inspect.State.ExitCode
	}
	if logs, err := r.docker.GetContainerLogs(ctx, containerID, 2000); err == nil {
		fmt.Println(logs)
	}

	result := &Result{Passed: exitCode == 0, ExitCode: exitCode, Duration: duration}

	// The launcher wrote the report as root; hand ownership back to the invoking
	// user so it can be read/opened (e.g. by a sandboxed browser) and cleaned up.
	r.chownToUser(ctx, image, reportsAbs)

	summary := filepath.Join(reportsAbs, summaryFile)
	if _, statErr := os.Stat(summary); statErr == nil {
		result.ReportPath = summary
		fmt.Printf("📊 Report: %s\n", summary)
		if opts.Open {
			if err := r.opener(summary); err != nil {
				r.logger.Warn("could not open report", zap.Error(err))
			}
		}
	}

	return result, nil
}

// chownToUser hands ownership of the reports tree back to the invoking user by
// running chown inside a short-lived root container (the launcher writes the
// reports as root). Best-effort: failures are logged, not fatal.
func (r *Runner) chownToUser(ctx context.Context, image, reportsAbs string) {
	uid, gid := os.Getuid(), os.Getgid()
	if uid < 0 || gid < 0 {
		return // not a Unix host
	}

	id, err := r.docker.CreateContainer(ctx, &docker.ContainerConfig{
		Image:      image,
		Entrypoint: []string{"chown"},
		Cmd:        []string{"-R", fmt.Sprintf("%d:%d", uid, gid), reportsTarget},
		Mounts:     []docker.Mount{{Type: "bind", Source: reportsAbs, Target: reportsTarget}},
		Labels:     map[string]string{"managed-by": "gtool", "gtool-role": "chown"},
	})
	if err != nil {
		r.logger.Warn("could not create chown container", zap.Error(err))
		return
	}
	defer func() { _ = r.docker.RemoveContainer(context.Background(), id, true) }()

	if err := r.docker.StartContainer(ctx, id); err != nil {
		r.logger.Warn("could not start chown container", zap.Error(err))
		return
	}
	if err := r.docker.WaitForContainer(ctx, id, container.WaitConditionNotRunning); err != nil {
		r.logger.Warn("chown container did not finish cleanly", zap.Error(err))
	}
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// openInBrowser opens a file/URL with the platform's default handler.
func openInBrowser(path string) error {
	cmd := exec.Command("xdg-open", path)
	return cmd.Start()
}
