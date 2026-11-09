package docker

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeRunID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "already valid", in: "build-42", want: "build-42"},
		{name: "jenkins folder tag", in: "jenkins-team/app-#12", want: "jenkins-team-app-12"},
		{name: "spaces", in: "my build 7", want: "my-build-7"},
		{name: "leading and trailing separators", in: "/-run-/", want: "run"},
		{name: "empty", in: "", want: ""},
		{name: "only invalid", in: "///", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizeRunID(tt.in))
		})
	}
}

func TestResolveRunID(t *testing.T) {
	hexID := regexp.MustCompile(`^[0-9a-f]{8}$`)

	t.Run("env wins and is sanitized", func(t *testing.T) {
		t.Setenv(RunIDEnv, "ci/build 9")
		assert.Equal(t, "ci-build-9", ResolveRunID(true))
		assert.Equal(t, "ci-build-9", ResolveRunID(false))
	})

	t.Run("generated when env unset and generate is true", func(t *testing.T) {
		t.Setenv(RunIDEnv, "")
		first, second := ResolveRunID(true), ResolveRunID(true)
		assert.Regexp(t, hexID, first)
		assert.NotEqual(t, first, second)
	})

	t.Run("unscoped when env unset and generate is false", func(t *testing.T) {
		t.Setenv(RunIDEnv, "")
		assert.Empty(t, ResolveRunID(false))
	})
}

func TestWithRunLabel(t *testing.T) {
	labels := map[string]string{"managed-by": "gtool"}

	t.Run("no run ID returns labels untouched", func(t *testing.T) {
		assert.Equal(t, labels, withRunLabel(labels, ""))
	})

	t.Run("run ID adds label without mutating input", func(t *testing.T) {
		got := withRunLabel(labels, "abc")
		assert.Equal(t, map[string]string{"managed-by": "gtool", RunLabel: "abc"}, got)
		assert.NotContains(t, labels, RunLabel)
	})

	t.Run("nil labels with run ID", func(t *testing.T) {
		assert.Equal(t, map[string]string{RunLabel: "abc"}, withRunLabel(nil, "abc"))
	})
}

func TestWithRunSuffix(t *testing.T) {
	assert.Equal(t, "gtool-redis-1-abc", withRunSuffix("gtool-redis-1", "abc"))
	assert.Equal(t, "gtool-redis-1", withRunSuffix("gtool-redis-1", ""))
	assert.Equal(t, "", withRunSuffix("", "abc"))
}

func TestClientRunID(t *testing.T) {
	c := &Client{}
	assert.Empty(t, c.RunID())

	c.SetRunID("team/app #3")
	assert.Equal(t, "team-app-3", c.RunID())

	c.SetRunID("")
	assert.Empty(t, c.RunID())
}
