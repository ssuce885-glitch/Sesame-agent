//go:build !unix

package runtime

import "os/exec"

func configureCommandCancellation(cmd *exec.Cmd) {}
