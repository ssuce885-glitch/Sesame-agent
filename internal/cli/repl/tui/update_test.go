package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleKeyKeepsArrowKeysInMultilineInput(t *testing.T) {
	model := NewModel(ModelOptions{})
	for i := 0; i < 100; i++ {
		model.appendNotice("line")
	}
	model.layout()
	model.textarea.SetValue("first\nsecond")
	model.viewport.GotoBottom()
	before := model.viewport.YOffset

	_, _ = model.handleKey(tea.KeyMsg{Type: tea.KeyUp})

	if model.viewport.YOffset != before {
		t.Fatalf("up key with multiline input changed viewport offset from %d to %d", before, model.viewport.YOffset)
	}
}

func TestCommandHelpUsesAcceptedHistorySyntax(t *testing.T) {
	help := commandHelpText()
	if strings.Contains(help, "/history [/load <head_id>]") {
		t.Fatalf("help includes unsupported /history /load syntax: %q", help)
	}
	if !strings.Contains(help, "/history [list|load <head_id>]") {
		t.Fatalf("help does not document accepted history syntax: %q", help)
	}
}
