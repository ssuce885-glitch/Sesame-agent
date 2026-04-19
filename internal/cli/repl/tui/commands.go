package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"go-agent/internal/extensions"
)

func (m *Model) handleCommand(line string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "/"))
	if len(fields) == 0 {
		return m, nil
	}

	switch fields[0] {
	case "help":
		m.appendActivity("commands", commandHelpText())
		m.layout()
		return m, nil
	case "chat":
		return m.switchView(ViewChat, nil)
	case "agents", "subagents":
		return m.switchView(ViewSubagents, m.loadAgentsCmd())
	case "exit":
		return m, tea.Quit
	case "clear":
		m.entries = nil
		m.toolIndexByCall = make(map[string]int)
		m.toolIndexByKey = make(map[string]int)
		m.layout()
		return m, nil
	case "status":
		return m, m.refreshStatusCmd(true)
	case "skills":
		if err := m.refreshCatalog(); err != nil {
			m.appendError(err.Error())
			m.layout()
			return m, nil
		}
		lines := formatSkillList(m.catalog)
		m.appendActivity("skills", strings.Join(lines, "\n"))
		m.layout()
		return m, nil
	case "tools":
		if err := m.refreshCatalog(); err != nil {
			m.appendError(err.Error())
			m.layout()
			return m, nil
		}
		lines := formatToolList(m.catalog)
		m.appendActivity("tools", strings.Join(lines, "\n"))
		m.layout()
		return m, nil
	case "history":
		return m.handleHistoryCommand(fields[1:])
	case "reopen":
		return m, m.reopenContextCmd()
	case "approve", "allow", "deny":
		cmd, err := m.permissionDecisionCmd(fields[0], fields[1:])
		if err != nil {
			m.appendError(err.Error())
			m.layout()
			return m, nil
		}
		return m, cmd
	case "mailbox", "inbox":
		return m.switchView(ViewMailbox, m.loadMailboxCmd())
	case "cron":
		return m.handleCronCommand(fields[1:])
	default:
		m.appendError("unknown command: /" + fields[0])
		m.layout()
		return m, nil
	}
}

func commandHelpText() string {
	return strings.Join([]string{
		"/help /clear /exit /status /skills /tools /history [/load <head_id>]",
		"/reopen /approve [<request_id>] [once|run|session] /deny [<request_id>]",
		"/mailbox /cron list [--all] /cron inspect <id> /cron pause <id>",
		"/cron resume <id> /cron remove <id>",
	}, "\n")
}

func (m *Model) handleHistoryCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 || strings.EqualFold(strings.TrimSpace(args[0]), "list") {
		return m, m.listContextHistoryCmd()
	}
	if strings.EqualFold(strings.TrimSpace(args[0]), "load") {
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			m.appendError("usage: /history [list] | load <head_id>")
			m.layout()
			return m, nil
		}
		return m, m.loadContextHistoryCmd(strings.TrimSpace(args[1]))
	}
	m.appendError("usage: /history [list] | load <head_id>")
	m.layout()
	return m, nil
}

func (m *Model) handleCronCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.appendError("usage: /cron list [--all] | inspect <id> | pause <id> | resume <id> | remove <id>")
		m.layout()
		return m, nil
	}
	switch args[0] {
	case "list":
		allWorkspaces := len(args) > 1 && strings.TrimSpace(args[1]) == "--all"
		return m.switchView(ViewCron, m.listCronJobsCmd(allWorkspaces))
	case "inspect":
		if len(args) < 2 {
			m.appendError("usage: /cron inspect <id>")
			m.layout()
			return m, nil
		}
		return m.switchView(ViewCron, m.inspectCronJobCmd(args[1]))
	case "pause":
		if len(args) < 2 {
			m.appendError("usage: /cron pause <id>")
			m.layout()
			return m, nil
		}
		return m.switchView(ViewCron, m.setCronJobEnabledCmd(args[1], false))
	case "resume":
		if len(args) < 2 {
			m.appendError("usage: /cron resume <id>")
			m.layout()
			return m, nil
		}
		return m.switchView(ViewCron, m.setCronJobEnabledCmd(args[1], true))
	case "remove":
		if len(args) < 2 {
			m.appendError("usage: /cron remove <id>")
			m.layout()
			return m, nil
		}
		return m.switchView(ViewCron, m.deleteCronJobCmd(args[1]))
	default:
		m.appendError("unknown cron command: " + args[0])
		m.layout()
		return m, nil
	}
}

func formatSkillList(catalog extensions.Catalog) []string {
	lines := make([]string, 0, len(catalog.Skills))
	for _, skill := range catalog.Skills {
		line := skill.Name + " [" + skill.Scope + "]"
		if desc := trim(skill.Description); desc != "" {
			line += " — " + desc
		}
		lines = append(lines, line)
	}
	return lines
}

func formatToolList(catalog extensions.Catalog) []string {
	lines := make([]string, 0, len(catalog.Tools))
	for _, tool := range catalog.Tools {
		line := tool.Name + " [" + tool.Scope + "]"
		if desc := trim(tool.Description); desc != "" {
			line += " — " + desc
		}
		lines = append(lines, line)
	}
	return lines
}
