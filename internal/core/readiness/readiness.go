// Package readiness waits for an HTTP service to accept requests.
package readiness

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

const (
	pollInterval = 500 * time.Millisecond
	probeTimeout = 2 * time.Second
)

// WaitHTTP polls url until it answers with any HTTP status. A published
// container port accepts TCP connections before the app listens (Docker's
// userland proxy), so a TCP check is not enough.
func WaitHTTP(ctx context.Context, url string, timeout time.Duration) error {
	return waitHTTP(ctx, url, timeout, pollInterval)
}

func waitHTTP(ctx context.Context, url string, timeout, interval time.Duration) error {
	client := &http.Client{Timeout: probeTimeout}
	deadline := time.Now().Add(timeout)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, fmt.Sprintf("invalid URL %s", url))
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return gtErrors.Wrap(err, gtErrors.ErrServiceNotReady, fmt.Sprintf("%s not reachable after %s", url, timeout))
		}
		select {
		case <-ctx.Done():
			return gtErrors.Wrap(ctx.Err(), gtErrors.ErrServiceNotReady, "cancelled while waiting for "+url)
		case <-time.After(interval):
		}
	}
}
