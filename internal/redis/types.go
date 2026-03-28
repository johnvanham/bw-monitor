package redis

import (
	"encoding/json"
	"time"
)

// BlockReport represents a single blocked request from BunkerWeb's Redis store.
type BlockReport struct {
	ID           string          `json:"id"`
	IP           string          `json:"ip"`
	DateUnix     float64         `json:"date"`
	Country      string          `json:"country"`
	Reason       string          `json:"reason"`
	Method       string          `json:"method"`
	URL          string          `json:"url"`
	Status       int             `json:"status"`
	UserAgent    string          `json:"user_agent"`
	ServerName   string          `json:"server_name"`
	SecurityMode string          `json:"security_mode"`
	Synced       bool            `json:"synced"`
	RawData      json.RawMessage `json:"data"`

	// Parsed from RawData
	BadBehaviorDetails []BadBehaviorDetail `json:"-"`
}

// Time returns the report's timestamp as a time.Time.
func (r *BlockReport) Time() time.Time {
	sec := int64(r.DateUnix)
	nsec := int64((r.DateUnix - float64(sec)) * 1e9)
	return time.Unix(sec, nsec)
}

// BadBehaviorDetail contains details from bad behavior ban events.
type BadBehaviorDetail struct {
	IP           string  `json:"ip"`
	Date         float64 `json:"date"`
	Country      string  `json:"country"`
	BanTime      int     `json:"ban_time"`
	BanScope     string  `json:"ban_scope"`
	Threshold    int     `json:"threshold"`
	URL          string  `json:"url"`
	ServerName   string  `json:"server_name"`
	Method       string  `json:"method"`
	ID           string  `json:"id"`
	CountTime    int     `json:"count_time"`
	Status       string  `json:"status"`
	SecurityMode string  `json:"security_mode"`
}

// ParseData attempts to unmarshal the RawData field into BadBehaviorDetails.
func (r *BlockReport) ParseData() {
	if len(r.RawData) == 0 {
		return
	}
	var details []BadBehaviorDetail
	if err := json.Unmarshal(r.RawData, &details); err == nil && len(details) > 0 {
		r.BadBehaviorDetails = details
	}
}
