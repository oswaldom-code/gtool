//go:build integration
// +build integration

package floci

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/pkg/logger"
)

func TestFlociIntegration(t *testing.T) {
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

	plugin := NewFlociPlugin(dockerClient, log.Logger)
	require.NotNil(t, plugin)

	const endpoint = "http://localhost:14566"
	config := map[string]interface{}{
		"image":   defaultImage,
		"port":    "14566",
		"buckets": []interface{}{"uploads"},
		"queues":  []interface{}{"orders"},
	}

	log.Info("launching Floci emulator for integration test")
	err = plugin.Launch(ctx, config)
	require.NoError(t, err, "Failed to launch Floci")

	defer func() {
		log.Info("cleaning up Floci container")
		if err := plugin.Stop(ctx); err != nil {
			t.Logf("Failed to stop Floci: %v", err)
		}
	}()

	t.Run("IsReady", func(t *testing.T) {
		ready, err := plugin.IsReady(ctx)
		require.NoError(t, err)
		assert.True(t, ready, "Floci should be ready")
	})

	t.Run("BucketCreated", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPut, endpoint+"/uploads/hello.json", strings.NewReader(`{"ok":true}`))
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		resp, err = http.Get(endpoint + "/uploads/hello.json")
		require.NoError(t, err)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, `{"ok":true}`, string(body))
	})

	t.Run("QueueCreated", func(t *testing.T) {
		resp, err := http.PostForm(endpoint+"/", url.Values{"Action": {"ListQueues"}})
		require.NoError(t, err)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "orders")
	})

	t.Run("Stop", func(t *testing.T) {
		err := plugin.Stop(ctx)
		require.NoError(t, err, "Failed to stop Floci")
	})
}
