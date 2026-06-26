//go:build integration
// +build integration

package pubsub

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

func TestPubSubIntegration(t *testing.T) {
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

	plugin := NewPubSubPlugin(dockerClient, log.Logger)
	require.NotNil(t, plugin)

	const port = "18085"
	config := map[string]interface{}{
		"image":      defaultImage,
		"port":       port,
		"project-id": "test-project",
		"topics":     []interface{}{"orders"},
		"subscriptions": []interface{}{
			map[string]interface{}{"name": "orders-sub", "topic": "orders"},
		},
	}

	log.Info("launching Pub/Sub emulator for integration test")
	err = plugin.Launch(ctx, config)
	require.NoError(t, err, "Failed to launch Pub/Sub")

	defer func() {
		log.Info("cleaning up Pub/Sub container")
		if err := plugin.Stop(ctx); err != nil {
			t.Logf("Failed to stop Pub/Sub: %v", err)
		}
	}()

	t.Run("IsReady", func(t *testing.T) {
		ready, err := plugin.IsReady(ctx)
		require.NoError(t, err)
		assert.True(t, ready, "Pub/Sub should be ready")
	})

	t.Run("GetConnectionInfo", func(t *testing.T) {
		connInfo, err := plugin.GetConnectionInfo()
		require.NoError(t, err)
		assert.Equal(t, "localhost", connInfo.Host)
		assert.Equal(t, 18085, connInfo.Port)
		assert.Equal(t, "test-project", connInfo.Metadata["project-id"])
	})

	t.Run("TopicCreated", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/v1/projects/test-project/topics/orders", port))
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "topic 'orders' should exist")
	})

	t.Run("SubscriptionCreated", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/v1/projects/test-project/subscriptions/orders-sub", port))
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "subscription 'orders-sub' should exist")
	})

	t.Run("Stop", func(t *testing.T) {
		err := plugin.Stop(ctx)
		require.NoError(t, err, "Failed to stop Pub/Sub")
	})
}
