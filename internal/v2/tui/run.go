package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func Run(ctx context.Context, client RuntimeClient, workspaceRoot string) error {
	session, err := client.EnsureSession(ctx, workspaceRoot)
	if err != nil {
		return fmt.Errorf("ensure session: %w", err)
	}
	m := NewModel(ctx, client, session.ID, firstNonEmpty(session.WorkspaceRoot, workspaceRoot))
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
