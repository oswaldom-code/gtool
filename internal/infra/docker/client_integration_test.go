//go:build integration

package docker

import (
	"context"
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunContainer_RemovesContainerWhenStartFails(t *testing.T) {
	ctx := context.Background()
	c, err := NewClient(nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	require.NoError(t, c.Ping(ctx))
	c.SetRunID(newRunID())

	busy, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = busy.Close() })
	hostPort := strconv.Itoa(busy.Addr().(*net.TCPAddr).Port)

	labels := map[string]string{"managed-by": "gtool", "service": "runcontainer-it"}
	_, err = c.RunContainer(ctx, &ContainerConfig{
		Image:        "alpine:3",
		Name:         "gtool-runcontainer-it",
		Cmd:          []string{"sleep", "30"},
		PortBindings: map[string]string{"80": hostPort},
		Labels:       labels,
	})
	require.Error(t, err, "binding an allocated host port must fail the start")

	left, err := c.ListContainersByLabels(ctx, labels)
	require.NoError(t, err)
	assert.Empty(t, left, "the created container must be removed after a failed start")
}

func TestRunContainer_StartsContainer(t *testing.T) {
	ctx := context.Background()
	c, err := NewClient(nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	c.SetRunID(newRunID())

	id, err := c.RunContainer(ctx, &ContainerConfig{
		Image:  "alpine:3",
		Name:   "gtool-runcontainer-ok",
		Cmd:    []string{"sleep", "30"},
		Labels: map[string]string{"managed-by": "gtool", "service": "runcontainer-it"},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.RemoveContainer(context.Background(), id, true) })

	running, err := c.IsContainerRunning(ctx, id)
	require.NoError(t, err)
	assert.True(t, running)
}

func TestExecInContainer_ReturnsPlainOutput(t *testing.T) {
	ctx := context.Background()
	c, err := NewClient(nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	c.SetRunID(newRunID())

	id, err := c.RunContainer(ctx, &ContainerConfig{
		Image:  "alpine:3",
		Name:   "gtool-exec-it",
		Cmd:    []string{"sleep", "30"},
		Labels: map[string]string{"managed-by": "gtool", "service": "exec-it"},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.RemoveContainer(context.Background(), id, true) })

	out, err := c.ExecInContainer(ctx, id, &ExecConfig{
		Cmd:          []string{"sh", "-c", "echo PONG; echo oops >&2"},
		AttachStdout: true,
		AttachStderr: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "PONG\noops\n", out, "no stream frame headers in the output")
}
