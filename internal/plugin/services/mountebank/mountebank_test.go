package mountebank

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewMountebankPlugin(t *testing.T) {
	p := NewMountebankPlugin(nil, zap.NewNop())

	assert.NotNil(t, p)
	assert.Equal(t, "mountebank", p.Name())
	assert.NotNil(t, p.httpClient)
}

func TestName(t *testing.T) {
	p := NewMountebankPlugin(nil, nil)
	assert.Equal(t, "mountebank", p.Name())
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]interface{}
		want  *MountebankConfig
	}{
		{
			name:  "default config",
			input: map[string]interface{}{},
			want: &MountebankConfig{
				Image: defaultImage,
				Port:  defaultPort,
			},
		},
		{
			name: "custom config with string port",
			input: map[string]interface{}{
				"image":          "bbyars/mountebank:2.8.0",
				"port":           "3000",
				"imposters-path": "/path/to/imposters",
			},
			want: &MountebankConfig{
				Image:         "bbyars/mountebank:2.8.0",
				Port:          "3000",
				ImpostersPath: "/path/to/imposters",
			},
		},
		{
			name: "numeric port",
			input: map[string]interface{}{
				"port": float64(3000),
			},
			want: &MountebankConfig{
				Image: defaultImage,
				Port:  "3000",
			},
		},
		{
			name: "custom container name",
			input: map[string]interface{}{
				"container-name": "my-mb",
			},
			want: &MountebankConfig{
				Image:         defaultImage,
				Port:          defaultPort,
				ContainerName: "my-mb",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewMountebankPlugin(nil, nil)
			got, err := p.parseConfig(tt.input)

			require.NoError(t, err)
			assert.Equal(t, tt.want.Image, got.Image)
			assert.Equal(t, tt.want.Port, got.Port)
			assert.Equal(t, tt.want.ImpostersPath, got.ImpostersPath)
			if tt.want.ContainerName != "" {
				assert.Equal(t, tt.want.ContainerName, got.ContainerName)
			} else {
				assert.Contains(t, got.ContainerName, containerNamePrefix)
			}
		})
	}
}

func TestGetConnectionInfo(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		p := NewMountebankPlugin(nil, nil)
		p.config = &MountebankConfig{Port: "2525"}

		got, err := p.GetConnectionInfo()

		require.NoError(t, err)
		assert.Equal(t, "localhost", got.Host)
		assert.Equal(t, 2525, got.Port)
		assert.Equal(t, "http", got.Protocol)
		assert.Equal(t, "http://localhost:2525/", got.Metadata["admin-url"])
	})

	t.Run("not launched", func(t *testing.T) {
		p := NewMountebankPlugin(nil, nil)

		_, err := p.GetConnectionInfo()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not launched")
	})
}

func TestAdminURL(t *testing.T) {
	p := NewMountebankPlugin(nil, nil)
	p.config = &MountebankConfig{Port: "3000"}

	assert.Equal(t, "http://localhost:3000/", p.adminURL())
}

func TestIsReady_NotStarted(t *testing.T) {
	p := NewMountebankPlugin(nil, nil)

	ready, err := p.IsReady(nil)

	assert.False(t, ready)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestStop_NoContainer(t *testing.T) {
	p := NewMountebankPlugin(nil, zap.NewNop())

	err := p.Stop(nil)
	assert.NoError(t, err)
}

func TestMustParsePort(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"2525", 2525},
		{"3000", 3000},
		{"0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, mustParsePort(tt.input))
		})
	}
}
