package couchbase

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewCouchbasePlugin(t *testing.T) {
	p := NewCouchbasePlugin(nil, zap.NewNop())

	assert.NotNil(t, p)
	assert.Equal(t, "couchbase", p.Name())
}

func TestName(t *testing.T) {
	p := NewCouchbasePlugin(nil, nil)
	assert.Equal(t, "couchbase", p.Name())
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]interface{}
		want *CouchbaseConfig
	}{
		{
			name: "default config",
			in:   map[string]interface{}{},
			want: &CouchbaseConfig{
				Image:        defaultImage,
				AdminPort:    defaultAdminPort,
				DataPort:     defaultDataPort,
				Username:     defaultUsername,
				Password:     defaultPassword,
				Bucket:       defaultBucket,
				BucketRAMMB:  defaultBucketRAMMB,
				ClusterRAMMB: defaultClusterRAMMB,
				Services:     defaultServices,
			},
		},
		{
			name: "full custom config",
			in: map[string]interface{}{
				"image":          "couchbase:community-7.2.0",
				"admin-port":     "18091",
				"data-port":      "21210",
				"username":       "admin",
				"password":       "secret",
				"bucket":         "orders",
				"services":       "data,query",
				"bucket-ram-mb":  float64(256),
				"cluster-ram-mb": float64(512),
			},
			want: &CouchbaseConfig{
				Image:        "couchbase:community-7.2.0",
				AdminPort:    "18091",
				DataPort:     "21210",
				Username:     "admin",
				Password:     "secret",
				Bucket:       "orders",
				Services:     "data,query",
				BucketRAMMB:  256,
				ClusterRAMMB: 512,
			},
		},
		{
			name: "numeric admin port",
			in: map[string]interface{}{
				"admin-port": float64(18091),
			},
			want: &CouchbaseConfig{
				Image:        defaultImage,
				AdminPort:    "18091",
				DataPort:     defaultDataPort,
				Username:     defaultUsername,
				Password:     defaultPassword,
				Bucket:       defaultBucket,
				BucketRAMMB:  defaultBucketRAMMB,
				ClusterRAMMB: defaultClusterRAMMB,
				Services:     defaultServices,
			},
		},
		{
			name: "non-positive ram falls back to defaults",
			in: map[string]interface{}{
				"bucket-ram-mb":  float64(0),
				"cluster-ram-mb": float64(-1),
			},
			want: &CouchbaseConfig{
				Image:        defaultImage,
				AdminPort:    defaultAdminPort,
				DataPort:     defaultDataPort,
				Username:     defaultUsername,
				Password:     defaultPassword,
				Bucket:       defaultBucket,
				BucketRAMMB:  defaultBucketRAMMB,
				ClusterRAMMB: defaultClusterRAMMB,
				Services:     defaultServices,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewCouchbasePlugin(nil, nil)
			got, err := p.parseConfig(tt.in)

			require.NoError(t, err)
			assert.Equal(t, tt.want.Image, got.Image)
			assert.Equal(t, tt.want.AdminPort, got.AdminPort)
			assert.Equal(t, tt.want.DataPort, got.DataPort)
			assert.Equal(t, tt.want.Username, got.Username)
			assert.Equal(t, tt.want.Password, got.Password)
			assert.Equal(t, tt.want.Bucket, got.Bucket)
			assert.Equal(t, tt.want.Services, got.Services)
			assert.Equal(t, tt.want.BucketRAMMB, got.BucketRAMMB)
			assert.Equal(t, tt.want.ClusterRAMMB, got.ClusterRAMMB)
			assert.Contains(t, got.ContainerName, containerNamePrefix)
		})
	}
}

func TestParseConfig_CustomContainerName(t *testing.T) {
	p := NewCouchbasePlugin(nil, nil)
	got, err := p.parseConfig(map[string]interface{}{"container-name": "my-cb"})

	require.NoError(t, err)
	assert.Equal(t, "my-cb", got.ContainerName)
}

func TestGetConnectionInfo(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		p := NewCouchbasePlugin(nil, nil)
		p.config = &CouchbaseConfig{
			AdminPort: "8091",
			Username:  "Administrator",
			Password:  "password",
			Bucket:    "default",
		}

		got, err := p.GetConnectionInfo()

		require.NoError(t, err)
		assert.Equal(t, "localhost", got.Host)
		assert.Equal(t, 8091, got.Port)
		assert.Equal(t, "couchbase", got.Protocol)
		assert.Equal(t, "Administrator", got.Metadata["username"])
		assert.Equal(t, "password", got.Metadata["password"])
		assert.Equal(t, "default", got.Metadata["bucket"])
		assert.Equal(t, "couchbase://localhost", got.Metadata["connection-string"])
	})

	t.Run("not launched", func(t *testing.T) {
		p := NewCouchbasePlugin(nil, nil)

		_, err := p.GetConnectionInfo()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not launched")
	})
}

func TestIsReady_NotStarted(t *testing.T) {
	p := NewCouchbasePlugin(nil, nil)

	ready, err := p.IsReady(nil)

	assert.False(t, ready)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestStop_NoContainer(t *testing.T) {
	p := NewCouchbasePlugin(nil, zap.NewNop())

	err := p.Stop(nil)
	assert.NoError(t, err)
}

func TestStringOrFloat(t *testing.T) {
	cfg := map[string]interface{}{
		"s": "value",
		"f": float64(8091),
		"e": "",
	}

	assert.Equal(t, "value", stringOrFloat(cfg, "s", "fallback"))
	assert.Equal(t, "8091", stringOrFloat(cfg, "f", "fallback"))
	assert.Equal(t, "fallback", stringOrFloat(cfg, "e", "fallback"))
	assert.Equal(t, "fallback", stringOrFloat(cfg, "missing", "fallback"))
}

func TestIntOrFloat(t *testing.T) {
	cfg := map[string]interface{}{
		"f":    float64(256),
		"i":    512,
		"zero": float64(0),
	}

	assert.Equal(t, 256, intOrFloat(cfg, "f", 10))
	assert.Equal(t, 512, intOrFloat(cfg, "i", 10))
	assert.Equal(t, 10, intOrFloat(cfg, "zero", 10))
	assert.Equal(t, 10, intOrFloat(cfg, "missing", 10))
}

func TestMustParsePort(t *testing.T) {
	assert.Equal(t, 8091, mustParsePort("8091"))
	assert.Equal(t, 0, mustParsePort("0"))
}
