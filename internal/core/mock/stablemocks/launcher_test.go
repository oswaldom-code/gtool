package stablemocks

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

// fakeDocker records interactions and lets tests drive readiness.
type fakeDocker struct {
	created   []*docker.ContainerConfig
	started   []string
	removed   []string
	logs      string
	ensureErr error
	foreign   map[string]bool
	labels    map[string]map[string]string
}

func (f *fakeDocker) EnsureImage(_ context.Context, _ string) error { return f.ensureErr }
func (f *fakeDocker) CreateContainer(_ context.Context, cfg *docker.ContainerConfig) (string, error) {
	f.created = append(f.created, cfg)
	return "id-" + cfg.Name, nil
}
func (f *fakeDocker) StartContainer(_ context.Context, id string) error {
	f.started = append(f.started, id)
	return nil
}
func (f *fakeDocker) GetContainerLogs(_ context.Context, _ string, _ int) (string, error) {
	return f.logs, nil
}
func (f *fakeDocker) RemoveContainerByName(_ context.Context, name string, labels map[string]string) (bool, error) {
	if f.labels == nil {
		f.labels = map[string]map[string]string{}
	}
	f.labels[name] = labels
	if f.foreign[name] {
		return false, gtErrors.New(gtErrors.ErrContainerForeign, "not ours")
	}
	f.removed = append(f.removed, name)
	return true, nil
}

type fakeHTTP struct {
	reqs []*http.Request
}

func (h *fakeHTTP) do(req *http.Request) (*http.Response, error) {
	h.reqs = append(h.reqs, req)
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func mockConfig(t *testing.T, yamlStr string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	require.NoError(t, yaml.Unmarshal([]byte(yamlStr), &m))
	return m
}

func TestParsePubsub(t *testing.T) {
	cfg := &config.Config{}
	cfg.ThirdParty.MockConfig = map[string]interface{}{
		"pubsub": mockConfig(t, `
project-id: my-project
topics:
  - topic-id: topic-a
    subscription-ids: [sub-a1, sub-a2]
  - topic-id: topic-b
  - topic-id: topic-c
    subscription-ids: [sub-c1]
`),
	}

	projectID, topics, err := parsePubsub(cfg)
	require.NoError(t, err)
	assert.Equal(t, "my-project", projectID)
	require.Len(t, topics, 3)
	assert.Equal(t, "topic-a", topics[0].name)
	assert.Equal(t, []string{"sub-a1", "sub-a2"}, topics[0].subs)
	assert.Equal(t, "topic-b", topics[1].name)
	assert.Empty(t, topics[1].subs)
	assert.Equal(t, "topic-c", topics[2].name)
	assert.Equal(t, []string{"sub-c1"}, topics[2].subs)
}

func TestParsePubsubErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  map[string]interface{}
	}{
		{name: "missing pubsub", cfg: map[string]interface{}{}},
		{name: "missing project-id", cfg: map[string]interface{}{
			"pubsub": mockConfig(t, "topics: []"), // present but no project-id
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.ThirdParty.MockConfig = tt.cfg
			_, _, err := parsePubsub(cfg)
			require.Error(t, err)
		})
	}
}

func TestContainsInOrder(t *testing.T) {
	assert.True(t, containsInOrder("abc ready def done", []string{"ready", "done"}))
	assert.False(t, containsInOrder("abc ready", []string{"ready", "done"}))
	assert.True(t, containsInOrder("anything", nil))

	initPhase := "database system is ready to accept connections\nPostgreSQL init process complete; ready for start up\n"
	needles := []string{"PostgreSQL init process complete; ready for start up", "database system is ready to accept connections"}
	assert.False(t, containsInOrder(initPhase, needles), "the temporary init server's ready line must not count")
	assert.True(t, containsInOrder(initPhase+"database system is ready to accept connections\n", needles))
}

func TestUpLaunchesPubsubWithPublicImage(t *testing.T) {
	fd := &fakeDocker{}
	fh := &fakeHTTP{}
	l := &Launcher{docker: fd, logger: zap.NewNop(), mocksDataPath: t.TempDir(), httpDo: fh.do}

	cfg := &config.Config{}
	cfg.ThirdParty.MockConfig = map[string]interface{}{
		"pubsub": mockConfig(t, `
project-id: p1
topics:
  - topic-id: t1
    subscription-ids: [s1]
`),
	}

	require.NoError(t, l.Up(context.Background(), []string{"pubsub"}, cfg))

	require.Len(t, fd.created, 1)
	cc := fd.created[0]
	assert.Equal(t, "pubsub", cc.Name)
	assert.Equal(t, pubsubImage, cc.Image)
	assert.Equal(t, pubsubMockPort, cc.PortBindings["8085"])
	assert.True(t, cc.Init)
	assert.Contains(t, cc.Cmd, "--project=p1")
	for _, e := range cc.Env {
		assert.NotContains(t, e, "TOPICS=")
	}
	assert.Equal(t, []string{"id-pubsub"}, fd.started)

	var gets, puts []string
	for _, r := range fh.reqs {
		switch r.Method {
		case http.MethodGet:
			gets = append(gets, r.URL.Path)
		case http.MethodPut:
			puts = append(puts, r.URL.Path)
		}
	}
	require.NotEmpty(t, gets)
	require.Len(t, puts, 2)
	assert.Contains(t, puts[0], "/topics/t1")
	assert.Contains(t, puts[1], "/subscriptions/s1")
}

func TestUpUnsupportedService(t *testing.T) {
	fd := &fakeDocker{}
	l := &Launcher{docker: fd, logger: zap.NewNop(), mocksDataPath: t.TempDir()}
	err := l.Up(context.Background(), []string{"kafka"}, &config.Config{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestDownRemovesContainers(t *testing.T) {
	fd := &fakeDocker{}
	l := &Launcher{docker: fd, logger: zap.NewNop()}
	require.NoError(t, l.Down(context.Background(), []string{"pubsub", "postgresql"}))
	assert.Equal(t, []string{"pubsub", "postgresql"}, fd.removed)
	assert.Equal(t, map[string]string{"managed-by": "gtool", "gtool-stable-mock": "pubsub"}, fd.labels["pubsub"],
		"removal must be restricted to gtool's stable mock containers")
}

func TestDownSkipsForeignContainers(t *testing.T) {
	fd := &fakeDocker{foreign: map[string]bool{"postgresql": true}}
	l := &Launcher{docker: fd, logger: zap.NewNop()}

	require.NoError(t, l.Down(context.Background(), []string{"postgresql", "pubsub"}))

	assert.Equal(t, []string{"pubsub"}, fd.removed, "a user's own container named postgresql is left alone")
}

func TestUpFailsOnForeignContainer(t *testing.T) {
	fd := &fakeDocker{foreign: map[string]bool{"pubsub": true}}
	l := &Launcher{docker: fd, logger: zap.NewNop(), mocksDataPath: t.TempDir()}

	cfg := &config.Config{}
	cfg.ThirdParty.MockConfig = map[string]interface{}{
		"pubsub": mockConfig(t, "project-id: p1\ntopics:\n  - topic-id: t1\n"),
	}

	err := l.Up(context.Background(), []string{"pubsub"}, cfg)

	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrContainerForeign), err.Error())
	assert.Empty(t, fd.created)
}
