// Package stablemocks reproduces the legacy DIA "component" tool's
// prepare_mock_environment / stop_mock_environment steps: it launches the
// private third-party STABLE mock images with the exact docker run contract
// (network, ports, mounts, env, container names and log-based readiness) so
// `gtool services up --stable` behaves like `component m`.
//
// This is a compatibility path for the STABLE images while they remain in use;
// gtool's native plugins (public images) stay the default.
package stablemocks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/oswaldo-montano/gtool/internal/infra/docker"
	"github.com/oswaldo-montano/gtool/pkg/config"
	gtErrors "github.com/oswaldo-montano/gtool/pkg/errors"
)

const (
	// mocksArtifactRepo and dockerTag mirror the constants in the legacy
	// component tool (MOCKS_ARTIFACT_REPO / DOCKER_TAG).
	mocksArtifactRepo = "europe-southwest1-docker.pkg.dev/dia-com-cicd-pro/third-party-mocks"
	dockerTag         = "STABLE"

	pubsubMockPort = "9085"

	readyAttempts = 60
	readyInterval = 3 * time.Second
)

// Supported lists the mocks this compatibility launcher can start. The other
// legacy mocks (couchbase, kafka, gcs) are not ported to STABLE mode yet.
var Supported = []string{"postgresql", "pubsub", "mountebank"}

// dockerClient is the subset of *docker.Client the launcher needs (kept small
// so tests can inject a fake).
type dockerClient interface {
	EnsureImage(ctx context.Context, image string) error
	CreateContainer(ctx context.Context, cfg *docker.ContainerConfig) (string, error)
	StartContainer(ctx context.Context, id string) error
	GetContainerLogs(ctx context.Context, id string, tail int) (string, error)
	RemoveContainerByName(ctx context.Context, name string) (bool, error)
}

// Launcher launches and stops the STABLE mock containers.
type Launcher struct {
	docker        dockerClient
	logger        *zap.Logger
	mocksDataPath string
}

// New builds a Launcher resolving the mocks-data directory from the working
// directory, matching the legacy tool's $PWD/test/component/mocks-data.
func New(d dockerClient, logger *zap.Logger) (*Launcher, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to resolve working directory")
	}
	return &Launcher{
		docker:        d,
		logger:        logger,
		mocksDataPath: filepath.Join(wd, "test", "component", "mocks-data"),
	}, nil
}

type launchSpec struct {
	name        string
	image       string
	networkMode string
	ports       map[string]string // container port -> host port
	mounts      []docker.Mount
	env         []string
	readyLogs   []string // every entry must be present in the logs to be ready
}

// Up launches the given services (or all configured mocks) with the STABLE
// contract and waits until each one is ready.
func (l *Launcher) Up(ctx context.Context, services []string, cfg *config.Config) error {
	if len(services) == 0 {
		services = cfg.ThirdParty.Mocks
	}
	if len(services) == 0 {
		return gtErrors.New(gtErrors.ErrInvalidArgument, "no services specified and none configured")
	}

	specs := make([]*launchSpec, 0, len(services))
	for _, name := range services {
		spec, err := l.specFor(name, cfg)
		if err != nil {
			return err
		}
		specs = append(specs, spec)
	}

	for _, spec := range specs {
		fmt.Printf("🚀 Launching %s (STABLE)...\n", spec.name)
		if err := l.launchOne(ctx, spec); err != nil {
			return err
		}
	}

	fmt.Printf("⏳ Waiting for mocks to be ready...\n")
	for _, spec := range specs {
		if err := l.waitReady(ctx, spec); err != nil {
			return err
		}
		fmt.Printf("✅ %s ready\n", spec.name)
	}

	return nil
}

// Down removes the fixed-name STABLE mock containers.
func (l *Launcher) Down(ctx context.Context, services []string) error {
	if len(services) == 0 {
		services = Supported
	}
	for _, name := range services {
		removed, err := l.docker.RemoveContainerByName(ctx, name)
		if err != nil {
			return err
		}
		if removed {
			fmt.Printf("🛑 %s stopped\n", name)
		}
	}
	return nil
}

func (l *Launcher) launchOne(ctx context.Context, spec *launchSpec) error {
	if _, err := l.docker.RemoveContainerByName(ctx, spec.name); err != nil {
		return err
	}
	if err := l.docker.EnsureImage(ctx, spec.image); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed,
			fmt.Sprintf("failed to ensure image for %s", spec.name))
	}

	cc := &docker.ContainerConfig{
		Image:        spec.image,
		Name:         spec.name,
		Env:          spec.env,
		PortBindings: spec.ports,
		Mounts:       spec.mounts,
		NetworkMode:  spec.networkMode,
		Init:         true,
		Labels: map[string]string{
			"managed-by":        "gtool",
			"gtool-stable-mock": spec.name,
		},
	}

	id, err := l.docker.CreateContainer(ctx, cc)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed,
			fmt.Sprintf("failed to create %s container", spec.name))
	}
	if err := l.docker.StartContainer(ctx, id); err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrDockerFailed,
			fmt.Sprintf("failed to start %s container", spec.name))
	}
	return nil
}

func (l *Launcher) waitReady(ctx context.Context, spec *launchSpec) error {
	// Mountebank has no readiness log (legacy is_ready_mountebank returns 0).
	if len(spec.readyLogs) == 0 {
		return nil
	}

	for attempt := 0; attempt < readyAttempts; attempt++ {
		logs, err := l.docker.GetContainerLogs(ctx, spec.name, 500)
		if err == nil && containsAll(logs, spec.readyLogs) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(readyInterval):
		}
	}

	return gtErrors.New(gtErrors.ErrServiceTimeout,
		fmt.Sprintf("timeout waiting for %s to be ready", spec.name))
}

func (l *Launcher) specFor(name string, cfg *config.Config) (*launchSpec, error) {
	image := fmt.Sprintf("%s/%s:%s", mocksArtifactRepo, name, dockerTag)

	switch name {
	case "mountebank":
		dir := filepath.Join(l.mocksDataPath, "mountebank")
		if err := requireDir(dir); err != nil {
			return nil, err
		}
		return &launchSpec{
			name:        name,
			image:       image,
			networkMode: "host",
			mounts:      []docker.Mount{{Type: "bind", Source: dir, Target: "/imposters"}},
		}, nil

	case "postgresql":
		dir := filepath.Join(l.mocksDataPath, "postgresql")
		if err := requireSQLData(dir); err != nil {
			return nil, err
		}
		return &launchSpec{
			name:   name,
			image:  image,
			ports:  map[string]string{"5432": "5432"},
			mounts: []docker.Mount{{Type: "bind", Source: dir, Target: "/data"}},
			env:    []string{"POSTGRES_PASSWORD=postgres"},
			readyLogs: []string{
				"PostgreSQL init process complete; ready for start up",
				"database system is ready to accept connections",
			},
		}, nil

	case "pubsub":
		projectID, topics, err := buildPubsubEnv(cfg)
		if err != nil {
			return nil, err
		}
		return &launchSpec{
			name:  name,
			image: image,
			ports: map[string]string{"8085": pubsubMockPort},
			env: []string{
				"PROJECT_ID=" + projectID,
				"TOPICS=" + topics,
			},
			readyLogs: []string{"pubsub emulator running and ready"},
		}, nil

	default:
		return nil, gtErrors.New(gtErrors.ErrInvalidArgument,
			fmt.Sprintf("%q is not supported in STABLE mode (supported: %s)", name, strings.Join(Supported, ", ")))
	}
}

// buildPubsubEnv reproduces the legacy TOPICS construction:
// "<topic>[:<sub0>[&<sub1>...]]" entries joined by spaces.
func buildPubsubEnv(cfg *config.Config) (projectID, topics string, err error) {
	raw, ok := cfg.ThirdParty.MockConfig["pubsub"]
	if !ok || raw == nil {
		return "", "", gtErrors.New(gtErrors.ErrConfigInvalid,
			"pubsub mock-config is required to use the pubsub STABLE mock")
	}

	data, err := yaml.Marshal(raw)
	if err != nil {
		return "", "", gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to encode pubsub config")
	}

	var pc struct {
		ProjectID string `yaml:"project-id"`
		Topics    []struct {
			TopicID         string   `yaml:"topic-id"`
			SubscriptionIDs []string `yaml:"subscription-ids"`
		} `yaml:"topics"`
	}
	if err := yaml.Unmarshal(data, &pc); err != nil {
		return "", "", gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to parse pubsub config")
	}
	if pc.ProjectID == "" {
		return "", "", gtErrors.New(gtErrors.ErrConfigInvalid, "pubsub mock-config requires project-id")
	}

	entries := make([]string, 0, len(pc.Topics))
	for _, t := range pc.Topics {
		entry := t.TopicID
		for i, sub := range t.SubscriptionIDs {
			if i == 0 {
				entry += ":" + sub
			} else {
				entry += "&" + sub
			}
		}
		entries = append(entries, entry)
	}

	return pc.ProjectID, strings.Join(entries, " "), nil
}

func containsAll(haystack string, needles []string) bool {
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			return false
		}
	}
	return true
}

func requireDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return gtErrors.New(gtErrors.ErrInvalidArgument,
			fmt.Sprintf("mock data directory not found: %s", dir))
	}
	return nil
}

func requireSQLData(dir string) error {
	if err := requireDir(dir); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to read postgresql mock data")
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			return nil
		}
	}
	return gtErrors.New(gtErrors.ErrInvalidArgument,
		fmt.Sprintf("no .sql data files found in %s", dir))
}
