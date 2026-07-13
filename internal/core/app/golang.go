package app

import (
	"fmt"
	"net/http"

	"github.com/oswaldom-code/gtool/internal/infra/process"
	"go.uber.org/zap"
)

// golangProbeClient is used by the HTTP readiness probe.
var golangProbeClient = &http.Client{Timeout: probeTimeout}

// GolangLauncher runs a compiled Go application as the application under test,
// with an HTTP readiness probe (any HTTP response means the server is up).
type GolangLauncher struct {
	*processLauncher
}

// NewGolangLauncher creates a Go launcher backed by the given process manager.
func NewGolangLauncher(procMgr *process.Manager, logger *zap.Logger) *GolangLauncher {
	return &GolangLauncher{newProcessLauncher("golang", procMgr, logger, httpProbe)}
}

// httpProbe reports whether the port answers an HTTP request. Any response
// (including 4xx/5xx) means the server is accepting requests; only a transport
// error counts as not ready.
func httpProbe(port int) bool {
	resp, err := golangProbeClient.Get(fmt.Sprintf("http://localhost:%d/", port))
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}
