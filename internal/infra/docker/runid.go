package docker

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"regexp"
	"strings"
)

const (
	// RunIDEnv lets CI pin the run ID (e.g. to Jenkins' BUILD_TAG) so separate
	// gtool invocations of one build share it and other builds stay isolated.
	RunIDEnv = "GTOOL_RUN_ID"
	// RunLabel is the container label that scopes resources to one run.
	RunLabel = "gtool-run"
)

var invalidRunIDChars = regexp.MustCompile(`[^a-zA-Z0-9_.]+`)

// sanitizeRunID makes id usable inside Docker container names.
func sanitizeRunID(id string) string {
	return strings.Trim(invalidRunIDChars.ReplaceAllString(id, "-"), "-._")
}

// newRunID returns a short random identifier.
func newRunID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ResolveRunID returns the run ID from GTOOL_RUN_ID. When unset it returns a
// random ID if generate is true (single-invocation pipelines) or "" otherwise,
// which keeps the legacy, unscoped behaviour.
func ResolveRunID(generate bool) string {
	if id := sanitizeRunID(os.Getenv(RunIDEnv)); id != "" {
		return id
	}
	if generate {
		return newRunID()
	}
	return ""
}

// withRunLabel returns a copy of labels scoped to runID (unchanged when empty).
func withRunLabel(labels map[string]string, runID string) map[string]string {
	if runID == "" {
		return labels
	}
	scoped := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		scoped[k] = v
	}
	scoped[RunLabel] = runID
	return scoped
}

// withRunSuffix appends runID to a container name so concurrent runs never
// collide on names. Anonymous containers (empty name) stay anonymous.
func withRunSuffix(name, runID string) string {
	if name == "" || runID == "" {
		return name
	}
	return name + "-" + runID
}
