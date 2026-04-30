package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// NightOwl-inspired palette — warm, sophisticated, dark background.
// The terminal is a slate/blue-black; entries pop with warm accents.
// Avoid pure primaries; favor muted tones with clear saturation for meaning.

const (
	// Base
	colorBg      = "17" // deep slate #0d1117
	colorSurface = "59" // raised surface #1c2128
	colorBorder  = "60" // subtle border

	// Brand & identity
	colorTitle    = "79" // teal #39d0c0 — Sesame brand
	colorTitleDim = "67" // dimmer teal for secondary

	// Entry kinds — warm on dark
	colorUser         = "210" // coral #ff7b72 — user messages
	colorUserDim      = "124" // muted red for secondary text
	colorAssistant    = "110" // mint green #88d9a0 — assistant
	colorAssistantDim = "114" // dimmer mint for secondary
	colorTool         = "215" // amber #ffb347 — tools/actions
	colorToolDim      = "137" // warm olive for detail
	colorToolTarget   = "230" // near-white for tool target

	// Semantic
	colorNotice      = "221" // warm yellow #f0e68c
	colorError       = "203" // soft red #ff6b6b
	colorActivity    = "102" // cool slate #9aa5ce
	colorActivityDim = "68"  // dimmed activity

	// Status
	colorTabActive   = "79"
	colorTabInactive = "240"
	colorTabBgActive = "59"
	colorRunning     = "215" // amber running dot
	colorQueue       = "110" // mint for queue

	// Input
	colorInputText   = "252"
	colorPlaceholder = "238"
	colorInputBorder = "67"

	// Metadata
	colorMeta      = "68" // muted gray
	colorStatus    = "68"
	colorHighlight = "79" // teal highlight
)

// Base styles
var (
	// Title bar: bold teal
	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorTitle))

	// Metadata: muted
	StyleMeta = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMeta))

	// Status line
	StyleStatus = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorStatus))

	// Header border: bottom only, subtle
	StyleBorder = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderTop(false).
			BorderLeft(false).
			BorderRight(false).
			BorderForeground(lipgloss.Color(colorBorder))

	// Input area: teal border when focused
	StyleInputFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colorInputBorder))

	// Tabs
	StyleTabActive = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color(colorTabBgActive)).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorTabActive))

	StyleTabInactive = lipgloss.NewStyle().
				Padding(0, 1).
				Foreground(lipgloss.Color(colorTabInactive)).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colorTabInactive))

	// Entry styles — label + body pair
	StyleEntryUser = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorUser))

	StyleEntryUserDim = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorUserDim))

	StyleEntryAssistant = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(colorAssistant))

	StyleEntryAssistantDim = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorAssistantDim))

	StyleEntryToolAction = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(colorTool))

	StyleEntryToolTarget = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorToolTarget))

	StyleEntryToolStatus = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorToolDim))

	// Tool state indicators — rendered inline
	StyleToolRunning = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorRunning))

	StyleToolCompleted = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorAssistant))

	StyleToolFailed = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorError))

	StyleEntryNotice = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(colorNotice))

	StyleEntryError = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorError))

	StyleEntryActivity = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(colorActivity))

	StyleEntryActivityDim = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorActivityDim))

	// General text
	StyleBody = lipgloss.NewStyle()

	StyleMuted = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMeta))

	StyleErrorBlock = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorError))

	StyleSectionHeading = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(colorTitle))

	// Push bar (reports notification)
	StylePushBar = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorHighlight))

	StylePushBarText = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorToolTarget))

	StylePushBarHint = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorMeta))

	// Running indicator
	StyleRunning = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorRunning))

	StyleQueue = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorQueue))

	// Cron job
	StyleCronJobTitle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(colorUser))

	StyleCronJobTitleSelected = lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color(colorHighlight))

	// Tool panel background (used for grouped tool entries)
	StyleToolPanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorBorder)).
			Padding(0, 1)

	// Code blocks inside tool panels
	StyleToolCode = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorToolTarget)).
			Background(lipgloss.Color(colorSurface)).
			Padding(0, 1).
			Border(lipgloss.HiddenBorder())

	// Surface panel for activity blocks
	StyleSurfacePanel = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colorBorder)).
				Padding(0, 1)
)
