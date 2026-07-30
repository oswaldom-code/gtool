package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleGoMod = `module notification

go 1.24.1

require (
	git.pro.sisdig.dgrp.io/es-dia-ecom/common-library-back v1.45.0
	github.com/onsi/ginkgo/v2 v2.26.0
)
`

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

func TestResolveSource(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(sampleGoMod), 0o644))
	t.Chdir(dir)

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
			want:   filepath.Join(modCache, "git.pro.sisdig.dgrp.io/es-dia-ecom/common-library-back@v1.45.0", "messaging/publisher/publisher_interface.go"),
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
			name:        "unknown module",
			source:      "<SEARCH-LIB>/foo.go",
			wantErr:     true,
			errContains: "search-library",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSource(tt.source, modCache)
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

func TestModuleCachePathNotFound(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(sampleGoMod), 0o644))
	t.Chdir(dir)

	_, err := moduleCachePath("nonexistent-library", "/cache")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent-library")
}
