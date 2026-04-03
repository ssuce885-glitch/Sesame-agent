package runtime

import (
	"context"
	"os/exec"
)

func RunCommand(ctx context.Context, command string) ([]byte, error) {
	return exec.CommandContext(ctx, "cmd", "/c", command).CombinedOutput()
}
