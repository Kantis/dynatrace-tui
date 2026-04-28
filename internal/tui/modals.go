package tui

import (
	"github.com/charmbracelet/lipgloss"
)

type modalKind int

const (
	modalNone modalKind = iota
	modalTimeRange
	modalSaveQuery
	modalTemplate
	modalExport
	modalSwitchEnv
)

var modalBox = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorAccent).
	Padding(1, 2)

func (m Model) renderModalOverlay(content string) string {
	box := modalBox.Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
