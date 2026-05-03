package tui

import "github.com/charmbracelet/lipgloss"

const (
	colorBorder      = "60"
	colorSurface     = "59"
	colorTitle       = "79"
	colorUser        = "210"
	colorAssistant   = "110"
	colorTool        = "215"
	colorToolTarget  = "230"
	colorNotice      = "221"
	colorError       = "203"
	colorActivity    = "102"
	colorActivityDim = "68"
	colorTabInactive = "240"
	colorRunning     = "215"
	colorQueue       = "110"
	colorInputText   = "252"
	colorPlaceholder = "238"
	colorInputBorder = "67"
	colorMeta        = "68"
	colorHighlight   = "79"
)

var (
	StyleTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorTitle))
	StyleStatus = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMeta))
	StyleBorder = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderTop(false).
			BorderLeft(false).
			BorderRight(false).
			BorderForeground(lipgloss.Color(colorBorder))
	StyleInputFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colorInputBorder))
	StyleTabActive = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color(colorSurface)).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorTitle))
	StyleTabInactive = lipgloss.NewStyle().
				Padding(0, 1).
				Foreground(lipgloss.Color(colorTabInactive)).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colorTabInactive))
	StyleEntryUser        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorUser))
	StyleEntryAssistant   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAssistant))
	StyleEntryToolAction  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorTool))
	StyleEntryToolTarget  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorToolTarget))
	StyleEntryToolStatus  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMeta))
	StyleToolRunning      = lipgloss.NewStyle().Foreground(lipgloss.Color(colorRunning))
	StyleToolCompleted    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAssistant))
	StyleToolFailed       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorError))
	StyleEntryNotice      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorNotice))
	StyleEntryError       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorError))
	StyleEntryActivity    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorActivity))
	StyleEntryActivityDim = lipgloss.NewStyle().Foreground(lipgloss.Color(colorActivityDim))
	StyleBody             = lipgloss.NewStyle()
	StyleMuted            = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMeta))
	StyleErrorBlock       = lipgloss.NewStyle().Foreground(lipgloss.Color(colorError))
	StyleSectionHeading   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorTitle))
	StyleRunning          = lipgloss.NewStyle().Foreground(lipgloss.Color(colorRunning))
	StyleQueue            = lipgloss.NewStyle().Foreground(lipgloss.Color(colorQueue))
	StyleToolPanel        = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colorBorder)).
				Padding(0, 1)
	StyleToolCode = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorToolTarget)).
			Background(lipgloss.Color(colorSurface)).
			Padding(0, 1)
	StyleSurfacePanel = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colorBorder)).
				Padding(0, 1)
)
