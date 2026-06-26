//go:build integration
// +build integration

package mountebank

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oswaldo-montano/gtool/internal/infra/docker"
	"github.com/oswaldo-montano/gtool/pkg/logger"
)

func TestMountebankIntegration(t *testing.T) {
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

	plugin := NewMountebankPlugin(dockerClient, log.Logger)
	require.NotNil(t, plugin)

	const port = "12525"
	config := map[string]interface{}{
		"image":          defaultImage,
		"port":           port,
		"imposters-path": "../../../../test/component/mocks-data/mountebank",
	}

	log.Info("launching Mountebank for integration test")
	err = plugin.Launch(ctx, config)
	require.NoError(t, err, "Failed to launch Mountebank")

	defer func() {
		log.Info("cleaning up Mountebank container")
		if err := plugin.Stop(ctx); err != nil {
			t.Logf("Failed to stop Mountebank: %v", err)
		}
	}()

	t.Run("IsReady", func(t *testing.T) {
		ready, err := plugin.IsReady(ctx)
		require.NoError(t, err)
		assert.True(t, ready, "Mountebank should be ready")
	})

	t.Run("GetConnectionInfo", func(t *testing.T) {
		connInfo, err := plugin.GetConnectionInfo()
		require.NoError(t, err)
		assert.Equal(t, "localhost", connInfo.Host)
		assert.Equal(t, 12525, connInfo.Port)
		assert.Equal(t, "http", connInfo.Protocol)
	})

	t.Run("ImposterLoaded", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/imposters/4545", port))
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "imposter 4545 should be registered")
	})

	t.Run("GetLogs", func(t *testing.T) {
		logs, err := plugin.GetLogs(ctx, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, logs, "Should have logs")
	})

	t.Run("Stop", func(t *testing.T) {
		err := plugin.Stop(ctx)
		require.NoError(t, err, "Failed to stop Mountebank")
	})
}
