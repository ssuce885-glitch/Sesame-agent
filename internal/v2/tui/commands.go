package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"go-agent/internal/skillcatalog"
)

func (m *Model) handleCommand(line string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "/"))
	if len(fields) == 0 {
		return m, nil
	}

	switch fields[0] {
	case "help":
		m.appendActivity("commands", commandHelpText())
	case "clear":
		m.entries = nil
		m.toolIndexByCall = make(map[string]int)
		m.toolIndexByKey = make(map[string]int)
	case "status":
		return m, m.refreshStatusCmd(true)
	case "tools":
		if err := m.refreshCatalog(); err != nil {
			m.appendError(err.Error())
			break
		}
		lines := formatToolList(m.catalog)
		if len(lines) == 0 {
			lines = append(lines, "No tools found in the local catalog.")
		}
		m.appendActivity("tools", strings.Join(lines, "\n"))
	case "skills":
		if err := m.refreshCatalog(); err != nil {
			m.appendError(err.Error())
			break
		}
		lines := formatSkillList(m.catalog)
		if len(lines) == 0 {
			lines = append(lines, "No skills found in the local catalog.")
		}
		m.appendActivity("skills", strings.Join(lines, "\n"))
	case "reports":
		return m.switchView(ViewReports, m.loadReportsCmd())
	case "automations":
		return m, m.loadAutomationsCmd()
	case "chat":
		return m.switchView(ViewChat, nil)
	case "project_state":
		summary := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "/project_state"))
		if summary == "" {
			return m, m.loadProjectStateCmd()
		}
		return m, m.updateProjectStateCmd(summary)
	case "project_state_auto":
		if len(fields) == 1 {
			return m, m.loadProjectStateAutoCmd()
		}
		switch strings.ToLower(fields[1]) {
		case "on", "true", "enable", "enabled":
			return m, m.setProjectStateAutoCmd("true")
		case "off", "false", "disable", "disabled":
			return m, m.setProjectStateAutoCmd("false")
		default:
			m.appendError("usage: /project_state_auto [on|off]")
		}
	case "exit", "quit":
		return m, tea.Quit
	default:
		m.appendError("unknown command: /" + fields[0])
	}
	m.layout()
	return m, nil
}

func commandHelpText() string {
	return strings.Join([]string{
		"/help /clear /status /tools /skills",
		"/reports /automations /chat /project_state [text]",
		"/project_state_auto [on|off] /exit",
	}, "\n")
}

var _ = skillcatalog.Catalog{}
