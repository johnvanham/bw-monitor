package model

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/biter777/countries"
	"github.com/johnvanham/bw-monitor/internal/redis"
	"github.com/johnvanham/bw-monitor/internal/ui"
)

// RenderBansList renders the bans list view.
func RenderBansList(bans []redis.Ban, cursor, offset, width, height int, lastErr error) string {
	var b strings.Builder

	b.WriteString(RenderTitleBar("BW Monitor", "Active Bans", width))
	b.WriteString("\n")

	// Header
	header := fmt.Sprintf("%s %s %s %s %s %s %s",
		ui.PadRight("IP", 16),
		ui.PadRight("CC", 4),
		ui.PadRight("Service", 30),
		ui.PadRight("Reason", 14),
		ui.PadRight("Banned At", 19),
		ui.PadRight("Expires In", 12),
		ui.PadRight("Events", 8),
	)
	b.WriteString(ui.HeaderStyle.Render(ui.PadRight(header, width)))
	b.WriteString("\n")

	dataRows := height - 4
	if dataRows < 1 {
		dataRows = 1
	}

	if len(bans) == 0 {
		b.WriteString("\n")
		b.WriteString(ui.DimStyle.Render("  No active bans"))
		b.WriteString("\n")
	}

	for i := 0; i < dataRows; i++ {
		idx := offset + i
		if idx >= len(bans) {
			b.WriteString("\n")
			continue
		}

		ban := &bans[idx]
		remaining := ban.TTL
		var expiresIn string
		if ban.Permanent {
			expiresIn = "permanent"
		} else {
			hours := int(remaining.Hours())
			mins := int(remaining.Minutes()) % 60
			expiresIn = fmt.Sprintf("%dh %dm", hours, mins)
		}

		row := fmt.Sprintf("%s %s %s %s %s %s %s",
			ui.PadRight(ban.IP, 16),
			ui.PadRight(ban.Country, 4),
			ui.PadRight(ui.Truncate(ban.Service, 30), 30),
			ui.PadRight(ui.Truncate(ban.Reason, 14), 14),
			ui.PadRight(ui.FormatTime(ban.Time()), 19),
			ui.PadRight(expiresIn, 12),
			ui.PadRight(fmt.Sprintf("%d", len(ban.Events)), 8),
		)

		ipColour := ui.ColourForIP(ban.IP)
		rowStyle := lipgloss.NewStyle().Foreground(ipColour)
		if idx == cursor {
			rowStyle = rowStyle.Background(lipgloss.Color("#333333")).Bold(true)
		}

		b.WriteString(rowStyle.Render(ui.PadRight(row, width)))
		b.WriteString("\n")
	}

	// Status bar
	var statusParts []string
	statusParts = append(statusParts, fmt.Sprintf("%d active ban(s)", len(bans)))
	if lastErr != nil {
		statusParts = append(statusParts, lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Render("Err: "+lastErr.Error()))
	}
	b.WriteString(ui.StatusBarStyle.Render(ui.PadRight(strings.Join(statusParts, "  |  "), width)))
	b.WriteString("\n")

	help := "[1] Reports  [2] Bans  [Enter] Detail  [r] Refresh  [q] Quit"
	b.WriteString(ui.HelpStyle.Render(help))

	return b.String()
}

// RenderBanDetail renders the detail view for a single ban.
func RenderBanDetail(ban *redis.Ban, width, height, offset int, dnsNames []string, dnsLoading bool) string {
	var lines []string

	add := func(s string) {
		lines = append(lines, s)
	}

	field := func(label, value string) {
		add(ui.LabelStyle.Render(label) + ui.ValueStyle.Render(value))
	}

	add("")
	field("IP Address:", ban.IP)

	if dnsLoading {
		field("rDNS:", "(looking up...)")
	} else if len(dnsNames) > 0 {
		field("rDNS:", strings.Join(dnsNames, ", "))
	}

	if c := countries.ByName(ban.Country); c != countries.Unknown {
		field("Country:", fmt.Sprintf("%s (%s)", c.String(), ban.Country))
	} else {
		field("Country:", ban.Country)
	}

	field("Service:", ban.Service)
	field("Reason:", ban.Reason)
	field("Ban Scope:", ban.BanScope)
	field("Banned At:", ban.Time().Format(time.RFC3339))

	if ban.Permanent {
		field("Expires:", "Never (permanent)")
	} else {
		hours := int(ban.TTL.Hours())
		mins := int(ban.TTL.Minutes()) % 60
		field("Expires In:", fmt.Sprintf("%dh %dm", hours, mins))
		field("Expires At:", ban.ExpiresAt().Format(time.RFC3339))
	}

	field("Events:", fmt.Sprintf("%d requests led to this ban", len(ban.Events)))

	if len(ban.Events) > 0 {
		add("")
		add(ui.TitleStyle.Render("  Events Leading to Ban"))
		add("")

		// Show events in chronological order (they're usually already sorted)
		for i, e := range ban.Events {
			evtStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#DCDCAA"))
			add(evtStyle.Render(fmt.Sprintf("  [%d] %s  %s %s  -> %s",
				i+1,
				e.Time().Format("15:04:05"),
				e.Method,
				e.URL,
				e.Status,
			)))
		}

		// Show a summary of the first event's user agent if available from reports
		add("")
		add(ui.DimStyle.Render(fmt.Sprintf("  Ban triggered after %d requests in %ds (threshold: %d)",
			len(ban.Events),
			ban.Events[0].CountTime,
			ban.Events[0].Threshold,
		)))
	}

	// Render with scroll
	var b strings.Builder
	b.WriteString(RenderTitleBar("BW Monitor", "Ban Detail", width))
	b.WriteString("\n")

	contentRows := height - 3
	if contentRows < 1 {
		contentRows = 1
	}

	maxOffset := len(lines) - contentRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}

	for i := 0; i < contentRows; i++ {
		idx := offset + i
		if idx < len(lines) {
			b.WriteString(lines[idx])
		}
		b.WriteString("\n")
	}

	scrollInfo := ""
	if len(lines) > contentRows {
		scrollInfo = fmt.Sprintf("  Line %d/%d", offset+1, len(lines))
	}

	help := ui.HelpStyle.Render("[Esc] Back" + scrollInfo + "  [Up/Down] Scroll")
	b.WriteString(help)

	return b.String()
}
