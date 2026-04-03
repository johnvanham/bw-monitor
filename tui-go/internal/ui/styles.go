package ui

import "charm.land/lipgloss/v2"

var (
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#333333"))

	SelectedStyle = lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color("#444444"))

	StatusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Background(lipgloss.Color("#1A1A1A"))

	PausedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6B6B"))

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#4EC9B0"))

	LabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#569CD6")).
			Width(16)

	ValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D4D4D4"))

	DimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555555"))

	FilterActiveStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#DCDCAA"))

	HelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666"))
)
