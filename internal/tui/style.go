package tui

import (
	"time"

	"github.com/charmbracelet/lipgloss"
)

const apiCancelTimeout = 10 * time.Second

var (
	colorAccent  = lipgloss.Color("#7d56f4")
	colorMuted   = lipgloss.Color("240")
	colorError   = lipgloss.Color("160")
	colorOK      = lipgloss.Color("42")
	colorPending = lipgloss.Color("214") // amber — contrasts the purple accent and reads as "staged"

	paneTitle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			Padding(0, 1)

	paneTitleFocused = paneTitle.
				Background(colorAccent).
				Foreground(lipgloss.Color("231"))

	paneBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted)

	paneBorderFocused = paneBorder.
				BorderForeground(colorAccent)

	statusBar = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	errorText = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	okText    = lipgloss.NewStyle().Foreground(colorOK)

	pendingNudgeStyle = lipgloss.NewStyle().
				Background(colorPending).
				Foreground(lipgloss.Color("16")).
				Bold(true)
)
