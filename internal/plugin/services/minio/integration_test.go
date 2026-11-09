//go:build integration

package minio

import (
	"context"
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/oswaldom-code/gtool/internal/infra/docker"
)

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

func TestMinIOIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	ctx := context.Background()
	dockerClient, err := docker.NewClient(zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = dockerClient.Close() })
	require.NoError(t, dockerClient.Ping(ctx), "Docker daemon not available")

	port := freePort(t)
	p := NewMinIOPlugin(dockerClient, zap.NewNop())
	require.NoError(t, p.Launch(ctx, map[string]interface{}{"port": port}))
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			_ = p.Stop(context.Background())
		}
	})

	ready, err := p.IsReady(ctx)
	require.NoError(t, err)
	assert.True(t, ready)

	info, err := p.GetConnectionInfo()
	require.NoError(t, err)
	assert.Equal(t, port, strconv.Itoa(info.Port))

	id := p.containerID
	require.NoError(t, p.Stop(ctx))
	stopped = true
	_, err = dockerClient.IsContainerRunning(ctx, id)
	assert.Error(t, err, "the container must be removed")
}
