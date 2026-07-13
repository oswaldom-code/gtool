//go:build integration
// +build integration

package postgresql

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/pkg/logger"
)

func TestPostgreSQLIntegration(t *testing.T) {
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

	plugin := NewPostgreSQLPlugin(dockerClient, log.Logger)
	require.NotNil(t, plugin)

	config := map[string]interface{}{
		"image":        "postgres:16-alpine",
		"port":         "15432",
		"user":         "integrationtest",
		"password":     "testpass123",
		"database":     "testdb",
		"scripts-path": "../../../../test/component/mocks-data/postgresql",
	}

	log.Info("launching PostgreSQL for integration test")
	err = plugin.Launch(ctx, config)
	require.NoError(t, err, "Failed to launch PostgreSQL")

	defer func() {
		log.Info("cleaning up PostgreSQL container")
		if err := plugin.Stop(ctx); err != nil {
			t.Logf("Failed to stop PostgreSQL: %v", err)
		}
	}()

	t.Run("IsReady", func(t *testing.T) {
		ready, err := plugin.IsReady(ctx)
		require.NoError(t, err)
		assert.True(t, ready, "PostgreSQL should be ready")
	})

	t.Run("GetConnectionInfo", func(t *testing.T) {
		connInfo, err := plugin.GetConnectionInfo()
		require.NoError(t, err)
		assert.NotNil(t, connInfo)
		assert.Equal(t, "localhost", connInfo.Host)
		assert.Equal(t, 15432, connInfo.Port)
		assert.Equal(t, "postgresql", connInfo.Protocol)
		assert.Equal(t, "integrationtest", connInfo.Metadata["user"])
		assert.Equal(t, "testpass123", connInfo.Metadata["password"])
		assert.Equal(t, "testdb", connInfo.Metadata["database"])
	})

	t.Run("GetLogs", func(t *testing.T) {
		logs, err := plugin.GetLogs(ctx, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, logs, "Should have logs")
	})

	t.Run("VerifyScripts", func(t *testing.T) {
		time.Sleep(2 * time.Second)

		output, err := dockerClient.ExecInContainer(ctx, plugin.containerID, &docker.ExecConfig{
			Cmd: []string{
				"psql",
				"-U", "integrationtest",
				"-d", "testdb",
				"-c", "SELECT COUNT(*) FROM users;",
			},
			AttachStdout: true,
			AttachStderr: true,
			Env: []string{
				"PGPASSWORD=testpass123",
			},
		})
		require.NoError(t, err, "Failed to query users table: %s", output)
		assert.Contains(t, output, "2", "Should have 2 users from init script")

		output, err = dockerClient.ExecInContainer(ctx, plugin.containerID, &docker.ExecConfig{
			Cmd: []string{
				"psql",
				"-U", "integrationtest",
				"-d", "testdb",
				"-c", "SELECT COUNT(*) FROM products;",
			},
			AttachStdout: true,
			AttachStderr: true,
			Env: []string{
				"PGPASSWORD=testpass123",
			},
		})
		require.NoError(t, err, "Failed to query products table: %s", output)
		assert.Contains(t, output, "2", "Should have 2 products from init script")
	})

	t.Run("Stop", func(t *testing.T) {
		err := plugin.Stop(ctx)
		require.NoError(t, err, "Failed to stop PostgreSQL")

		running, err := dockerClient.IsContainerRunning(ctx, plugin.containerID)
		if plugin.containerID != "" {
			require.Error(t, err)
		}
		assert.False(t, running, "Container should not be running")
	})
}

func TestPostgreSQLIntegrationWithoutScripts(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	log := zap.NewNop()

	dockerClient, err := docker.NewClient(log)
	require.NoError(t, err)
	defer dockerClient.Close()

	ctx := context.Background()

	plugin := NewPostgreSQLPlugin(dockerClient, log)

	config := map[string]interface{}{
		"port":     "25432",
		"user":     "testuser",
		"password": "testpass",
		"database": "testdb",
	}

	err = plugin.Launch(ctx, config)
	require.NoError(t, err)

	defer plugin.Stop(ctx)

	ready, err := plugin.IsReady(ctx)
	require.NoError(t, err)
	assert.True(t, ready)

	err = plugin.Stop(ctx)
	require.NoError(t, err)
}
