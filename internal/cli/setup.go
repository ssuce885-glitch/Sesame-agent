package cli

import (
	"io"

	"go-agent/internal/cli/setupflow"
	"go-agent/internal/config"
)

func ensureRuntimeConfigured(stdin io.Reader, stdout io.Writer, cfg config.Config) error {
	return ensureRuntimeConfiguredAction(stdin, stdout, cfg, "")
}

func ensureRuntimeConfiguredAction(stdin io.Reader, stdout io.Writer, cfg config.Config, action string) error {
	return setupflow.Run(stdin, stdout, cfg, action)
}
