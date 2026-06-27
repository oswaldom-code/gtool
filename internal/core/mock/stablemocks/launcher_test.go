package stablemocks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/oswaldo-montano/gtool/internal/infra/docker"
	"github.com/oswaldo-montano/gtool/pkg/config"
)

// fakeDocker records interactions and lets tests drive readiness.
type fakeDocker struct {
	created   []*docker.ContainerConfig
	started   []string
	removed   []string
	logs      string
	ensureErr error
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
func (f *fakeDocker) RemoveContainerByName(_ context.Context, name string) (bool, error) {
	f.removed = append(f.removed, name)
	return true, nil
}

func mockConfig(t *testing.T, yamlStr string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	require.NoError(t, yaml.Unmarshal([]byte(yamlStr), &m))
	return m
}

func TestBuildPubsubEnv(t *testing.T) {
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

	projectID, topics, err := buildPubsubEnv(cfg)
	require.NoError(t, err)
	assert.Equal(t, "my-project", projectID)
	// First sub uses ':', the rest use '&'; topics joined by spaces.
	assert.Equal(t, "topic-a:sub-a1&sub-a2 topic-b topic-c:sub-c1", topics)
}

func TestBuildPubsubEnvErrors(t *testing.T) {
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
			_, _, err := buildPubsubEnv(cfg)
			require.Error(t, err)
		})
	}
}

func TestContainsAll(t *testing.T) {
	assert.True(t, containsAll("abc ready def done", []string{"ready", "done"}))
	assert.False(t, containsAll("abc ready", []string{"ready", "done"}))
	assert.True(t, containsAll("anything", nil))
}

func TestUpLaunchesPubsubWithContract(t *testing.T) {
	fd := &fakeDocker{logs: "pubsub emulator running and ready"}
	l := &Launcher{docker: fd, logger: zap.NewNop(), mocksDataPath: t.TempDir()}

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
	assert.Equal(t, mocksArtifactRepo+"/pubsub:"+dockerTag, cc.Image)
	assert.Equal(t, pubsubMockPort, cc.PortBindings["8085"])
	assert.True(t, cc.Init)
	assert.Contains(t, cc.Env, "PROJECT_ID=p1")
	assert.Contains(t, cc.Env, "TOPICS=t1:s1")
	assert.Equal(t, []string{"id-pubsub"}, fd.started)
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
}
