package cli

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVcsCommit(t *testing.T) {
	tests := []struct {
		name     string
		settings []debug.BuildSetting
		want     string
	}{
		{name: "no settings", want: ""},
		{
			name:     "revision is shortened",
			settings: []debug.BuildSetting{{Key: "vcs", Value: "git"}, {Key: "vcs.revision", Value: "eafaadf0123456789abcdef"}},
			want:     "eafaadf",
		},
		{
			name:     "malformed revision is ignored",
			settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}},
			want:     "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, vcsCommit(tt.settings))
		})
	}
}

func TestBuildCommit_PrefersInjectedValue(t *testing.T) {
	original := GitCommit
	t.Cleanup(func() { GitCommit = original })

	GitCommit = "c0ffee1"
	assert.Equal(t, "c0ffee1", buildCommit())
}

func TestOrUnknown(t *testing.T) {
	assert.Equal(t, "unknown", orUnknown(""))
	assert.Equal(t, "c0ffee1", orUnknown("c0ffee1"))
}
