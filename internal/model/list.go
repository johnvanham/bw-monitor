package model

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/johnvanham/bw-monitor/internal/redis"
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

// RenderList renders the list view with header, report rows, and status bar.
func RenderList(reports []redis.BlockReport, filteredIdx []int, cursor, offset, width, height int, paused bool, filter *Filter, totalReports, excludeCount int, lastErr error) string {
	var b strings.Builder

	// Title bar
	b.WriteString(RenderTitleBar("BW Monitor", "Live View", width))
	b.WriteString("\n")

	// Header row
	header := ui.HeaderStyle.Render(ui.PadRight(ui.FormatHeaderRow(width), width))
	b.WriteString(header)
	b.WriteString("\n")

	// Available rows for data (minus title bar, header, status bar, help bar)
	dataRows := height - 4
	if dataRows < 1 {
		dataRows = 1
	}

	// Render visible rows
	for i := 0; i < dataRows; i++ {
		idx := offset + i
		if idx >= len(filteredIdx) {
			b.WriteString("\n")
			continue
		}

		reportIdx := filteredIdx[idx]
		report := &reports[reportIdx]
		row := ui.FormatReportRow(report, width)

		// Apply IP colour
		ipColour := ui.ColourForIP(report.IP)
		rowStyle := lipgloss.NewStyle().Foreground(ipColour)

		if idx == cursor {
			rowStyle = rowStyle.Background(lipgloss.Color("#333333")).Bold(true)
		}

		b.WriteString(rowStyle.Render(ui.PadRight(row, width)))
		b.WriteString("\n")
	}

	// Status bar
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

	statusLine := ui.StatusBarStyle.Render(ui.PadRight(strings.Join(statusParts, "  |  "), width))
	b.WriteString(statusLine)
	b.WriteString("\n")

	// Help bar
	help := "[1] Reports  [2] Bans  [Space] Pause  [Enter] Detail  [f] Filter  [c] Clear  [x] Exclude IP  [X] Excludes  [q] Quit"
	b.WriteString(ui.HelpStyle.Render(help))

	return b.String()
}
