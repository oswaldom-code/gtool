//go:build integration
// +build integration

package couchbase

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/pkg/logger"
)

func TestCouchbaseIntegration(t *testing.T) {
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

	plugin := NewCouchbasePlugin(dockerClient, log.Logger)
	require.NotNil(t, plugin)

	config := map[string]interface{}{
		"image":      defaultImage,
		"admin-port": "18091",
		"data-port":  "21210",
		"bucket":     "orders",
	}

	log.Info("launching Couchbase for integration test")
	err = plugin.Launch(ctx, config)
	require.NoError(t, err, "Failed to launch Couchbase")

	defer func() {
		log.Info("cleaning up Couchbase container")
		if err := plugin.Stop(ctx); err != nil {
			t.Logf("Failed to stop Couchbase: %v", err)
		}
	}()

	t.Run("IsReady", func(t *testing.T) {
		ready, err := plugin.IsReady(ctx)
		require.NoError(t, err)
		assert.True(t, ready, "Couchbase should be ready")
	})

	t.Run("GetConnectionInfo", func(t *testing.T) {
		connInfo, err := plugin.GetConnectionInfo()
		require.NoError(t, err)
		assert.Equal(t, "localhost", connInfo.Host)
		assert.Equal(t, 18091, connInfo.Port)
		assert.Equal(t, "couchbase", connInfo.Protocol)
		assert.Equal(t, "orders", connInfo.Metadata["bucket"])
	})

	t.Run("BucketCreated", func(t *testing.T) {
		output, err := dockerClient.ExecInContainer(ctx, plugin.containerID, &docker.ExecConfig{
			Cmd: []string{
				couchbaseCLIBin, "bucket-list",
				"--cluster", clusterEndpoint,
				"--username", defaultUsername,
				"--password", defaultPassword,
			},
			AttachStdout: true,
			AttachStderr: true,
		})
		require.NoError(t, err, "Failed to list buckets: %s", output)
		assert.True(t, strings.Contains(output, "orders"), "should list the 'orders' bucket")
	})

	t.Run("GetLogs", func(t *testing.T) {
		logs, err := plugin.GetLogs(ctx, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, logs, "Should have logs")
	})

	t.Run("Stop", func(t *testing.T) {
		err := plugin.Stop(ctx)
		require.NoError(t, err, "Failed to stop Couchbase")
	})
}
