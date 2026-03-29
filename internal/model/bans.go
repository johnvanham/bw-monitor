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

// rebuildBansContent builds the content lines for the bans viewport
// with cursor highlighting and IP colours baked in.
func (m *Model) rebuildBansContent() {
	if m.width == 0 {
		return
	}

	if len(m.bans) == 0 {
		m.bansViewport.SetContentLines([]string{
			"",
			ui.DimStyle.Render("  No active bans"),
		})
		return
	}

	lines := make([]string, len(m.bans))
	for i := range m.bans {
		ban := &m.bans[i]
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
		if i == m.bansCursor {
			rowStyle = rowStyle.Background(lipgloss.Color("#333333")).Bold(true)
		}

		lines[i] = rowStyle.Render(ui.PadRight(row, m.width))
	}

	m.bansViewport.SetContentLines(lines)
}

// BuildBanDetailContent builds the full content string for a ban detail view.
// This content is set on the detail viewport which handles scrolling natively.
func BuildBanDetailContent(ban *redis.Ban, width int, dnsNames []string, dnsLoading bool) string {
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

	return strings.Join(lines, "\n")
}
