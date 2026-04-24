package automation

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	runtimex "go-agent/internal/types"
)

const defaultContractValidationTimeout = 5 * time.Second
const scriptStatusContractGuidance = `When trigger_on is script_status, stdout must be one JSON object with:
- status: one of healthy, recovered, needs_agent, needs_human
- summary: non-empty string
- facts: optional JSON object

Examples:
{"status":"healthy","summary":"no .txt files found","facts":{"count":0}}
{"status":"needs_agent","summary":"found .txt files to clean","facts":{"count":2}}`

// ValidateWatcherContract performs a dry-run execution of a watch script
// and validates that its stdout conforms to the script_status detector
// signal contract: valid JSON with a "status" field set to one of
// healthy, recovered, needs_agent, or needs_human.
func ValidateWatcherContract(ctx context.Context, command string, workingDir string) error {
	runCtx, cancel := context.WithTimeout(ctx, defaultContractValidationTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "sh", "-c", command)
	if strings.TrimSpace(workingDir) != "" {
		cmd.Dir = workingDir
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	if err != nil {
		return &runtimex.AutomationValidationError{
			Code:    "watcher_contract_failed",
			Message: fmt.Sprintf("watcher contract validation failed: script exited with error: %v\nstdout: %s\nstderr: %s", err, truncateContractOutput(stdoutStr), truncateContractOutput(stderrStr)),
		}
	}

	raw := bytes.TrimSpace(stdout.Bytes())
	if len(raw) == 0 {
		return &runtimex.AutomationValidationError{
			Code:    "watcher_contract_failed",
			Message: "watcher contract validation failed: script produced empty stdout.\n" + scriptStatusContractGuidance,
		}
	}

	signal, err := ParseAutomationDetectorSignalPayload(raw)
	if err != nil {
		return &runtimex.AutomationValidationError{
			Code:    "watcher_contract_failed",
			Message: fmt.Sprintf("watcher contract validation failed: %v\noutput: %s\n%s", err, truncateContractOutput(stdoutStr), scriptStatusContractGuidance),
		}
	}

	switch signal.Status {
	case runtimex.AutomationDetectorStatusHealthy, runtimex.AutomationDetectorStatusRecovered, runtimex.AutomationDetectorStatusNeedsAgent, runtimex.AutomationDetectorStatusNeedsHuman:
		return nil
	default:
		return &runtimex.AutomationValidationError{
			Code:    "watcher_contract_failed",
			Message: fmt.Sprintf("watcher contract validation failed: script output JSON \"status\" field must be one of: healthy, recovered, needs_agent, needs_human. Got: %q\noutput: %s\n%s", signal.Status, truncateContractOutput(stdoutStr), scriptStatusContractGuidance),
		}
	}
}

func truncateContractOutput(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 512 {
		return s
	}
	return s[:512] + "..."
}
