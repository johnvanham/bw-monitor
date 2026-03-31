package model

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/johnvanham/bw-monitor/internal/redis"
)

const filterFileName = ".bw-monitor-filter"

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

// MatchesBan returns true if the ban passes the filter criteria.
func (f *Filter) MatchesBan(b *redis.Ban) bool {
	if f.IP != "" && !strings.Contains(b.IP, f.IP) {
		return false
	}
	if f.Country != "" {
		countries := strings.Split(f.Country, ",")
		matched := false
		for _, c := range countries {
			if strings.EqualFold(strings.TrimSpace(c), b.Country) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if f.Server != "" && !strings.Contains(strings.ToLower(b.Service), strings.ToLower(f.Server)) {
		return false
	}
	if !f.DateFrom.IsZero() && b.Time().Before(f.DateFrom) {
		return false
	}
	if !f.DateTo.IsZero() && b.Time().After(f.DateTo) {
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
	return strings.Join(parts, " / ")
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

func filterPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, filterFileName)
}

// Save persists the current filter to disk.
func (f *Filter) Save() {
	file, err := os.Create(filterPath())
	if err != nil {
		return
	}
	defer file.Close()

	if f.IP != "" {
		file.WriteString("ip=" + f.IP + "\n")
	}
	if f.Country != "" {
		file.WriteString("country=" + f.Country + "\n")
	}
	if f.Server != "" {
		file.WriteString("server=" + f.Server + "\n")
	}
	if !f.DateFrom.IsZero() {
		file.WriteString("from=" + f.DateFrom.Format("2006-01-02 15:04") + "\n")
	}
	if !f.DateTo.IsZero() {
		file.WriteString("to=" + f.DateTo.Format("2006-01-02 15:04") + "\n")
	}
}

// Load restores a filter from disk.
func (f *Filter) Load() {
	file, err := os.Open(filterPath())
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], parts[1]
		switch key {
		case "ip":
			f.IP = val
		case "country":
			f.Country = val
		case "server":
			f.Server = val
		case "from":
			if t, err := time.Parse("2006-01-02 15:04", val); err == nil {
				f.DateFrom = t
			}
		case "to":
			if t, err := time.Parse("2006-01-02 15:04", val); err == nil {
				f.DateTo = t
			}
		}
	}
	f.SetActive()
}

// Delete removes the persisted filter file.
func (f *Filter) Delete() {
	os.Remove(filterPath())
}
