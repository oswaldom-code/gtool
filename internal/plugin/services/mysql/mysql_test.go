package mysql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewMySQLPlugin(t *testing.T) {
	logger := zap.NewNop()
	plugin := NewMySQLPlugin(nil, logger)

	assert.NotNil(t, plugin)
	assert.Equal(t, "mysql", plugin.Name())
}

func TestName(t *testing.T) {
	plugin := NewMySQLPlugin(nil, nil)
	assert.Equal(t, "mysql", plugin.Name())
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]interface{}
		want        *MySQLConfig
		wantErr     bool
		errContains string
	}{
		{
			name:  "default config",
			input: map[string]interface{}{},
			want: &MySQLConfig{
				Image:        defaultImage,
				Port:         defaultPort,
				RootPassword: defaultRootPassword,
				Database:     defaultDatabase,
			},
			wantErr: false,
		},
		{
			name: "custom config with string port",
			input: map[string]interface{}{
				"image":         "mysql:8.0",
				"port":          "3307",
				"root-password": "secret",
				"database":      "mydb",
			},
			want: &MySQLConfig{
				Image:        "mysql:8.0",
				Port:         "3307",
				RootPassword: "secret",
				Database:     "mydb",
			},
			wantErr: false,
		},
		{
			name: "custom config with numeric port",
			input: map[string]interface{}{
				"port": float64(3307),
			},
			want: &MySQLConfig{
				Image:        defaultImage,
				Port:         "3307",
				RootPassword: defaultRootPassword,
				Database:     defaultDatabase,
			},
			wantErr: false,
		},
		{
			name: "with user and password",
			input: map[string]interface{}{
				"user":     "appuser",
				"password": "apppass",
			},
			want: &MySQLConfig{
				Image:        defaultImage,
				Port:         defaultPort,
				RootPassword: defaultRootPassword,
				Database:     defaultDatabase,
				User:         "appuser",
				Password:     "apppass",
			},
			wantErr: false,
		},
		{
			name: "with container name",
			input: map[string]interface{}{
				"container-name": "my-mysql",
			},
			want: &MySQLConfig{
				Image:         defaultImage,
				Port:          defaultPort,
				RootPassword:  defaultRootPassword,
				Database:      defaultDatabase,
				ContainerName: "my-mysql",
			},
			wantErr: false,
		},
		{
			name: "partial config",
			input: map[string]interface{}{
				"root-password": "custompass",
				"database":      "customdb",
			},
			want: &MySQLConfig{
				Image:        defaultImage,
				Port:         defaultPort,
				RootPassword: "custompass",
				Database:     "customdb",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := NewMySQLPlugin(nil, nil)
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
			assert.Equal(t, tt.want.RootPassword, got.RootPassword)
			assert.Equal(t, tt.want.Database, got.Database)
			assert.Equal(t, tt.want.User, got.User)
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
		name         string
		config       *MySQLConfig
		wantUser     string
		wantPassword string
		wantErr      bool
		errContains  string
	}{
		{
			name: "valid config root only",
			config: &MySQLConfig{
				Port:         "3306",
				RootPassword: "rootpass",
				Database:     "testdb",
			},
			wantUser:     "root",
			wantPassword: "rootpass",
			wantErr:      false,
		},
		{
			name: "valid config with user",
			config: &MySQLConfig{
				Port:         "3306",
				RootPassword: "rootpass",
				Database:     "testdb",
				User:         "appuser",
				Password:     "apppass",
			},
			wantUser:     "appuser",
			wantPassword: "apppass",
			wantErr:      false,
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
			plugin := NewMySQLPlugin(nil, nil)
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
			assert.Equal(t, "mysql", got.Protocol)
			assert.Equal(t, tt.wantUser, got.Metadata["user"])
			assert.Equal(t, tt.wantPassword, got.Metadata["password"])
			assert.Equal(t, tt.config.Database, got.Metadata["database"])
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
			input: "3306",
			want:  3306,
		},
		{
			name:  "custom port",
			input: "3307",
			want:  3307,
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
	plugin := NewMySQLPlugin(nil, nil)

	// Should return error when container not started
	ready, err := plugin.IsReady(nil)
	assert.False(t, ready)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestStop_NoContainer(t *testing.T) {
	plugin := NewMySQLPlugin(nil, zap.NewNop())

	// Should not error when no container to stop
	err := plugin.Stop(nil)
	assert.NoError(t, err)
}
