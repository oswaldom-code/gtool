package gcs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewGCSPlugin(t *testing.T) {
	p := NewGCSPlugin(nil, zap.NewNop())

	assert.NotNil(t, p)
	assert.Equal(t, "gcs", p.Name())
	assert.NotNil(t, p.httpClient)
}

func TestName(t *testing.T) {
	p := NewGCSPlugin(nil, nil)
	assert.Equal(t, "gcs", p.Name())
}

func TestParseConfig(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		p := NewGCSPlugin(nil, nil)
		got, err := p.parseConfig(map[string]interface{}{})

		require.NoError(t, err)
		assert.Equal(t, defaultImage, got.Image)
		assert.Equal(t, defaultPort, got.Port)
		assert.Equal(t, defaultProjectID, got.ProjectID)
		assert.Empty(t, got.Buckets)
		assert.Contains(t, got.ContainerName, containerNamePrefix)
	})

	t.Run("buckets, project and numeric port", func(t *testing.T) {
		p := NewGCSPlugin(nil, nil)
		got, err := p.parseConfig(map[string]interface{}{
			"port":       float64(14443),
			"project-id": "my-project",
			"buckets":    []interface{}{"uploads", "", 42, "exports"},
		})

		require.NoError(t, err)
		assert.Equal(t, "14443", got.Port)
		assert.Equal(t, "my-project", got.ProjectID)
		assert.Equal(t, []string{"uploads", "exports"}, got.Buckets)
	})

	t.Run("custom container name", func(t *testing.T) {
		p := NewGCSPlugin(nil, nil)
		got, err := p.parseConfig(map[string]interface{}{"container-name": "my-gcs"})

		require.NoError(t, err)
		assert.Equal(t, "my-gcs", got.ContainerName)
	})
}

func TestGetConnectionInfo(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		p := NewGCSPlugin(nil, nil)
		p.config = &GCSConfig{Port: "4443", ProjectID: "test-project"}

		got, err := p.GetConnectionInfo()

		require.NoError(t, err)
		assert.Equal(t, "localhost", got.Host)
		assert.Equal(t, 4443, got.Port)
		assert.Equal(t, "http", got.Protocol)
		assert.Equal(t, "http://localhost:4443", got.Metadata["storage-emulator-host"])
		assert.Equal(t, "test-project", got.Metadata["project-id"])
	})

	t.Run("not launched", func(t *testing.T) {
		p := NewGCSPlugin(nil, nil)

		_, err := p.GetConnectionInfo()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not launched")
	})
}

func TestBucketsURL(t *testing.T) {
	p := NewGCSPlugin(nil, nil)
	p.config = &GCSConfig{Port: "4443", ProjectID: "test-project"}

	assert.Equal(t, "http://localhost:4443/storage/v1/b?project=test-project", p.bucketsURL())
}

func TestBuildCmd(t *testing.T) {
	cmd := buildCmd(&GCSConfig{Port: "14443"})

	assert.Equal(t, []string{
		"-scheme", "http",
		"-host", "0.0.0.0",
		"-port", "4443",
		"-external-url", "http://localhost:14443",
		"-public-host", "localhost:14443",
	}, cmd)
}

func TestIsReady_NotStarted(t *testing.T) {
	p := NewGCSPlugin(nil, nil)

	ready, err := p.IsReady(nil)

	assert.False(t, ready)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestStop_NoContainer(t *testing.T) {
	p := NewGCSPlugin(nil, zap.NewNop())

	err := p.Stop(nil)
	assert.NoError(t, err)
}

func TestMustParsePort(t *testing.T) {
	assert.Equal(t, 4443, mustParsePort("4443"))
	assert.Equal(t, 0, mustParsePort("0"))
}
