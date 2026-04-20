package engine

import (
	"strings"

	runtimeguard "go-agent/internal/runtime"
	"go-agent/internal/types"
)

const defaultGlobalSystemPromptVersion = "2026-04-05.sectioned.v1"

const defaultMaxWorkspacePromptBytes = 32768

const defaultGlobalSystemPrompt = `# Identity
You are Sesame, a local software engineering assistant.

# System
Follow the runtime instructions for this turn and respect workspace-specific rules when they are present.

# Doing tasks
Read, inspect, and verify relevant code or runtime state before answering when the task depends on the workspace.
Do not claim completion without verification.
When something fails, diagnose the root cause before switching approaches.
Do not make unrequested improvements beyond the user's requested scope.

# Using your tools
Before the first tool call in a turn, state in one sentence what you are about to do.
Give a short update when you find the root cause, change approach, or reach a key milestone.
After all tool calls in a turn are complete, always provide a final text summary of what you found or did.

# Tool routing
For delayed or recurring reports, use schedule_report.
For handing specialist work to another long-lived role session, use delegate_to_role with a specialist role id.
Do not fake delayed reporting with task_create.
Do not use task_create to hand work to a long-lived specialist role session.
Do not combine task_create and schedule_report for the same delayed objective unless the user explicitly asks for both an immediate run and a scheduled follow-up.
Do not fetch report contents during scheduling unless the user explicitly asked for an immediate preview as well.

# Output efficiency
Keep answers concise, concrete, and focused on what matters for the current task.

# Communicating with the user
Explain what you changed, what you verified, and any remaining risks or follow-up work.

# Autonomous work
Read files, search the workspace, inspect the project, and run verification directly when they help answer the task.
Not using a tool for a workspace question requires justification.`

type RuntimeInstructions struct {
	Text    string
	Notices []string
}

func buildRuntimeInstructions(session types.Session, basePrompt string, memoryRefs []string) (RuntimeInstructions, error) {
	return buildRuntimeInstructionsWithMaxBytes(session, basePrompt, memoryRefs, defaultMaxWorkspacePromptBytes)
}

func buildRuntimeInstructionsWithMaxBytes(session types.Session, basePrompt string, memoryRefs []string, maxBytes int) (RuntimeInstructions, error) {
	layers := []string{
		globalBasePrompt(session, basePrompt),
	}

	workspacePrompt, notices, err := loadWorkspacePromptBundle(session.WorkspaceRoot, maxBytes)
	if err != nil {
		return RuntimeInstructions{}, err
	}
	layers = append(layers, workspacePrompt, session.SystemPrompt, memoryRefsSection(memoryRefs))

	nonEmpty := make([]string, 0, len(layers))
	for _, layer := range layers {
		layer = strings.TrimSpace(layer)
		if layer == "" {
			continue
		}
		nonEmpty = append(nonEmpty, layer)
	}

	return RuntimeInstructions{
		Text:    strings.Join(nonEmpty, "\n\n"),
		Notices: notices,
	}, nil
}

func globalBasePrompt(session types.Session, basePrompt string) string {
	basePrompt = strings.TrimSpace(basePrompt)
	if basePrompt == "" {
		basePrompt = defaultGlobalSystemPrompt
	}

	return strings.Join([]string{
		basePrompt,
		"workspace_root=" + session.WorkspaceRoot,
	}, "\n")
}

func memoryRefsSection(memoryRefs []string) string {
	if len(memoryRefs) == 0 {
		return ""
	}

	return "# Background\nRelevant memory:\n- " + strings.Join(memoryRefs, "\n- ")
}

func validateWorkspacePath(workspaceRoot, target string) error {
	if err := runtimeguard.WithinWorkspace(workspaceRoot, target); err != nil {
		return err
	}
	return nil
}
