package ui

import (
	"hash/fnv"
	"image/color"

	"charm.land/lipgloss/v2"
)

// Palette of colours distinguishable on a black background terminal.
// Carefully chosen to avoid dark colours and maintain readability.
var ipPalette = []color.Color{
	lipgloss.Color("#FF6B6B"), // coral red
	lipgloss.Color("#4EC9B0"), // teal
	lipgloss.Color("#DCDCAA"), // warm yellow
	lipgloss.Color("#569CD6"), // soft blue
	lipgloss.Color("#C586C0"), // purple/magenta
	lipgloss.Color("#4FC1FF"), // bright cyan
	lipgloss.Color("#CE9178"), // orange/rust
	lipgloss.Color("#B5CEA8"), // soft green
	lipgloss.Color("#D7BA7D"), // gold
	lipgloss.Color("#9CDCFE"), // ice blue
	lipgloss.Color("#F48771"), // salmon
	lipgloss.Color("#7EC8E3"), // sky blue
	lipgloss.Color("#C3E88D"), // lime green
	lipgloss.Color("#F78C6C"), // bright orange
	lipgloss.Color("#FF79C6"), // pink
	lipgloss.Color("#8BE9FD"), // aqua
	lipgloss.Color("#50FA7B"), // bright green
	lipgloss.Color("#FFB86C"), // peach
	lipgloss.Color("#BD93F9"), // violet
	lipgloss.Color("#F1FA8C"), // pale yellow
}

// ColourForIP returns a deterministic colour for a given IP address.
func ColourForIP(ip string) color.Color {
	h := fnv.New32a()
	h.Write([]byte(ip))
	idx := int(h.Sum32()) % len(ipPalette)
	return ipPalette[idx]
}
