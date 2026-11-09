package test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/pkg/config"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

type fakeDocker struct {
	exitCode   int
	inspectErr error
	pullErr    error
	createErr  error
	startErr   error
	waitErr    error

	created    *docker.ContainerConfig // first container: the launcher
	createdAll []*docker.ContainerConfig
	removed    bool
	logsText   string
}

func (d *fakeDocker) EnsureImage(_ context.Context, _ string) error { return d.pullErr }
func (d *fakeDocker) CreateContainer(_ context.Context, c *docker.ContainerConfig) (string, error) {
	if d.created == nil {
		d.created = c
	}
	d.createdAll = append(d.createdAll, c)
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
	}, d.inspectErr
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

func TestKarateRunner_NoFeaturesPathIsAnError(t *testing.T) {
	d := &fakeDocker{}
	r := NewKarateRunner(d, "", nil)

	_, err := r.Run(context.Background(), config.DefaultConfig())

	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrConfigInvalid), "a pipeline without features must not pass")
	assert.Nil(t, d.created, "docker must not be touched")
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
	assert.Equal(t, docker.HostUser(), d.created.User)
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

func TestKarateRunner_ChownsReportsToUser(t *testing.T) {
	cfg := cfgWithFeatures(t)
	cfg.TestConfig.ReportsPath = filepath.Join(t.TempDir(), "reports")
	d := &fakeDocker{exitCode: 0}
	r := NewKarateRunner(d, "", nil)

	_, err := r.Run(context.Background(), cfg)
	require.NoError(t, err)

	require.Len(t, d.createdAll, 2, "launcher plus chown container")
	chown := d.createdAll[1]
	assert.Equal(t, []string{"chown"}, chown.Entrypoint)
	assert.Equal(t, []string{"-R", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), reportsTarget}, chown.Cmd)
	require.Len(t, chown.Mounts, 1)
	assert.Equal(t, cfg.TestConfig.ReportsPath, chown.Mounts[0].Source)
	assert.Equal(t, "chown", chown.Labels["gtool-role"])
}

func TestKarateRunner_NoChownWithoutReports(t *testing.T) {
	d := &fakeDocker{exitCode: 0}
	r := NewKarateRunner(d, "", nil)

	_, err := r.Run(context.Background(), cfgWithFeatures(t))
	require.NoError(t, err)

	assert.Len(t, d.createdAll, 1, "only the launcher container")
}

func TestPubsubEmulatorHost(t *testing.T) {
	tests := []struct {
		name  string
		mocks []string
		cfg   interface{}
		want  string
	}{
		{name: "pubsub not configured", mocks: []string{"postgresql"}, want: ""},
		{name: "default port", mocks: []string{"pubsub"}, want: "127.0.0.1:8085"},
		{name: "string port", mocks: []string{"pubsub"}, cfg: map[string]interface{}{"port": "9085"}, want: "127.0.0.1:9085"},
		{name: "int port", mocks: []string{"pubsub"}, cfg: map[string]interface{}{"port": 9086}, want: "127.0.0.1:9086"},
		{name: "float port", mocks: []string{"pubsub"}, cfg: map[string]interface{}{"port": float64(9087)}, want: "127.0.0.1:9087"},
		{name: "empty port falls back", mocks: []string{"pubsub"}, cfg: map[string]interface{}{"port": ""}, want: "127.0.0.1:8085"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.ThirdParty.Mocks = tt.mocks
			cfg.ThirdParty.MockConfig = map[string]interface{}{}
			if tt.cfg != nil {
				cfg.ThirdParty.MockConfig["pubsub"] = tt.cfg
			}
			assert.Equal(t, tt.want, pubsubEmulatorHost(cfg))
		})
	}
}

func TestKarateRunner_InjectsRunPorts(t *testing.T) {
	cfg := cfgWithFeatures(t)
	cfg.AppConfig.Port = 8080
	cfg.AppConfig.HostPort = "41000"
	cfg.ThirdParty.Mocks = []string{"postgresql"}
	cfg.ThirdParty.MockConfig = map[string]interface{}{
		"postgresql": map[string]interface{}{"port": "41001"},
	}
	d := &fakeDocker{exitCode: 0}
	r := NewKarateRunner(d, "", nil)

	_, err := r.Run(context.Background(), cfg)
	require.NoError(t, err)

	assert.Contains(t, d.created.Env, "GTOOL_APP_URL=http://localhost:41000")
	assert.Contains(t, d.created.Env, "GTOOL_APP_HOST_PORT=41000")
	assert.Contains(t, d.created.Env, "GTOOL_APP_PORT=8080")
	assert.Contains(t, d.created.Env, "GTOOL_POSTGRESQL_PORT=41001")
}

func TestKarateRunner_InjectsConnectionSettings(t *testing.T) {
	cfg := cfgWithFeatures(t)
	cfg.ThirdParty.Mocks = []string{"postgresql"}
	cfg.ThirdParty.MockConfig = map[string]interface{}{
		"postgresql": map[string]interface{}{"user": "app", "password": 1234, "database": "orders", "image": "x"},
		"mysql":      map[string]interface{}{"user": "not-enabled"},
	}
	d := &fakeDocker{exitCode: 0}

	_, err := NewKarateRunner(d, "", nil).Run(context.Background(), cfg)
	require.NoError(t, err)

	assert.Contains(t, d.created.Env, "GTOOL_POSTGRESQL_USER=app")
	assert.Contains(t, d.created.Env, "GTOOL_POSTGRESQL_PASSWORD=1234")
	assert.Contains(t, d.created.Env, "GTOOL_POSTGRESQL_DATABASE=orders")
	for _, e := range d.created.Env {
		assert.NotContains(t, e, "MYSQL", "mocks that are not enabled export nothing")
		assert.NotContains(t, e, "IMAGE")
	}
}

func TestKarateRunner_InjectsPubSubEmulatorHost(t *testing.T) {
	cfg := cfgWithFeatures(t)
	cfg.ThirdParty.Mocks = []string{"pubsub"}
	d := &fakeDocker{exitCode: 0}
	r := NewKarateRunner(d, "", nil)

	_, err := r.Run(context.Background(), cfg)
	require.NoError(t, err)

	assert.Contains(t, d.created.Env, "PUBSUB_EMULATOR_HOST=127.0.0.1:8085")
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

func TestKarateRunner_InspectFailureIsNotAPass(t *testing.T) {
	d := &fakeDocker{exitCode: 0, inspectErr: errors.New("daemon hiccup")}
	r := NewKarateRunner(d, "", nil)

	res, err := r.Run(context.Background(), cfgWithFeatures(t))

	require.Error(t, err)
	assert.Nil(t, res)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrTestFailed))
	assert.True(t, d.removed, "container must still be cleaned up")
}

// withSummary returns a config whose reports dir already holds a Karate summary.
func withSummary(t *testing.T, summary string) *config.Config {
	t.Helper()
	cfg := cfgWithFeatures(t)
	cfg.TestConfig.ReportsPath = t.TempDir()
	if summary != "" {
		require.NoError(t, os.WriteFile(filepath.Join(cfg.TestConfig.ReportsPath, summaryFile), []byte(summary), 0o644))
	}
	return cfg
}

func TestKarateRunner_UsesScenarioCounts(t *testing.T) {
	cfg := withSummary(t, `{"scenariosPassed": 2, "scenariosfailed": 1, "featureSummary": [
		{"name": "Users", "relativePath": "users.feature", "failedCount": 1},
		{"name": "Health", "relativePath": "health.feature", "failedCount": 0}]}`)
	d := &fakeDocker{exitCode: 1}

	res, err := NewKarateRunner(d, "", nil).Run(context.Background(), cfg)

	require.NoError(t, err)
	assert.Equal(t, 3, res.Total)
	assert.Equal(t, 2, res.Passed)
	assert.Equal(t, 1, res.Failed)
	require.Len(t, res.Failures, 2)
	assert.Equal(t, "users.feature", res.Failures[0].Feature)
}

func TestKarateRunner_AllPassed(t *testing.T) {
	cfg := withSummary(t, `{"scenariosPassed": 3, "scenariosfailed": 0}`)
	d := &fakeDocker{exitCode: 0}

	res, err := NewKarateRunner(d, "", nil).Run(context.Background(), cfg)

	require.NoError(t, err)
	assert.Equal(t, 3, res.Total)
	assert.Equal(t, 3, res.Passed)
	assert.Zero(t, res.Failed)
}

func TestKarateRunner_ZeroScenariosFails(t *testing.T) {
	cfg := withSummary(t, `{"scenariosPassed": 0, "scenariosfailed": 0}`)
	d := &fakeDocker{exitCode: 0}

	_, err := NewKarateRunner(d, "", nil).Run(context.Background(), cfg)

	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrTestFailed))
	assert.Contains(t, err.Error(), "no scenarios ran")
}

func TestKarateRunner_NonZeroExitWithoutFailedScenarios(t *testing.T) {
	cfg := withSummary(t, `{"scenariosPassed": 3, "scenariosfailed": 0}`)
	d := &fakeDocker{exitCode: 1}

	res, err := NewKarateRunner(d, "", nil).Run(context.Background(), cfg)

	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed, "a failing launcher is never a pass")
	assert.Equal(t, 4, res.Total)
}

func TestKarateRunner_InvalidSummaryFallsBackToExitCode(t *testing.T) {
	cfg := withSummary(t, `not json`)
	d := &fakeDocker{exitCode: 0}

	res, err := NewKarateRunner(d, "", nil).Run(context.Background(), cfg)

	require.NoError(t, err)
	assert.Equal(t, 1, res.Passed)
}
