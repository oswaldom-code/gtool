//go:build integration
// +build integration

package kafka

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/pkg/logger"
)

func TestKafkaIntegration(t *testing.T) {
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

	plugin := NewKafkaPlugin(dockerClient, log.Logger)
	require.NotNil(t, plugin)

	config := map[string]interface{}{
		"image":      defaultImage,
		"port":       "19092",
		"partitions": float64(2),
		"topics":     []interface{}{"orders", "payments"},
	}

	log.Info("launching Kafka for integration test")
	err = plugin.Launch(ctx, config)
	require.NoError(t, err, "Failed to launch Kafka")

	defer func() {
		log.Info("cleaning up Kafka container")
		if err := plugin.Stop(ctx); err != nil {
			t.Logf("Failed to stop Kafka: %v", err)
		}
	}()

	t.Run("IsReady", func(t *testing.T) {
		ready, err := plugin.IsReady(ctx)
		require.NoError(t, err)
		assert.True(t, ready, "Kafka should be ready")
	})

	t.Run("GetConnectionInfo", func(t *testing.T) {
		connInfo, err := plugin.GetConnectionInfo()
		require.NoError(t, err)
		assert.Equal(t, "localhost", connInfo.Host)
		assert.Equal(t, 19092, connInfo.Port)
		assert.Equal(t, "kafka", connInfo.Protocol)
		assert.Equal(t, "localhost:19092", connInfo.Metadata["bootstrap-servers"])
	})

	t.Run("TopicsCreated", func(t *testing.T) {
		output, err := dockerClient.ExecInContainer(ctx, plugin.containerID, &docker.ExecConfig{
			Cmd:          []string{kafkaTopicsBin, "--bootstrap-server", internalBootstrap, "--list"},
			AttachStdout: true,
			AttachStderr: true,
		})
		require.NoError(t, err, "Failed to list topics: %s", output)
		assert.True(t, strings.Contains(output, "orders"), "should list 'orders' topic")
		assert.True(t, strings.Contains(output, "payments"), "should list 'payments' topic")
	})

	t.Run("GetLogs", func(t *testing.T) {
		logs, err := plugin.GetLogs(ctx, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, logs, "Should have logs")
	})

	t.Run("Stop", func(t *testing.T) {
		err := plugin.Stop(ctx)
		require.NoError(t, err, "Failed to stop Kafka")
	})
}
