package termout

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func withPlain(t *testing.T, plainOutput bool) {
	t.Helper()
	orig := Plain
	Plain = func() bool { return plainOutput }
	t.Cleanup(func() { Plain = orig })
}

func TestText(t *testing.T) {
	withPlain(t, true)
	assert.Equal(t, "OK Pipeline completed", Text("✅ Pipeline completed"))
	assert.Equal(t, "Starting services...", Text("🚀 Starting services..."))
	assert.Equal(t, "WARN no config", Text("⚠️  no config"))
	assert.Equal(t, "* running", Text("● running"))
	assert.Equal(t, "plain text", Text("plain text"))

	withPlain(t, false)
	assert.Equal(t, "✅ Pipeline completed", Text("✅ Pipeline completed"), "terminals keep the symbols")
}

func TestFprintf(t *testing.T) {
	withPlain(t, true)
	var b bytes.Buffer
	Fprintf(&b, "🛑 %s stopped\n", "redis")
	assert.Equal(t, "redis stopped\n", b.String())
}

func TestPlainHonoursEnv(t *testing.T) {
	t.Setenv("CI", "true")
	assert.True(t, Plain())
	t.Setenv("CI", "")
	t.Setenv("NO_COLOR", "1")
	assert.True(t, Plain())
}
