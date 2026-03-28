package ui

import (
	"fmt"
	"strings"
	"time"

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
	ColURL        = 0 // fills remaining space
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
	urlWidth := calcURLWidth(totalWidth)
	return fmt.Sprintf("%s %s %s %s %s %s %s %s",
		PadRight("Time", ColTime),
		PadRight("IP", ColIP),
		PadRight("CC", ColCountry),
		PadRight("Method", ColMethod),
		PadRight("Status", ColStatus),
		PadRight("Reason", ColReason),
		PadRight("Server", ColServerName),
		PadRight("URL", urlWidth),
	)
}

// FormatReportRow formats a single report as a table row.
func FormatReportRow(r *redis.BlockReport, totalWidth int) string {
	urlWidth := calcURLWidth(totalWidth)
	return fmt.Sprintf("%s %s %s %s %s %s %s %s",
		PadRight(FormatTime(r.Time()), ColTime),
		PadRight(r.IP, ColIP),
		PadRight(r.Country, ColCountry),
		PadRight(r.Method, ColMethod),
		PadRight(fmt.Sprintf("%d", r.Status), ColStatus),
		PadRight(Truncate(r.Reason, ColReason), ColReason),
		PadRight(Truncate(r.ServerName, ColServerName), ColServerName),
		PadRight(Truncate(r.URL, urlWidth), urlWidth),
	)
}

func calcURLWidth(totalWidth int) int {
	used := ColTime + ColIP + ColCountry + ColMethod + ColStatus + ColReason + ColServerName + 7 // 7 spaces between columns
	remaining := totalWidth - used
	if remaining < 10 {
		remaining = 10
	}
	return remaining
}
