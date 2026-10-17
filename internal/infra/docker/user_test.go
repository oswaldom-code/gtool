package docker

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHostUser(t *testing.T) {
	assert.Equal(t, fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), HostUser())
}
