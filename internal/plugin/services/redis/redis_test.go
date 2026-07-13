package redis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewRedisPlugin(t *testing.T) {
	logger := zap.NewNop()
	plugin := NewRedisPlugin(nil, logger)

	assert.NotNil(t, plugin)
	assert.Equal(t, "redis", plugin.Name())
}

func TestName(t *testing.T) {
	plugin := NewRedisPlugin(nil, nil)
	assert.Equal(t, "redis", plugin.Name())
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]interface{}
		want        *RedisConfig
		wantErr     bool
		errContains string
	}{
		{
			name:  "default config",
			input: map[string]interface{}{},
			want: &RedisConfig{
				Image: defaultImage,
				Port:  defaultPort,
			},
			wantErr: false,
		},
		{
			name: "custom config with string port",
			input: map[string]interface{}{
				"image": "redis:6",
				"port":  "6380",
			},
			want: &RedisConfig{
				Image: "redis:6",
				Port:  "6380",
			},
			wantErr: false,
		},
		{
			name: "custom config with numeric port",
			input: map[string]interface{}{
				"port": float64(6380),
			},
			want: &RedisConfig{
				Image: defaultImage,
				Port:  "6380",
			},
			wantErr: false,
		},
		{
			name: "with password",
			input: map[string]interface{}{
				"password": "secret",
			},
			want: &RedisConfig{
				Image:    defaultImage,
				Port:     defaultPort,
				Password: "secret",
			},
			wantErr: false,
		},
		{
			name: "with container name",
			input: map[string]interface{}{
				"container-name": "my-redis",
			},
			want: &RedisConfig{
				Image:         defaultImage,
				Port:          defaultPort,
				ContainerName: "my-redis",
			},
			wantErr: false,
		},
		{
			name: "partial config",
			input: map[string]interface{}{
				"image": "redis:7",
			},
			want: &RedisConfig{
				Image: "redis:7",
				Port:  defaultPort,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := NewRedisPlugin(nil, nil)
			got, err := plugin.parseConfig(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want.Image, got.Image)
			assert.Equal(t, tt.want.Port, got.Port)
			assert.Equal(t, tt.want.Password, got.Password)
			if tt.want.ContainerName != "" {
				assert.Equal(t, tt.want.ContainerName, got.ContainerName)
			} else {
				// Container name should be auto-generated
				assert.NotEmpty(t, got.ContainerName)
				assert.Contains(t, got.ContainerName, containerNamePrefix)
			}
		})
	}
}

func TestGetConnectionInfo(t *testing.T) {
	tests := []struct {
		name        string
		config      *RedisConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "valid config",
			config: &RedisConfig{
				Port:     "6379",
				Password: "testpass",
			},
			wantErr: false,
		},
		{
			name:        "no config",
			config:      nil,
			wantErr:     true,
			errContains: "not launched",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := NewRedisPlugin(nil, nil)
			plugin.config = tt.config

			got, err := plugin.GetConnectionInfo()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, got)
			assert.Equal(t, "localhost", got.Host)
			assert.Equal(t, mustParsePort(tt.config.Port), got.Port)
			assert.Equal(t, "redis", got.Protocol)
			assert.Equal(t, tt.config.Password, got.Metadata["password"])
		})
	}
}

func TestMustParsePort(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "standard port",
			input: "6379",
			want:  6379,
		},
		{
			name:  "custom port",
			input: "6380",
			want:  6380,
		},
		{
			name:  "zero returns zero",
			input: "0",
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustParsePort(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsReady_NotStarted(t *testing.T) {
	plugin := NewRedisPlugin(nil, nil)

	// Should return error when container not started
	ready, err := plugin.IsReady(nil)
	assert.False(t, ready)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestStop_NoContainer(t *testing.T) {
	plugin := NewRedisPlugin(nil, zap.NewNop())

	// Should not error when no container to stop
	err := plugin.Stop(nil)
	assert.NoError(t, err)
}
