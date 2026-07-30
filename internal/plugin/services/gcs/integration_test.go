//go:build integration
// +build integration

package gcs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/pkg/logger"
)

func TestGCSIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	log := logger.Default()
	defer log.Sync()

	dockerClient, err := docker.NewClient(log.Logger)
	require.NoError(t, err, "Failed to create Docker client")
	defer dockerClient.Close()

	ctx := context.Background()
	err = dockerClient.Ping(ctx)
	require.NoError(t, err, "Docker daemon not available")

	plugin := NewGCSPlugin(dockerClient, log.Logger)
	require.NotNil(t, plugin)

	const port = "14443"
	config := map[string]interface{}{
		"image":      defaultImage,
		"port":       port,
		"project-id": "test-project",
		"buckets":    []interface{}{"uploads"},
	}

	log.Info("launching GCS emulator for integration test")
	err = plugin.Launch(ctx, config)
	require.NoError(t, err, "Failed to launch GCS")

	defer func() {
		log.Info("cleaning up GCS container")
		if err := plugin.Stop(ctx); err != nil {
			t.Logf("Failed to stop GCS: %v", err)
		}
	}()

	t.Run("IsReady", func(t *testing.T) {
		ready, err := plugin.IsReady(ctx)
		require.NoError(t, err)
		assert.True(t, ready, "GCS should be ready")
	})

	t.Run("GetConnectionInfo", func(t *testing.T) {
		connInfo, err := plugin.GetConnectionInfo()
		require.NoError(t, err)
		assert.Equal(t, "localhost", connInfo.Host)
		assert.Equal(t, 14443, connInfo.Port)
		assert.Equal(t, "test-project", connInfo.Metadata["project-id"])
	})

	t.Run("BucketCreated", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/storage/v1/b?project=test-project", port))
		require.NoError(t, err)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, strings.Contains(string(body), "uploads"), "should list the 'uploads' bucket")
	})

	t.Run("Stop", func(t *testing.T) {
		err := plugin.Stop(ctx)
		require.NoError(t, err, "Failed to stop GCS")
	})
}
