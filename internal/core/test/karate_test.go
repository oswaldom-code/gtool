package test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oswaldo-montano/gtool/internal/infra/docker"
	"github.com/oswaldo-montano/gtool/pkg/config"
	gtErrors "github.com/oswaldo-montano/gtool/pkg/errors"
)

type fakeDocker struct {
	exitCode  int
	pullErr   error
	createErr error
	startErr  error
	waitErr   error

	created  *docker.ContainerConfig
	removed  bool
	logsText string
}

func (d *fakeDocker) PullImage(_ context.Context, _ string) error { return d.pullErr }
func (d *fakeDocker) CreateContainer(_ context.Context, c *docker.ContainerConfig) (string, error) {
	d.created = c
	if d.createErr != nil {
		return "", d.createErr
	}
	return "test-container", nil
}
func (d *fakeDocker) StartContainer(_ context.Context, _ string) error { return d.startErr }
func (d *fakeDocker) WaitForContainer(_ context.Context, _ string, _ container.WaitCondition) error {
	return d.waitErr
}
func (d *fakeDocker) InspectContainer(_ context.Context, _ string) (*dockertypes.ContainerJSON, error) {
	return &dockertypes.ContainerJSON{
		ContainerJSONBase: &dockertypes.ContainerJSONBase{
			State: &dockertypes.ContainerState{ExitCode: d.exitCode},
		},
	}, nil
}
func (d *fakeDocker) GetContainerLogs(_ context.Context, _ string, _ int) (string, error) {
	return d.logsText, nil
}
func (d *fakeDocker) RemoveContainer(_ context.Context, _ string, _ bool) error {
	d.removed = true
	return nil
}

func cfgWithFeatures(t *testing.T) *config.Config {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.TestConfig.FeaturesPath = t.TempDir() // exists, abs-resolvable
	cfg.TestConfig.Tags = "smoke,regression"
	return cfg
}

func TestKarateRunner_Skip_NoFeatures(t *testing.T) {
	d := &fakeDocker{}
	r := NewKarateRunner(d, "", nil)

	res, err := r.Run(context.Background(), config.DefaultConfig())

	require.NoError(t, err)
	assert.Equal(t, 0, res.Total, "no features means nothing ran")
	assert.Nil(t, d.created, "docker must not be touched when skipping")
}

func TestKarateRunner_Pass(t *testing.T) {
	d := &fakeDocker{exitCode: 0}
	r := NewKarateRunner(d, "", nil)

	res, err := r.Run(context.Background(), cfgWithFeatures(t))

	require.NoError(t, err)
	assert.Equal(t, 1, res.Total)
	assert.Equal(t, 1, res.Passed)
	assert.Equal(t, 0, res.Failed)
	assert.True(t, d.removed, "container must be cleaned up")
	// Tags propagated and host networking used so tests reach localhost.
	assert.Contains(t, d.created.Env, "TAGS=smoke,regression")
	assert.Equal(t, "host", d.created.NetworkMode)
	assert.Equal(t, DefaultLauncherImage, d.created.Image)
}

func TestKarateRunner_Fail_NonZeroExit(t *testing.T) {
	d := &fakeDocker{exitCode: 1, logsText: "scenario failed\nassert error"}
	r := NewKarateRunner(d, "", nil)

	res, err := r.Run(context.Background(), cfgWithFeatures(t))

	require.NoError(t, err, "a test failure is data, not an execution error")
	assert.Equal(t, 1, res.Failed)
	require.Len(t, res.Failures, 1)
	assert.Contains(t, res.Failures[0].Message, "code 1")
}

func TestKarateRunner_MountsFeaturesAndReports(t *testing.T) {
	cfg := cfgWithFeatures(t)
	cfg.TestConfig.ReportsPath = filepath.Join(t.TempDir(), "reports")
	d := &fakeDocker{exitCode: 0}
	r := NewKarateRunner(d, "", nil)

	_, err := r.Run(context.Background(), cfg)
	require.NoError(t, err)

	require.Len(t, d.created.Mounts, 2)
	assert.Equal(t, featuresTarget, d.created.Mounts[0].Target)
	assert.True(t, d.created.Mounts[0].ReadOnly)
	assert.Equal(t, reportsTarget, d.created.Mounts[1].Target)
}

func TestKarateRunner_InfraErrors(t *testing.T) {
	cases := map[string]*fakeDocker{
		"pull":   {pullErr: errors.New("boom")},
		"create": {createErr: errors.New("boom")},
		"start":  {startErr: errors.New("boom")},
		"wait":   {waitErr: errors.New("boom")},
	}
	for name, d := range cases {
		t.Run(name, func(t *testing.T) {
			r := NewKarateRunner(d, "", nil)
			_, err := r.Run(context.Background(), cfgWithFeatures(t))
			require.Error(t, err)
			assert.True(t, gtErrors.Is(err, gtErrors.ErrTestFailed))
		})
	}
}

func TestNewKarateRunner_DefaultImage(t *testing.T) {
	r := NewKarateRunner(&fakeDocker{}, "", nil)
	assert.Equal(t, DefaultLauncherImage, r.image)

	r2 := NewKarateRunner(&fakeDocker{}, "custom:1", nil)
	assert.Equal(t, "custom:1", r2.image)
}
