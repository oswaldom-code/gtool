package docker

import (
	"archive/tar"
	"errors"
	"io"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

func TestExitCode(t *testing.T) {
	withState := func(code int) *types.ContainerJSON {
		return &types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{
			State: &types.ContainerState{ExitCode: code},
		}}
	}

	t.Run("returns the state exit code", func(t *testing.T) {
		code, err := ExitCode(withState(3), nil)
		require.NoError(t, err)
		assert.Equal(t, 3, code)
	})

	t.Run("inspect error is an error, not a pass", func(t *testing.T) {
		_, err := ExitCode(withState(0), errors.New("daemon hiccup"))
		require.Error(t, err)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrContainerFailed))
	})

	t.Run("missing state is an error", func(t *testing.T) {
		for name, in := range map[string]*types.ContainerJSON{
			"nil inspect": nil,
			"nil base":    {},
			"nil state":   {ContainerJSONBase: &types.ContainerJSONBase{}},
		} {
			_, err := ExitCode(in, nil)
			assert.Error(t, err, name)
		}
	})
}

func TestRunningState(t *testing.T) {
	running, err := runningState(&types.ContainerState{Running: true})
	require.NoError(t, err)
	assert.True(t, running)

	for _, status := range []string{"created", "restarting", "paused"} {
		running, err := runningState(&types.ContainerState{Status: status})
		require.NoError(t, err, status)
		assert.False(t, running, status)
	}

	for _, status := range []string{"exited", "dead"} {
		_, err := runningState(&types.ContainerState{Status: status, ExitCode: 1})
		require.Error(t, err, status)
		assert.True(t, gtErrors.Is(err, gtErrors.ErrContainerExited), status)
		assert.Contains(t, err.Error(), "code 1")
	}

	running, err = runningState(nil)
	require.NoError(t, err)
	assert.False(t, running)
}

func TestSingleFileTar(t *testing.T) {
	buf, err := singleFileTar("01-init.sql", []byte("SELECT 1;"))
	require.NoError(t, err)

	tr := tar.NewReader(buf)
	hdr, err := tr.Next()
	require.NoError(t, err)
	assert.Equal(t, "01-init.sql", hdr.Name)
	body, err := io.ReadAll(tr)
	require.NoError(t, err)
	assert.Equal(t, "SELECT 1;", string(body))
}
