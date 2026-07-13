// Package stablemocks launches the third-party mocks for the component pipeline
// using the official public images for each tool, with a fixed docker run
// contract (network, ports, mounts, container names and readiness) so
// `gtool services up --stable` provides a reproducible mock environment that
// seeds data from test/component/mocks-data.
package stablemocks

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
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
	// Official public images for each mocked tool.
	postgresImage   = "postgres:16-alpine"
	pubsubImage     = "gcr.io/google.com/cloudsdktool/cloud-sdk:emulators"
	mountebankImage = "bbyars/mountebank:2.9.1"

	pubsubMockPort = "9085"

	readyAttempts = 60
	readyInterval = 3 * time.Second
)

// Supported lists the mocks this launcher can start.
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

// Launcher launches and stops the mock containers.
type Launcher struct {
	docker        dockerClient
	logger        *zap.Logger
	mocksDataPath string
	httpDo        func(req *http.Request) (*http.Response, error)
}

// New builds a Launcher resolving the mocks-data directory from the working
// directory ($PWD/test/component/mocks-data).
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
		httpDo:        (&http.Client{Timeout: 10 * time.Second}).Do,
	}, nil
}

// pubsubTopic is a topic and its subscriptions, created via the emulator REST
// API once it is ready.
type pubsubTopic struct {
	name string
	subs []string
}

type launchSpec struct {
	name        string
	image       string
	networkMode string
	entrypoint  []string
	cmd         []string
	ports       map[string]string // container port -> host port
	mounts      []docker.Mount
	env         []string
	readyLogs   []string // every entry must be present in the logs to be ready
	readyHTTP   string   // GET url that must return 200 to be ready

	// pubsub-only: resources created over REST after readiness.
	projectID string
	topics    []pubsubTopic
}

// Up launches the given services (or all configured mocks), waits until each
// one is ready and seeds any post-start resources.
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
		fmt.Printf("🚀 Launching %s...\n", spec.name)
		if err := l.launchOne(ctx, spec); err != nil {
			return err
		}
	}

	fmt.Printf("⏳ Waiting for mocks to be ready...\n")
	for _, spec := range specs {
		if err := l.waitReady(ctx, spec); err != nil {
			return err
		}
		if len(spec.topics) > 0 {
			if err := l.seedPubsub(ctx, spec); err != nil {
				return err
			}
		}
		fmt.Printf("✅ %s ready\n", spec.name)
	}

	return nil
}

// Down removes the fixed-name mock containers.
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
		Entrypoint:   spec.entrypoint,
		Cmd:          spec.cmd,
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
	if spec.readyHTTP != "" {
		return l.waitHTTP(ctx, spec.readyHTTP)
	}
	// No readiness signal (e.g. mountebank loads its imposters at startup).
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

func (l *Launcher) waitHTTP(ctx context.Context, url string) error {
	do := l.httpClient()
	for attempt := 0; attempt < readyAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to build readiness request")
		}
		if resp, err := do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(readyInterval):
		}
	}
	return gtErrors.New(gtErrors.ErrServiceTimeout, fmt.Sprintf("timeout waiting for %s", url))
}

func (l *Launcher) specFor(name string, cfg *config.Config) (*launchSpec, error) {
	switch name {
	case "mountebank":
		dir := filepath.Join(l.mocksDataPath, "mountebank")
		if err := requireDir(dir); err != nil {
			return nil, err
		}

		return &launchSpec{
			name:        name,
			image:       mountebankImage,
			networkMode: "host",
			entrypoint:  []string{"mb"},
			cmd:         []string{"start", "--configfile", "/imposters/imposters.ejs", "--allowInjection"},
			mounts:      []docker.Mount{{Type: "bind", Source: dir, Target: "/imposters"}},
		}, nil

	case "postgresql":
		dir := filepath.Join(l.mocksDataPath, "postgresql")
		if err := requireSQLData(dir); err != nil {
			return nil, err
		}

		return &launchSpec{
			name:   name,
			image:  postgresImage,
			ports:  map[string]string{"5432": "5432"},
			mounts: []docker.Mount{{Type: "bind", Source: dir, Target: "/docker-entrypoint-initdb.d", ReadOnly: true}},
			env:    []string{"POSTGRES_PASSWORD=postgres"},
			readyLogs: []string{
				"PostgreSQL init process complete; ready for start up",
				"database system is ready to accept connections",
			},
		}, nil

	case "pubsub":
		projectID, topics, err := parsePubsub(cfg)
		if err != nil {
			return nil, err
		}
		return &launchSpec{
			name:  name,
			image: pubsubImage,
			cmd: []string{
				"gcloud", "beta", "emulators", "pubsub", "start",
				"--host-port=0.0.0.0:8085",
				"--project=" + projectID,
			},
			ports:     map[string]string{"8085": pubsubMockPort},
			readyHTTP: fmt.Sprintf("http://localhost:%s/v1/projects/%s/topics", pubsubMockPort, projectID),
			projectID: projectID,
			topics:    topics,
		}, nil

	default:
		return nil, gtErrors.New(gtErrors.ErrInvalidArgument,
			fmt.Sprintf("%q is not supported in stable mode (supported: %s)", name, strings.Join(Supported, ", ")))
	}
}

// seedPubsub creates the configured topics and subscriptions over the emulator
// REST API once it is serving requests.
func (l *Launcher) seedPubsub(ctx context.Context, spec *launchSpec) error {
	base := fmt.Sprintf("http://localhost:%s/v1/projects/%s", pubsubMockPort, spec.projectID)
	for _, t := range spec.topics {
		if err := l.putResource(ctx, fmt.Sprintf("%s/topics/%s", base, t.name), nil); err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
				fmt.Sprintf("failed to create topic %s", t.name))
		}
		for _, sub := range t.subs {
			body := []byte(fmt.Sprintf(`{"topic":"projects/%s/topics/%s"}`, spec.projectID, t.name))
			if err := l.putResource(ctx, fmt.Sprintf("%s/subscriptions/%s", base, sub), body); err != nil {
				return gtErrors.Wrap(err, gtErrors.ErrServiceFailed,
					fmt.Sprintf("failed to create subscription %s", sub))
			}
		}
	}
	return nil
}

func (l *Launcher) putResource(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to build request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.httpClient()(req)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return gtErrors.New(gtErrors.ErrServiceFailed,
			fmt.Sprintf("unexpected status %d for %s", resp.StatusCode, url))
	}
	return nil
}

func (l *Launcher) httpClient() func(*http.Request) (*http.Response, error) {
	if l.httpDo != nil {
		return l.httpDo
	}
	return http.DefaultClient.Do
}

// parsePubsub reads the pubsub mock-config into a project id and its topics.
func parsePubsub(cfg *config.Config) (string, []pubsubTopic, error) {
	raw, ok := cfg.ThirdParty.MockConfig["pubsub"]
	if !ok || raw == nil {
		return "", nil, gtErrors.New(gtErrors.ErrConfigInvalid,
			"pubsub mock-config is required to use the pubsub mock")
	}

	data, err := yaml.Marshal(raw)
	if err != nil {
		return "", nil, gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to encode pubsub config")
	}

	var pc struct {
		ProjectID string `yaml:"project-id"`
		Topics    []struct {
			TopicID         string   `yaml:"topic-id"`
			SubscriptionIDs []string `yaml:"subscription-ids"`
		} `yaml:"topics"`
	}
	if err := yaml.Unmarshal(data, &pc); err != nil {
		return "", nil, gtErrors.Wrap(err, gtErrors.ErrConfigInvalid, "failed to parse pubsub config")
	}
	if pc.ProjectID == "" {
		return "", nil, gtErrors.New(gtErrors.ErrConfigInvalid, "pubsub mock-config requires project-id")
	}

	topics := make([]pubsubTopic, 0, len(pc.Topics))
	for _, t := range pc.Topics {
		topics = append(topics, pubsubTopic{name: t.TopicID, subs: t.SubscriptionIDs})
	}
	return pc.ProjectID, topics, nil
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
