package cli

import (
	"bufio"
	"fmt"
	"io"
	"runtime"
	"strings"

	"go-agent/internal/cli/setupflow"
	"go-agent/internal/config"
)

func ensureRuntimeConfigured(stdin io.Reader, stdout io.Writer, cfg config.Config) error {
	action := ""
	if calledFromRunSetup() {
		action = "setup"
	}
	return setupflow.Run(stdin, stdout, cfg, action)
}

func promptRequired(reader *bufio.Reader, stdout io.Writer, label, defaultValue string) (string, error) {
	return promptValue(reader, stdout, label, defaultValue, defaultValue)
}

func promptSecretRequired(reader *bufio.Reader, stdout io.Writer, label, defaultValue string) (string, error) {
	display := ""
	if strings.TrimSpace(defaultValue) != "" {
		display = "your-key"
	}
	return promptValue(reader, stdout, label, display, defaultValue)
}

func promptValue(reader *bufio.Reader, stdout io.Writer, label, displayValue, actualDefault string) (string, error) {
	for {
		if strings.TrimSpace(displayValue) != "" {
			fmt.Fprintf(stdout, "%s [%s]: ", label, displayValue)
		} else {
			fmt.Fprintf(stdout, "%s: ", label)
		}
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		value := strings.TrimSpace(line)
		if value == "" && err == io.EOF && len(line) == 0 {
			return "", io.EOF
		}
		if value == "" {
			value = strings.TrimSpace(actualDefault)
		}
		if value != "" {
			return value, nil
		}
		if err == io.EOF {
			return "", io.EOF
		}
	}
}

func firstNonEmptyLocal(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func calledFromRunSetup() bool {
	pcs := make([]uintptr, 8)
	n := runtime.Callers(2, pcs)
	if n == 0 {
		return false
	}
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if strings.HasSuffix(frame.Function, ".runSetup") {
			return true
		}
		if !more {
			break
		}
	}
	return false
}
