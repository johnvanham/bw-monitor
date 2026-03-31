package model

import (
	"strings"
	"time"

	"github.com/johnvanham/bw-monitor/internal/redis"
)

// Filter holds the current filter criteria.
type Filter struct {
	IP        string
	Country   string
	Server    string
	DateFrom  time.Time
	DateTo    time.Time
	active    bool
}

// IsActive returns true if any filter criteria are set.
func (f *Filter) IsActive() bool {
	return f.active
}

// Apply filters a slice of reports, returning indices into the original slice
// that match the filter criteria.
func (f *Filter) Apply(reports []redis.BlockReport) []int {
	var indices []int
	for i := range reports {
		if f.Matches(&reports[i]) {
			indices = append(indices, i)
		}
	}
	return indices
}

// Matches returns true if the report passes the filter criteria.
func (f *Filter) Matches(r *redis.BlockReport) bool {
	if f.IP != "" && !strings.Contains(r.IP, f.IP) {
		return false
	}
	if f.Country != "" {
		countries := strings.Split(f.Country, ",")
		matched := false
		for _, c := range countries {
			if strings.EqualFold(strings.TrimSpace(c), r.Country) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if f.Server != "" && !strings.Contains(strings.ToLower(r.ServerName), strings.ToLower(f.Server)) {
		return false
	}
	if !f.DateFrom.IsZero() && r.Time().Before(f.DateFrom) {
		return false
	}
	if !f.DateTo.IsZero() && r.Time().After(f.DateTo) {
		return false
	}
	return true
}

// Summary returns a human-readable summary of the active filters.
func (f *Filter) Summary() string {
	if !f.active {
		return ""
	}
	var parts []string
	if f.IP != "" {
		parts = append(parts, "IP:"+f.IP)
	}
	if f.Country != "" {
		parts = append(parts, "CC:"+f.Country)
	}
	if f.Server != "" {
		parts = append(parts, "Server:"+f.Server)
	}
	if !f.DateFrom.IsZero() {
		parts = append(parts, "From:"+f.DateFrom.Format("2006-01-02 15:04"))
	}
	if !f.DateTo.IsZero() {
		parts = append(parts, "To:"+f.DateTo.Format("2006-01-02 15:04"))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " | ")
}

// Clear resets all filter criteria.
func (f *Filter) Clear() {
	f.IP = ""
	f.Country = ""
	f.Server = ""
	f.DateFrom = time.Time{}
	f.DateTo = time.Time{}
	f.active = false
}

// SetActive marks the filter as active.
func (f *Filter) SetActive() {
	f.active = f.IP != "" || f.Country != "" || f.Server != "" || !f.DateFrom.IsZero() || !f.DateTo.IsZero()
}
