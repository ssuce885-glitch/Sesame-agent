package agent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go-agent/internal/v2/contracts"
)

const (
	workspaceInstructionsFile     = "AGENTS.md"
	maxWorkspaceInstructionsBytes = 64 * 1024
)

// BuildContext returns messages to send to the model.
// It includes a system message, prior messages truncated from the oldest side,
// and the current turn messages.
func BuildContext(systemPrompt string, prior []contracts.Message, turn []contracts.Message, maxTokens int) []contracts.Message {
	if maxTokens <= 0 {
		maxTokens = effectiveContextTokens(maxTokens)
	}
	prior = messagesAfterCompactBoundary(prior)

	used := approximateContextTokens(systemPrompt, nil, turn)

	selectedPrior := make([]contracts.Message, 0, len(prior))
	for i := len(prior) - 1; i >= 0; i-- {
		msg := prior[i]
		next := used + approximateMessageTokens([]contracts.Message{msg})
		if next > maxTokens && len(selectedPrior) > 0 {
			break
		}
		if next > maxTokens && len(selectedPrior) == 0 {
			continue
		}
		used = next
		selectedPrior = append(selectedPrior, msg)
	}
	reverseMessages(selectedPrior)

	out := make([]contracts.Message, 0, 1+len(selectedPrior)+len(turn))
	if systemPrompt != "" {
		out = append(out, contracts.Message{Role: "system", Content: systemPrompt})
	}
	out = append(out, selectedPrior...)
	out = append(out, turn...)
	return out
}

func buildContext(systemPrompt string, prior []contracts.Message, turn []contracts.Message, maxTokens int) []contracts.Message {
	return BuildContext(systemPrompt, prior, turn, maxTokens)
}

// BuildInstructions combines the base system prompt with workspace project state.
func BuildInstructions(systemPrompt, projectState string) string {
	return BuildInstructionsWithWorkspace(systemPrompt, "", projectState)
}

// BuildInstructionsWithWorkspace combines the base system prompt with
// user-maintained workspace instructions and workspace project state.
func BuildInstructionsWithWorkspace(systemPrompt, workspaceInstructions, projectState string) string {
	systemPrompt = strings.TrimSpace(systemPrompt)
	workspaceInstructions = strings.TrimSpace(workspaceInstructions)
	projectState = strings.TrimSpace(projectState)
	if workspaceInstructions == "" && projectState == "" {
		return systemPrompt
	}
	var b strings.Builder
	if systemPrompt != "" {
		b.WriteString(systemPrompt)
		b.WriteString("\n\n")
	}
	if workspaceInstructions != "" {
		b.WriteString("Workspace Instructions (AGENTS.md):\n")
		b.WriteString(workspaceInstructions)
		b.WriteString("\n\nUse Workspace Instructions as user-maintained baseline rules for this workspace. Do not treat them as a new user request by themselves.")
		if projectState != "" {
			b.WriteString("\n\n")
		}
	}
	if projectState != "" {
		b.WriteString("Project State:\n")
		b.WriteString(projectState)
		b.WriteString("\n\nUse Project State as the compact source of truth for long-running workspace goals, decisions, open threads, artifacts, and validation status. Do not treat it as a user request by itself.")
	}
	return b.String()
}

func buildInstructions(systemPrompt, projectState string) string {
	return BuildInstructions(systemPrompt, projectState)
}

func buildInstructionsWithWorkspace(systemPrompt, workspaceInstructions, projectState string) string {
	return BuildInstructionsWithWorkspace(systemPrompt, workspaceInstructions, projectState)
}

func LoadWorkspaceInstructions(workspaceRoot string) (string, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return "", nil
	}
	path := filepath.Join(workspaceRoot, workspaceInstructionsFile)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("open %s: %w", workspaceInstructionsFile, err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxWorkspaceInstructionsBytes+1))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", workspaceInstructionsFile, err)
	}
	content := strings.TrimSpace(strings.ToValidUTF8(string(data), ""))
	if len(data) <= maxWorkspaceInstructionsBytes {
		return content, nil
	}
	truncated := strings.ToValidUTF8(string(data[:maxWorkspaceInstructionsBytes]), "")
	return strings.TrimSpace(truncated) + "\n\n[Workspace instructions truncated by runtime.]", nil
}

func reverseMessages(messages []contracts.Message) {
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
}
