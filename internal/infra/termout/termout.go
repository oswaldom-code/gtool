// Package termout prints user-facing CLI output. Symbols and emojis are kept
// on an interactive terminal and replaced by plain ASCII markers (or dropped
// when purely decorative) when stdout is a pipe or NO_COLOR / CI is set, so
// CI consoles and log files stay readable.
package termout

import (
	"fmt"
	"io"
	"os"
	"strings"
)

var plain = strings.NewReplacer(
	"✅ ", "OK ", "✓ ", "OK ", "✗ ", "FAIL ", "⚠️  ", "WARN ", "⚠️ ", "WARN ",
	"● ", "* ", "▶ ", "> ", "ℹ️  ", "", "ℹ️ ", "",
	"🚀 ", "", "🛑 ", "", "✨ ", "", "🧹 ", "", "🔄 ", "", "📊 ", "", "🧪 ", "", "⏳ ", "", "🥋 ", "",
	"✅", "OK", "✓", "OK", "✗", "FAIL", "●", "*", "▶", ">",
)

// Plain reports whether output must avoid symbols.
var Plain = func() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("CI") != "" {
		return true
	}
	info, err := os.Stdout.Stat()
	return err != nil || info.Mode()&os.ModeCharDevice == 0
}

// Text adapts s to the current output.
func Text(s string) string {
	if Plain() {
		return plain.Replace(s)
	}
	return s
}

// Fprintf writes a formatted line to w, adapting the format string.
func Fprintf(w io.Writer, format string, args ...interface{}) {
	_, _ = fmt.Fprintf(w, Text(format), args...)
}

// Printf writes a formatted line to stdout.
func Printf(format string, args ...interface{}) {
	Fprintf(os.Stdout, format, args...)
}

// Println writes s and a newline to stdout.
func Println(s string) {
	_, _ = fmt.Fprintln(os.Stdout, Text(s))
}
