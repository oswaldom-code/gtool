package mountebank

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
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

	ready, err := p.IsReady(context.TODO())

	assert.False(t, ready)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestStop_NoContainer(t *testing.T) {
	p := NewMountebankPlugin(nil, zap.NewNop())

	err := p.Stop(context.TODO())
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

func TestImposterPorts(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"api.json":     `{"port": 4545, "protocol": "http"}`,
		"other.json":   `{"port": 4546}`,
		"no-port.json": `{"protocol": "http"}`,
		"invalid.json": `{not json`,
		"ignored.ejs":  `{"port": 9999}`,
	}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}

	assert.ElementsMatch(t, []string{"4545", "4546"}, imposterPorts(dir))
	assert.Nil(t, imposterPorts(""))
	assert.Empty(t, imposterPorts(filepath.Join(dir, "missing")))
}

func TestPortBindings(t *testing.T) {
	t.Run("admin and imposters", func(t *testing.T) {
		got, err := portBindings("41000", []string{"4545", "4546"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"2525": "41000", "4545": "4545", "4546": "4546"}, got)
	})

	for name, tc := range map[string]struct{ admin, imposter string }{
		"imposter on the admin container port": {"41000", "2525"},
		"imposter on the admin host port":      {"4545", "4545"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := portBindings(tc.admin, []string{tc.imposter})
			require.Error(t, err)
			assert.True(t, gtErrors.Is(err, gtErrors.ErrConfigInvalid))
		})
	}
}

func TestImposterDefinitions(t *testing.T) {
	single, err := imposterDefinitions([]byte(`{"port": 4545, "protocol": "http"}`))
	require.NoError(t, err)
	assert.Len(t, single, 1)

	multi, err := imposterDefinitions([]byte(`{"imposters": [{"port": 4545}, {"port": 4546}]}`))
	require.NoError(t, err)
	assert.Len(t, multi, 2)

	_, err = imposterDefinitions([]byte(`not json`))
	assert.Error(t, err)
}

func TestImposterPorts_ConfigFileFormat(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "all.json"),
		[]byte(`{"imposters": [{"port": 4545}, {"port": 4546}]}`), 0o644))

	assert.ElementsMatch(t, []string{"4545", "4546"}, imposterPorts(dir))
}

func TestPostImposter_PostsEachImposterOfAConfigFile(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "all.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"imposters": [{"port": 4545}, {"port": 4546}]}`), 0o644))

	p := NewMountebankPlugin(nil, zap.NewNop())
	p.config = &MountebankConfig{Port: srv.URL[strings.LastIndex(srv.URL, ":")+1:]}

	require.NoError(t, p.postImposter(context.Background(), file))
	assert.Equal(t, []string{`{"port": 4545}`, `{"port": 4546}`}, bodies)
}

func TestPostImposter_RejectedImposterFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	file := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"port": 4545}`), 0o644))
	p := NewMountebankPlugin(nil, zap.NewNop())
	p.config = &MountebankConfig{Port: srv.URL[strings.LastIndex(srv.URL, ":")+1:]}

	assert.Error(t, p.postImposter(context.Background(), file))
}
