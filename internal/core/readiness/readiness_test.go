package readiness

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

func TestWaitHTTP_ReadyOnAnyStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	assert.NoError(t, WaitHTTP(context.Background(), srv.URL+"/", time.Second))
}

func TestWaitHTTP_WaitsUntilTheServerListens(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())

	go func() {
		time.Sleep(100 * time.Millisecond)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return
		}
		_ = http.Serve(ln, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	}()

	assert.NoError(t, waitHTTP(context.Background(), "http://"+addr+"/", 3*time.Second, 20*time.Millisecond))
}

func TestWaitHTTP_Timeout(t *testing.T) {
	err := waitHTTP(context.Background(), "http://127.0.0.1:1/", 50*time.Millisecond, 10*time.Millisecond)
	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrServiceNotReady))
}

func TestWaitHTTP_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := waitHTTP(ctx, "http://127.0.0.1:1/", time.Minute, time.Hour)
	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrServiceNotReady))
}

func TestWaitHTTP_InvalidURL(t *testing.T) {
	err := WaitHTTP(context.Background(), "http://bad host/", time.Second)
	require.Error(t, err)
	assert.True(t, gtErrors.Is(err, gtErrors.ErrInvalidArgument))
}
