package floci

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewFlociPlugin(t *testing.T) {
	p := NewFlociPlugin(nil, zap.NewNop())

	assert.NotNil(t, p)
	assert.Equal(t, "floci", p.Name())
	assert.NotNil(t, p.httpClient)
}

func TestParseConfig(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		p := NewFlociPlugin(nil, nil)
		got, err := p.parseConfig(map[string]interface{}{})

		require.NoError(t, err)
		assert.Equal(t, defaultImage, got.Image)
		assert.Equal(t, defaultPort, got.Port)
		assert.Equal(t, defaultRegion, got.Region)
		assert.Empty(t, got.Buckets)
		assert.Empty(t, got.Queues)
		assert.Contains(t, got.ContainerName, containerNamePrefix)
	})

	t.Run("resources, region and numeric port", func(t *testing.T) {
		p := NewFlociPlugin(nil, nil)
		got, err := p.parseConfig(map[string]interface{}{
			"port":    float64(14566),
			"region":  "eu-west-1",
			"buckets": []interface{}{"reports", "", 42, "exports"},
			"queues":  []interface{}{"orders"},
		})

		require.NoError(t, err)
		assert.Equal(t, "14566", got.Port)
		assert.Equal(t, "eu-west-1", got.Region)
		assert.Equal(t, []string{"reports", "42", "exports"}, got.Buckets, "numeric names are names, not ignored")
		assert.Equal(t, []string{"orders"}, got.Queues)
	})

	t.Run("int port and custom container name", func(t *testing.T) {
		p := NewFlociPlugin(nil, nil)
		got, err := p.parseConfig(map[string]interface{}{"port": 4567, "container-name": "my-floci"})

		require.NoError(t, err)
		assert.Equal(t, "4567", got.Port)
		assert.Equal(t, "my-floci", got.ContainerName)
	})
}

// newTestPlugin returns a plugin whose endpoint points at the given server
func newTestPlugin(t *testing.T, srv *httptest.Server, buckets, queues []string) (*FlociPlugin, *FlociConfig) {
	t.Helper()
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	p := NewFlociPlugin(nil, nil)
	p.config = &FlociConfig{Port: u.Port(), Buckets: buckets, Queues: queues}
	return p, p.config
}

func TestCreateResources(t *testing.T) {
	t.Run("creates buckets with PUT and queues with CreateQueue", func(t *testing.T) {
		var mu sync.Mutex
		var calls []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			if r.Method == http.MethodPost {
				require.NoError(t, r.ParseForm())
				calls = append(calls, r.Form.Get("Action")+" "+r.Form.Get("QueueName"))
			} else {
				calls = append(calls, r.Method+" "+r.URL.Path)
			}
		}))
		defer srv.Close()

		p, cfg := newTestPlugin(t, srv, []string{"reports"}, []string{"orders"})
		require.NoError(t, p.createResources(context.Background(), cfg))

		assert.Equal(t, []string{"PUT /reports", "CreateQueue orders"}, calls)
	})

	t.Run("fails on an unexpected status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		p, cfg := newTestPlugin(t, srv, []string{"reports"}, nil)
		err := p.createResources(context.Background(), cfg)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status 500 creating bucket reports")
	})

	t.Run("fails when the emulator is unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		p, cfg := newTestPlugin(t, srv, nil, []string{"orders"})
		srv.Close()

		err := p.createResources(context.Background(), cfg)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create queue orders")
	})
}

func TestGetConnectionInfo(t *testing.T) {
	p := NewFlociPlugin(nil, nil)
	_, err := p.GetConnectionInfo()
	require.Error(t, err)

	p.config = &FlociConfig{Port: "4566", Region: "us-east-1"}
	info, err := p.GetConnectionInfo()

	require.NoError(t, err)
	assert.Equal(t, 4566, info.Port)
	assert.Equal(t, "http://localhost:4566", info.Metadata["endpoint"])
	assert.Equal(t, "us-east-1", info.Metadata["region"])
}

func TestIsReadyWithoutContainer(t *testing.T) {
	p := NewFlociPlugin(nil, nil)
	ready, err := p.IsReady(context.Background())

	assert.False(t, ready)
	require.Error(t, err)
}

func TestStopWithoutDocker(t *testing.T) {
	p := NewFlociPlugin(nil, nil)
	assert.NoError(t, p.Stop(context.Background()))
}
