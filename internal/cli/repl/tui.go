package repl

import (
	"context"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	tuiv2 "go-agent/internal/cli/repl/tui"
)

const (
	enableAlternateScrollSeq  = "\x1b[?1007h"
	disableAlternateScrollSeq = "\x1b[?1007l"
)

func canUseTUI(stdin io.Reader, stdout io.Writer) bool {
	in, ok := stdin.(*os.File)
	if !ok {
		return false
	}
	out, ok := stdout.(*os.File)
	if !ok {
		return false
	}
	return isTerminal(in) && isTerminal(out)
}

func isTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	fd := file.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func (r *REPL) runTUI(ctx context.Context, initialPrompt string) error {
	uiClient := newTUIClientAdapter(r.client)
	status := tuiv2.StatusResponse{}
	if loaded, err := uiClient.Status(ctx); err == nil {
		status = loaded
	}
	timeline := tuiv2.SessionTimelineResponse{}
	if strings.TrimSpace(r.sessionID) != "" {
		if loaded, err := uiClient.GetTimeline(ctx); err == nil {
			timeline = loaded
			r.lastSeq = loaded.LatestSeq
		}
	}

	model := tuiv2.NewModel(tuiv2.ModelOptions{
		Context:       ctx,
		Client:        uiClient,
		SessionID:     r.sessionID,
		WorkspaceRoot: r.workspaceRoot,
		Status:        status,
		Catalog:       r.catalog,
		CatalogLoader: r.catalogLoader,
		Timeline:      timeline,
		InitialPrompt: initialPrompt,
	})

	programOpts := []tea.ProgramOption{
		tea.WithContext(ctx),
		tea.WithInput(r.stdin),
		tea.WithOutput(r.stdout),
	}
	if shouldUseTUIAltScreen(os.LookupEnv) {
		writeTUICtrlSeq(r.stdout, enableAlternateScrollSeq)
		defer writeTUICtrlSeq(r.stdout, disableAlternateScrollSeq)
		programOpts = append([]tea.ProgramOption{tea.WithAltScreen()}, programOpts...)
	}

	program := tea.NewProgram(model, programOpts...)
	_, err := program.Run()
	if err == tea.ErrProgramKilled {
		return nil
	}
	return err
}

func shouldUseTUIAltScreen(lookupEnv func(string) (string, bool)) bool {
	if lookupEnv == nil {
		return true
	}
	if envValueSet(lookupEnv, "ZELLIJ") {
		return false
	}
	return true
}

func envValueSet(lookupEnv func(string) (string, bool), key string) bool {
	value, ok := lookupEnv(key)
	return ok && strings.TrimSpace(value) != ""
}

func writeTUICtrlSeq(w io.Writer, seq string) {
	if w == nil || seq == "" {
		return
	}
	_, _ = io.WriteString(w, seq)
}
