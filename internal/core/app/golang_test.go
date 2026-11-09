package app

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oswaldom-code/gtool/internal/infra/process"
	"github.com/oswaldom-code/gtool/internal/plugin"
)

func newGolang() *GolangLauncher {
	return NewGolangLauncher(process.NewManager(nil), nil)
}

func TestGolangLauncher_Technology(t *testing.T) {
	assert.Equal(t, "golang", newGolang().Technology())
}

func TestGolangLauncher_LaunchAndStop(t *testing.T) {
	ctx := context.Background()
	g := newGolang()

	require.NoError(t, g.Launch(ctx, &plugin.AppConfig{BinaryPath: writeScript(t, "sleep 30")}))

	pid, err := g.GetPID()
	require.NoError(t, err)
	assert.Greater(t, pid, 0)

	require.NoError(t, g.Stop(ctx))
}

func TestGolangLauncher_IsReady_HTTPProbe(t *testing.T) {
	ctx := context.Background()

	// Server replies 404, which still proves the HTTP server is up.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	port := srv.Listener.Addr().(*net.TCPAddr).Port

	g := newGolang()
	require.NoError(t, g.Launch(ctx, &plugin.AppConfig{
		BinaryPath: writeScript(t, "sleep 30"),
		Port:       port,
	}))
	defer func() { assert.NoError(t, g.Stop(ctx)) }()

	ready, err := g.IsReady(ctx)
	require.NoError(t, err)
	assert.True(t, ready, "any HTTP response means the server is up")

	srv.Close()

	ready, err = g.IsReady(ctx)
	require.NoError(t, err)
	assert.False(t, ready, "no server should report not ready")
}

func TestGolangLauncher_IsReady_NoPort(t *testing.T) {
	ctx := context.Background()
	g := newGolang()
	require.NoError(t, g.Launch(ctx, &plugin.AppConfig{BinaryPath: writeScript(t, "sleep 30")}))
	defer func() { assert.NoError(t, g.Stop(ctx)) }()

	ready, err := g.IsReady(ctx)
	require.NoError(t, err)
	assert.True(t, ready)
}

func TestHTTPProbe_NoServer(t *testing.T) {
	// Port 1 (tcpmux) has nothing listening on a dev or CI host, so the probe
	// is refused deterministically instead of racing for a freed ephemeral port.
	assert.False(t, httpProbe(1))
}
