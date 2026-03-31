package model

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/johnvanham/bw-monitor/internal/ui"
)

// RenderTitleBar renders the app title bar with a left title and right context label.
func RenderTitleBar(appName, context string, width int) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#1A1A1A")).
		Background(lipgloss.Color("#4EC9B0"))

	contextStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#1A1A1A")).
		Background(lipgloss.Color("#4EC9B0"))

	left := titleStyle.Render(" " + appName + " ")
	right := contextStyle.Render(" " + context + " ")

	leftLen := lipgloss.Width(left)
	rightLen := lipgloss.Width(right)
	gap := width - leftLen - rightLen
	if gap < 0 {
		gap = 0
	}

	barBg := lipgloss.NewStyle().Background(lipgloss.Color("#4EC9B0"))
	middle := barBg.Render(strings.Repeat(" ", gap))

	return left + middle + right
}

// rebuildReportsContent builds the content lines for the reports viewport
// with cursor highlighting and IP colours baked in.
func (m *Model) rebuildReportsContent() {
	if m.width == 0 {
		return
	}

	lines := make([]string, len(m.filteredIdx))
	for i, fidx := range m.filteredIdx {
		report := &m.allReports[fidx]
		row := ui.FormatReportRow(report, m.width)

		ipColour := ui.ColourForIP(report.IP)
		rowStyle := lipgloss.NewStyle().Foreground(ipColour)

		if i == m.reportsCursor {
			rowStyle = rowStyle.Background(lipgloss.Color("#333333")).Bold(true)
		}

		lines[i] = rowStyle.Render(ui.PadRight(row, m.width))
	}

	m.reportsViewport.SetContentLines(lines)
}

// RenderReportsStatusBar renders the status bar for the reports list view.
func RenderReportsStatusBar(filteredIdx []int, totalReports int, paused bool, filter *Filter, excludeCount int, lastErr error, width int) string {
	var statusParts []string

	if paused {
		statusParts = append(statusParts, ui.PausedStyle.Render("[PAUSED]"))
	} else {
		statusParts = append(statusParts, ui.TitleStyle.Render("[LIVE]"))
	}

	statusParts = append(statusParts, fmt.Sprintf("Showing %d/%d", len(filteredIdx), totalReports))

	if filter.IsActive() {
		statusParts = append(statusParts, ui.FilterActiveStyle.Render("Filter: "+filter.Summary()))
	}

	if excludeCount > 0 {
		statusParts = append(statusParts, ui.DimStyle.Render(fmt.Sprintf("%d IP(s) excluded", excludeCount)))
	}

	if lastErr != nil {
		statusParts = append(statusParts, lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Render("Err: "+lastErr.Error()))
	}

	return ui.StatusBarStyle.Render(ui.PadRight(strings.Join(statusParts, "  |  "), width))
}

// RenderBansHeader returns the column header string for the bans list.
func RenderBansHeader(width int) string {
	return fmt.Sprintf("%s %s %s %s %s %s %s",
		ui.PadRight("IP", 16),
		ui.PadRight("CC", 4),
		ui.PadRight("Service", 30),
		ui.PadRight("Reason", 14),
		ui.PadRight("Banned At", 19),
		ui.PadRight("Expires In", 12),
		ui.PadRight("Events", 8),
	)
}

// RenderBansStatusBar renders the status bar for the bans list view.
func RenderBansStatusBar(filteredCount, totalCount, excludeCount int, filter *Filter, lastErr error, width int) string {
	var statusParts []string
	statusParts = append(statusParts, fmt.Sprintf("Showing %d/%d ban(s)", filteredCount, totalCount))
	if filter.IsActive() {
		statusParts = append(statusParts, ui.FilterActiveStyle.Render("Filter: "+filter.Summary()))
	}
	if excludeCount > 0 {
		statusParts = append(statusParts, ui.DimStyle.Render(fmt.Sprintf("%d IP(s) excluded", excludeCount)))
	}
	if lastErr != nil {
		statusParts = append(statusParts, lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Render("Err: "+lastErr.Error()))
	}
	return ui.StatusBarStyle.Render(ui.PadRight(strings.Join(statusParts, "  |  "), width))
}
