package ui

import (
	"fmt"
	"strings"
	"time"

	useragent "github.com/mileusna/useragent"
	"github.com/johnvanham/bw-monitor/internal/redis"
)

// Column widths
const (
	ColTime       = 19
	ColIP         = 16
	ColCountry    = 4
	ColMethod     = 7
	ColStatus     = 6
	ColReason     = 14
	ColServerName = 28
)

// FormatTime formats a unix timestamp for display.
func FormatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// Truncate truncates a string to maxLen, adding ellipsis if needed.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// PadRight pads a string to width with spaces.
func PadRight(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

// FormatHeaderRow returns the column header string.
func FormatHeaderRow(totalWidth int) string {
	urlWidth, uaWidth := calcFlexWidths(totalWidth)
	return fmt.Sprintf("%s %s %s %s %s %s %s %s %s",
		PadRight("Time", ColTime),
		PadRight("IP", ColIP),
		PadRight("CC", ColCountry),
		PadRight("Method", ColMethod),
		PadRight("Status", ColStatus),
		PadRight("Reason", ColReason),
		PadRight("Server", ColServerName),
		PadRight("URL", urlWidth),
		PadRight("User Agent", uaWidth),
	)
}

// FormatReportRow formats a single report as a table row.
func FormatReportRow(r *redis.BlockReport, totalWidth int) string {
	urlWidth, uaWidth := calcFlexWidths(totalWidth)
	return fmt.Sprintf("%s %s %s %s %s %s %s %s %s",
		PadRight(FormatTime(r.Time()), ColTime),
		PadRight(r.IP, ColIP),
		PadRight(r.Country, ColCountry),
		PadRight(r.Method, ColMethod),
		PadRight(fmt.Sprintf("%d", r.Status), ColStatus),
		PadRight(Truncate(r.Reason, ColReason), ColReason),
		PadRight(Truncate(r.ServerName, ColServerName), ColServerName),
		PadRight(Truncate(r.URL, urlWidth), urlWidth),
		PadRight(Truncate(ParsedUA(r.UserAgent), uaWidth), uaWidth),
	)
}

// ParsedUA returns a short summary of the parsed user agent string.
func ParsedUA(raw string) string {
	if raw == "" || raw == "-" {
		return "-"
	}
	ua := useragent.Parse(raw)
	var parts []string
	if ua.Name != "" {
		v := ua.Name
		if ua.Version != "" {
			v += " " + ua.Version
		}
		parts = append(parts, v)
	}
	if ua.OS != "" {
		parts = append(parts, ua.OS)
	}
	if ua.Bot {
		parts = append(parts, "Bot")
	} else if ua.Mobile {
		parts = append(parts, "Mobile")
	} else if ua.Tablet {
		parts = append(parts, "Tablet")
	} else if ua.Desktop {
		parts = append(parts, "Desktop")
	}
	if len(parts) == 0 {
		return Truncate(raw, 30)
	}
	return strings.Join(parts, " / ")
}

func calcFlexWidths(totalWidth int) (urlWidth, uaWidth int) {
	fixedCols := ColTime + ColIP + ColCountry + ColMethod + ColStatus + ColReason + ColServerName + 8 // 8 spaces between 9 columns
	remaining := totalWidth - fixedCols
	if remaining < 20 {
		remaining = 20
	}
	// Give 60% to URL, 40% to UA
	urlWidth = remaining * 60 / 100
	uaWidth = remaining - urlWidth
	return
}
