package stablekarate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/oswaldom-code/gtool/internal/infra/docker"
)

type fakeDocker struct {
	created    []*docker.ContainerConfig
	exitCode   int
	removed    bool
	reportsSrc string
}

func (f *fakeDocker) EnsureImage(context.Context, string) error { return nil }

func (f *fakeDocker) CreateContainer(_ context.Context, cfg *docker.ContainerConfig) (string, error) {
	f.created = append(f.created, cfg)
	for _, m := range cfg.Mounts {
		if m.Target == reportsTarget {
			f.reportsSrc = m.Source
		}
	}
	return "cid", nil
}

// karateCfg returns the launcher container config (the one on the host network),
// as opposed to the helper chown container.
func (f *fakeDocker) karateCfg() *docker.ContainerConfig {
	for _, c := range f.created {
		if c.NetworkMode == "host" {
			return c
		}
	}
	return nil
}

func (f *fakeDocker) StartContainer(context.Context, string) error { return nil }

func (f *fakeDocker) WaitForContainer(context.Context, string, container.WaitCondition) error {
	// Simulate the launcher writing the HTML report.
	if f.reportsSrc != "" {
		_ = os.WriteFile(filepath.Join(f.reportsSrc, summaryFile), []byte("<html></html>"), 0o644)
	}
	return nil
}

func (f *fakeDocker) InspectContainer(context.Context, string) (*dockertypes.ContainerJSON, error) {
	return &dockertypes.ContainerJSON{
		ContainerJSONBase: &dockertypes.ContainerJSONBase{
			State: &dockertypes.ContainerState{ExitCode: f.exitCode},
		},
	}, nil
}

func (f *fakeDocker) GetContainerLogs(context.Context, string, int) (string, error) {
	return "karate logs", nil
}

func (f *fakeDocker) RemoveContainer(context.Context, string, bool) error {
	f.removed = true
	return nil
}

func newRunner(t *testing.T, fd *fakeDocker) (*Runner, *[]string) {
	t.Helper()
	opened := &[]string{}
	r := New(fd, zap.NewNop())
	r.opener = func(path string) error {
		*opened = append(*opened, path)
		return nil
	}
	return r, opened
}

func dirs(t *testing.T) (features, reports string) {
	t.Helper()
	base := t.TempDir()
	features = filepath.Join(base, "features")
	reports = filepath.Join(base, "reports")
	require.NoError(t, os.Mkdir(features, 0o755))
	return features, reports
}

func TestRunPassesAndOpensReport(t *testing.T) {
	features, reports := dirs(t)
	fd := &fakeDocker{exitCode: 0}
	r, opened := newRunner(t, fd)

	res, err := r.Run(context.Background(), Options{
		FeaturesPath: features,
		ReportsPath:  reports,
		Tags:         "@smoke",
		UrlsToBlock:  "a.com,b.com",
		Open:         true,
	})
	require.NoError(t, err)
	assert.True(t, res.Passed)
	assert.Equal(t, 0, res.ExitCode)
	assert.True(t, fd.removed, "container should be removed")

	// Report generated and opened.
	require.NotEmpty(t, res.ReportPath)
	require.Len(t, *opened, 1)
	assert.Equal(t, res.ReportPath, (*opened)[0])

	// Container contract.
	cc := fd.karateCfg()
	require.NotNil(t, cc)
	assert.Equal(t, defaultImage, cc.Image)
	assert.Equal(t, "host", cc.NetworkMode)
	assert.True(t, cc.Init)
	assert.Contains(t, cc.Env, "PUBSUB_EMULATOR_HOST="+pubsubEmulatorHost)
	assert.Contains(t, cc.Env, "TAGS=@smoke")
	assert.Contains(t, cc.Env, "URLS_TO_BLOCK=a.com,b.com")

	var featTarget, repTarget string
	for _, m := range cc.Mounts {
		switch m.Target {
		case featuresTarget:
			featTarget = m.Source
			assert.True(t, m.ReadOnly)
		case reportsTarget:
			repTarget = m.Source
		}
	}
	assert.NotEmpty(t, featTarget)
	assert.NotEmpty(t, repTarget)
}

func TestRunDoesNotOpenWhenDisabled(t *testing.T) {
	features, reports := dirs(t)
	fd := &fakeDocker{exitCode: 0}
	r, opened := newRunner(t, fd)

	_, err := r.Run(context.Background(), Options{FeaturesPath: features, ReportsPath: reports, Open: false})
	require.NoError(t, err)
	assert.Empty(t, *opened)
}

func TestRunFailsOnNonZeroExit(t *testing.T) {
	features, reports := dirs(t)
	fd := &fakeDocker{exitCode: 1}
	r, _ := newRunner(t, fd)

	res, err := r.Run(context.Background(), Options{FeaturesPath: features, ReportsPath: reports})
	require.NoError(t, err)
	assert.False(t, res.Passed)
	assert.Equal(t, 1, res.ExitCode)
}

func TestRunErrorsWhenFeaturesMissing(t *testing.T) {
	fd := &fakeDocker{}
	r, _ := newRunner(t, fd)
	_, err := r.Run(context.Background(), Options{FeaturesPath: "/does/not/exist", ReportsPath: t.TempDir()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "features directory not found")
}

func TestRunImageOverride(t *testing.T) {
	features, reports := dirs(t)
	fd := &fakeDocker{}
	r, _ := newRunner(t, fd)
	_, err := r.Run(context.Background(), Options{FeaturesPath: features, ReportsPath: reports, Image: "custom:tag"})
	require.NoError(t, err)
	assert.Equal(t, "custom:tag", fd.karateCfg().Image)
}
