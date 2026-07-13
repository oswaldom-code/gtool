package minio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewMinIOPlugin(t *testing.T) {
	logger := zap.NewNop()
	plugin := NewMinIOPlugin(nil, logger)

	assert.NotNil(t, plugin)
	assert.Equal(t, "minio", plugin.Name())
}

func TestName(t *testing.T) {
	plugin := NewMinIOPlugin(nil, nil)
	assert.Equal(t, "minio", plugin.Name())
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]interface{}
		want        *MinIOConfig
		wantErr     bool
		errContains string
	}{
		{
			name:  "default config",
			input: map[string]interface{}{},
			want: &MinIOConfig{
				Image:       defaultImage,
				Port:        defaultPort,
				ConsolePort: defaultConsolePort,
				AccessKey:   defaultAccessKey,
				SecretKey:   defaultSecretKey,
			},
			wantErr: false,
		},
		{
			name: "custom config with string port",
			input: map[string]interface{}{
				"image":        "minio/minio:RELEASE.2024-01-01",
				"port":         "9100",
				"console-port": "9101",
				"access-key":   "myaccess",
				"secret-key":   "mysecret",
			},
			want: &MinIOConfig{
				Image:       "minio/minio:RELEASE.2024-01-01",
				Port:        "9100",
				ConsolePort: "9101",
				AccessKey:   "myaccess",
				SecretKey:   "mysecret",
			},
			wantErr: false,
		},
		{
			name: "custom config with numeric port",
			input: map[string]interface{}{
				"port":         float64(9100),
				"console-port": float64(9101),
			},
			want: &MinIOConfig{
				Image:       defaultImage,
				Port:        "9100",
				ConsolePort: "9101",
				AccessKey:   defaultAccessKey,
				SecretKey:   defaultSecretKey,
			},
			wantErr: false,
		},
		{
			name: "custom access and secret key",
			input: map[string]interface{}{
				"access-key": "customaccess",
				"secret-key": "customsecret",
			},
			want: &MinIOConfig{
				Image:       defaultImage,
				Port:        defaultPort,
				ConsolePort: defaultConsolePort,
				AccessKey:   "customaccess",
				SecretKey:   "customsecret",
			},
			wantErr: false,
		},
		{
			name: "with container name",
			input: map[string]interface{}{
				"container-name": "my-minio",
			},
			want: &MinIOConfig{
				Image:         defaultImage,
				Port:          defaultPort,
				ConsolePort:   defaultConsolePort,
				AccessKey:     defaultAccessKey,
				SecretKey:     defaultSecretKey,
				ContainerName: "my-minio",
			},
			wantErr: false,
		},
		{
			name: "partial config",
			input: map[string]interface{}{
				"port":       "9200",
				"access-key": "partialaccess",
			},
			want: &MinIOConfig{
				Image:       defaultImage,
				Port:        "9200",
				ConsolePort: defaultConsolePort,
				AccessKey:   "partialaccess",
				SecretKey:   defaultSecretKey,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := NewMinIOPlugin(nil, nil)
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
			assert.Equal(t, tt.want.ConsolePort, got.ConsolePort)
			assert.Equal(t, tt.want.AccessKey, got.AccessKey)
			assert.Equal(t, tt.want.SecretKey, got.SecretKey)
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
		config      *MinIOConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "valid config",
			config: &MinIOConfig{
				Port:        "9000",
				ConsolePort: "9001",
				AccessKey:   "testaccess",
				SecretKey:   "testsecret",
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
			plugin := NewMinIOPlugin(nil, nil)
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
			assert.Equal(t, "s3", got.Protocol)
			assert.Equal(t, tt.config.AccessKey, got.Metadata["accessKey"])
			assert.Equal(t, tt.config.SecretKey, got.Metadata["secretKey"])
			assert.Equal(t, "http://localhost:"+tt.config.Port, got.Metadata["endpoint"])
			assert.Equal(t, tt.config.ConsolePort, got.Metadata["consolePort"])
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
			input: "9000",
			want:  9000,
		},
		{
			name:  "custom port",
			input: "9100",
			want:  9100,
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
	plugin := NewMinIOPlugin(nil, nil)

	// Should return error when container not started
	ready, err := plugin.IsReady(nil)
	assert.False(t, ready)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestStop_NoContainer(t *testing.T) {
	plugin := NewMinIOPlugin(nil, zap.NewNop())

	// Should not error when no container to stop
	err := plugin.Stop(nil)
	assert.NoError(t, err)
}
