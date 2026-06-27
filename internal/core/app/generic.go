package app

import (
	"net"
	"strconv"

	"github.com/oswaldo-montano/gtool/internal/infra/process"
	"go.uber.org/zap"
)

// GenericLauncher runs an arbitrary local executable or script as the
// application under test, with a TCP readiness probe.
type GenericLauncher struct {
	*processLauncher
}

// NewGenericLauncher creates a generic launcher backed by the given process
// manager.
func NewGenericLauncher(procMgr *process.Manager, logger *zap.Logger) *GenericLauncher {
	return &GenericLauncher{newProcessLauncher("generic", procMgr, logger, tcpProbe)}
}

// tcpProbe reports whether a TCP connection to the port succeeds.
func tcpProbe(port int) bool {
	addr := net.JoinHostPort("localhost", strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, probeTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
