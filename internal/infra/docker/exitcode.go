package docker

import (
	"fmt"

	"github.com/docker/docker/api/types"

	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

// ExitCode extracts the exit code of a finished container from the result of
// InspectContainer. A failed inspect or a missing state is an error: assuming
// 0 would report a run nobody observed as passed.
func ExitCode(inspect *types.ContainerJSON, err error) (int, error) {
	if err != nil {
		return 0, gtErrors.Wrap(err, gtErrors.ErrContainerFailed, "failed to inspect container")
	}
	if inspect == nil || inspect.ContainerJSONBase == nil || inspect.State == nil {
		return 0, gtErrors.New(gtErrors.ErrContainerFailed, "container inspect returned no state")
	}
	return inspect.State.ExitCode, nil
}

func runningState(state *types.ContainerState) (bool, error) {
	if state == nil {
		return false, nil
	}
	if state.Running {
		return true, nil
	}
	if state.Status == "exited" || state.Status == "dead" {
		return false, gtErrors.New(gtErrors.ErrContainerExited,
			fmt.Sprintf("container exited with code %d", state.ExitCode))
	}
	return false, nil
}
