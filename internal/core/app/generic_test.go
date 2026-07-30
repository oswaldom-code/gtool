package app

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oswaldom-code/gtool/internal/infra/process"
	"github.com/oswaldom-code/gtool/internal/plugin"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

// writeScript creates an executable shell script running body and returns its path.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.sh")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	return path
}

func newLauncher() *GenericLauncher {
	return NewGenericLauncher(process.NewManager(nil), nil)
}

func TestGenericLauncher_Technology(t *testing.T) {
	assert.Equal(t, "generic", newLauncher().Technology())
}

func TestGenericLauncher_Launch_NoBinary(t *testing.T) {
	g := newLauncher()

	err := g.Launch(context.Background(), &plugin.AppConfig{})

	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrInvalidArgument))
}

func TestGenericLauncher_Launch_InvalidCommand(t *testing.T) {
	g := newLauncher()

	err := g.Launch(context.Background(), &plugin.AppConfig{
		BinaryName: "this-binary-does-not-exist-gtool",
	})

	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrProcessFailed))
}

func TestGenericLauncher_LaunchAndGetPID(t *testing.T) {
	ctx := context.Background()
	g := newLauncher()

	require.NoError(t, g.Launch(ctx, &plugin.AppConfig{BinaryPath: writeScript(t, "sleep 30")}))

	pid, err := g.GetPID()
	require.NoError(t, err)
	assert.Greater(t, pid, 0)

	require.NoError(t, g.Stop(ctx))

	_, err = g.GetPID()
	assert.Error(t, err, "GetPID should fail after stop")
}

func TestGenericLauncher_IsReady_NotLaunched(t *testing.T) {
	g := newLauncher()

	ready, err := g.IsReady(context.Background())

	assert.False(t, ready)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not launched")
}

func TestGenericLauncher_IsReady_NoPort(t *testing.T) {
	ctx := context.Background()
	g := newLauncher()
	require.NoError(t, g.Launch(ctx, &plugin.AppConfig{BinaryPath: writeScript(t, "sleep 30")}))
	defer g.Stop(ctx)

	ready, err := g.IsReady(ctx)
	require.NoError(t, err)
	assert.True(t, ready, "a running process with no port should be ready")
}

func TestGenericLauncher_IsReady_ProcessExited(t *testing.T) {
	ctx := context.Background()
	g := newLauncher()
	require.NoError(t, g.Launch(ctx, &plugin.AppConfig{BinaryName: "true"}))

	require.Eventually(t, func() bool {
		ready, err := g.IsReady(ctx)
		return err == nil && !ready
	}, 2*time.Second, 10*time.Millisecond, "an exited process should report not ready")
}

func TestGenericLauncher_IsReady_PortProbe(t *testing.T) {
	ctx := context.Background()

	ln, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port

	g := newLauncher()
	require.NoError(t, g.Launch(ctx, &plugin.AppConfig{
		BinaryPath: writeScript(t, "sleep 30"),
		Port:       port,
	}))
	defer g.Stop(ctx)

	ready, err := g.IsReady(ctx)
	require.NoError(t, err)
	assert.True(t, ready, "open port should report ready")

	require.NoError(t, ln.Close())

	ready, err = g.IsReady(ctx)
	require.NoError(t, err)
	assert.False(t, ready, "closed port should report not ready")
}

func TestGenericLauncher_Stop_NotLaunched(t *testing.T) {
	assert.NoError(t, newLauncher().Stop(context.Background()))
}

func TestGenericLauncher_Restart(t *testing.T) {
	ctx := context.Background()
	g := newLauncher()
	require.NoError(t, g.Launch(ctx, &plugin.AppConfig{BinaryPath: writeScript(t, "sleep 30")}))

	pid1, err := g.GetPID()
	require.NoError(t, err)

	require.NoError(t, g.Restart(ctx))
	defer g.Stop(ctx)

	pid2, err := g.GetPID()
	require.NoError(t, err)
	assert.NotEqual(t, pid1, pid2, "restart should produce a new process")
}

func TestGenericLauncher_Restart_NotLaunched(t *testing.T) {
	err := newLauncher().Restart(context.Background())
	require.Error(t, err)
}
