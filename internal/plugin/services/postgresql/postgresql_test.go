package postgresql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewPostgreSQLPlugin(t *testing.T) {
	logger := zap.NewNop()
	plugin := NewPostgreSQLPlugin(nil, logger)

	assert.NotNil(t, plugin)
	assert.Equal(t, "postgresql", plugin.Name())
}

func TestName(t *testing.T) {
	plugin := NewPostgreSQLPlugin(nil, nil)
	assert.Equal(t, "postgresql", plugin.Name())
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]interface{}
		want        *PostgreSQLConfig
		wantErr     bool
		errContains string
	}{
		{
			name:  "default config",
			input: map[string]interface{}{},
			want: &PostgreSQLConfig{
				Image:    defaultImage,
				Port:     defaultPort,
				User:     defaultUser,
				Password: defaultPassword,
				Database: defaultDatabase,
			},
			wantErr: false,
		},
		{
			name: "custom config with string port",
			input: map[string]interface{}{
				"image":    "postgres:15",
				"port":     "5433",
				"user":     "myuser",
				"password": "mypass",
				"database": "mydb",
			},
			want: &PostgreSQLConfig{
				Image:    "postgres:15",
				Port:     "5433",
				User:     "myuser",
				Password: "mypass",
				Database: "mydb",
			},
			wantErr: false,
		},
		{
			name: "custom config with numeric port",
			input: map[string]interface{}{
				"port": float64(5433),
			},
			want: &PostgreSQLConfig{
				Image:    defaultImage,
				Port:     "5433",
				User:     defaultUser,
				Password: defaultPassword,
				Database: defaultDatabase,
			},
			wantErr: false,
		},
		{
			name: "with scripts path",
			input: map[string]interface{}{
				"scripts-path": "/path/to/scripts",
			},
			want: &PostgreSQLConfig{
				Image:       defaultImage,
				Port:        defaultPort,
				User:        defaultUser,
				Password:    defaultPassword,
				Database:    defaultDatabase,
				ScriptsPath: "/path/to/scripts",
			},
			wantErr: false,
		},
		{
			name: "with container name",
			input: map[string]interface{}{
				"container-name": "my-postgres",
			},
			want: &PostgreSQLConfig{
				Image:         defaultImage,
				Port:          defaultPort,
				User:          defaultUser,
				Password:      defaultPassword,
				Database:      defaultDatabase,
				ContainerName: "my-postgres",
			},
			wantErr: false,
		},
		{
			name: "partial config",
			input: map[string]interface{}{
				"user":     "customuser",
				"database": "customdb",
			},
			want: &PostgreSQLConfig{
				Image:    defaultImage,
				Port:     defaultPort,
				User:     "customuser",
				Password: defaultPassword,
				Database: "customdb",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := NewPostgreSQLPlugin(nil, nil)
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
			assert.Equal(t, tt.want.User, got.User)
			assert.Equal(t, tt.want.Password, got.Password)
			assert.Equal(t, tt.want.Database, got.Database)
			assert.Equal(t, tt.want.ScriptsPath, got.ScriptsPath)
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
		config      *PostgreSQLConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "valid config",
			config: &PostgreSQLConfig{
				Port:     "5432",
				User:     "testuser",
				Password: "testpass",
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
			plugin := NewPostgreSQLPlugin(nil, nil)
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
			assert.Equal(t, "postgresql", got.Protocol)
			assert.Equal(t, tt.config.User, got.Metadata["user"])
			assert.Equal(t, tt.config.Password, got.Metadata["password"])
			assert.Equal(t, tt.config.Database, got.Metadata["database"])
			assert.Equal(t, "disable", got.Metadata["sslmode"])
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
			input: "5432",
			want:  5432,
		},
		{
			name:  "custom port",
			input: "5433",
			want:  5433,
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
	plugin := NewPostgreSQLPlugin(nil, nil)

	// Should return error when container not started
	ready, err := plugin.IsReady(nil)
	assert.False(t, ready)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestStop_NoContainer(t *testing.T) {
	plugin := NewPostgreSQLPlugin(nil, zap.NewNop())

	// Should not error when no container to stop
	err := plugin.Stop(nil)
	assert.NoError(t, err)
}
