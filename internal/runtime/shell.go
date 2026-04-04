package runtime

import (
	"context"
	"os/exec"
)

func RunCommand(ctx context.Context, workdir, command string, maxOutputBytes int) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "cmd", "/c", command)
	if workdir != "" {
		cmd.Dir = workdir
	}

	output, err := cmd.CombinedOutput()
	if maxOutputBytes > 0 && len(output) > maxOutputBytes {
		output = output[:maxOutputBytes]
	}

	return output, err
}
