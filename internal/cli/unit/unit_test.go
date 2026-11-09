package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadBuildConfig(t *testing.T) {
	dir := t.TempDir()

	valid := filepath.Join(dir, "build-config.yml")
	require.NoError(t, os.WriteFile(valid, []byte(`mocks:
  - source: internal/foo/foo_interface.go
    filename: foo_interface.go
`), 0o644))

	invalid := filepath.Join(dir, "invalid.yml")
	require.NoError(t, os.WriteFile(invalid, []byte("mocks: [oops"), 0o644))

	tests := []struct {
		name        string
		path        string
		wantMocks   int
		wantErr     bool
		errContains string
	}{
		{name: "valid", path: valid, wantMocks: 1},
		{name: "not found", path: filepath.Join(dir, "missing.yml"), wantErr: true, errContains: "not found"},
		{name: "invalid yaml", path: invalid, wantErr: true, errContains: "parse"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := loadBuildConfig(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)
			assert.Len(t, cfg.Mocks, tt.wantMocks)
		})
	}
}

var sampleModules = map[string]string{
	"example.com/acme/shared-lib": "/cache/example.com/acme/shared-lib@v1.45.0",
	"example.com/acme/search-lib-v2":   "/cache/search-lib-v2@v2.0.0",
	"github.com/!some!org/business-lib":                  "",
}

func TestResolveSource(t *testing.T) {
	modCache := "/home/user/go/pkg/mod"

	tests := []struct {
		name        string
		source      string
		want        string
		wantErr     bool
		errContains string
	}{
		{
			name:   "common-lib wildcard",
			source: "<COMMON-LIB>/messaging/publisher/publisher_interface.go",
			want:   "/cache/example.com/acme/shared-lib@v1.45.0/messaging/publisher/publisher_interface.go",
		},
		{
			name:   "lib-root wildcard",
			source: "<LIB-ROOT>/some/path.go",
			want:   filepath.Join(modCache, "some/path.go"),
		},
		{
			name:   "plain path untouched",
			source: "internal/service/foo_interface.go",
			want:   "internal/service/foo_interface.go",
		},
		{
			name:        "a longer module name with the same prefix does not match",
			source:      "<SEARCH-LIB>/foo.go",
			wantErr:     true,
			errContains: "search-lib",
		},
		{
			name:        "module in the build list but not downloaded",
			source:      "<BUSINESS-LIB>/foo.go",
			wantErr:     true,
			errContains: "not downloaded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSource(tt.source, modCache, sampleModules)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUsesWildcards(t *testing.T) {
	assert.False(t, usesWildcards([]mockSpec{{Source: "internal/a.go"}}))
	assert.True(t, usesWildcards([]mockSpec{{Source: "internal/a.go"}, {Source: "<COMMON-LIB>/b.go"}}))
}

func TestListModules_ParsesTheGoCommand(t *testing.T) {
	modules, err := listModules()
	require.NoError(t, err)
	assert.Contains(t, modules, "github.com/oswaldom-code/gtool", "runs inside this module")
}
