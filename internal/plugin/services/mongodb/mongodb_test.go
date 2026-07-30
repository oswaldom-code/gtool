package mongodb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewMongoDBPlugin(t *testing.T) {
	logger := zap.NewNop()
	plugin := NewMongoDBPlugin(nil, logger)

	assert.NotNil(t, plugin)
	assert.Equal(t, "mongodb", plugin.Name())
}

func TestName(t *testing.T) {
	plugin := NewMongoDBPlugin(nil, nil)
	assert.Equal(t, "mongodb", plugin.Name())
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]interface{}
		want        *MongoDBConfig
		wantErr     bool
		errContains string
	}{
		{
			name:  "default config",
			input: map[string]interface{}{},
			want: &MongoDBConfig{
				Image:    defaultImage,
				Port:     defaultPort,
				Database: defaultDatabase,
			},
			wantErr: false,
		},
		{
			name: "custom config with string port",
			input: map[string]interface{}{
				"image":    "mongo:6",
				"port":     "27018",
				"database": "mydb",
			},
			want: &MongoDBConfig{
				Image:    "mongo:6",
				Port:     "27018",
				Database: "mydb",
			},
			wantErr: false,
		},
		{
			name: "custom config with numeric port",
			input: map[string]interface{}{
				"port": float64(27018),
			},
			want: &MongoDBConfig{
				Image:    defaultImage,
				Port:     "27018",
				Database: defaultDatabase,
			},
			wantErr: false,
		},
		{
			name: "with database",
			input: map[string]interface{}{
				"database": "customdb",
			},
			want: &MongoDBConfig{
				Image:    defaultImage,
				Port:     defaultPort,
				Database: "customdb",
			},
			wantErr: false,
		},
		{
			name: "with container name",
			input: map[string]interface{}{
				"container-name": "my-mongodb",
			},
			want: &MongoDBConfig{
				Image:         defaultImage,
				Port:          defaultPort,
				Database:      defaultDatabase,
				ContainerName: "my-mongodb",
			},
			wantErr: false,
		},
		{
			name: "partial config",
			input: map[string]interface{}{
				"image": "mongo:5",
			},
			want: &MongoDBConfig{
				Image:    "mongo:5",
				Port:     defaultPort,
				Database: defaultDatabase,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := NewMongoDBPlugin(nil, nil)
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
			assert.Equal(t, tt.want.Database, got.Database)
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
		config      *MongoDBConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "valid config",
			config: &MongoDBConfig{
				Port:     "27017",
				Database: "testdb",
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
			plugin := NewMongoDBPlugin(nil, nil)
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
			assert.Equal(t, "mongodb", got.Protocol)
			assert.Equal(t, tt.config.Database, got.Metadata["database"])
			assert.Equal(t, "mongodb://localhost:"+tt.config.Port, got.Metadata["connectionString"])
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
			input: "27017",
			want:  27017,
		},
		{
			name:  "custom port",
			input: "27018",
			want:  27018,
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
	plugin := NewMongoDBPlugin(nil, nil)

	// Should return error when container not started
	ready, err := plugin.IsReady(nil)
	assert.False(t, ready)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestStop_NoContainer(t *testing.T) {
	plugin := NewMongoDBPlugin(nil, zap.NewNop())

	// Should not error when no container to stop
	err := plugin.Stop(nil)
	assert.NoError(t, err)
}
