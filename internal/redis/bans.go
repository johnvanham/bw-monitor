package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Ban represents a currently active ban from BunkerWeb's Redis store.
type Ban struct {
	Key        string          // Redis key (e.g. bans_service_www.ter-europe.org_ip_1.2.3.4)
	IP         string          // Parsed from key
	Service    string          `json:"service"`
	Reason     string          `json:"reason"`
	DateUnix   float64         `json:"date"`
	Country    string          `json:"country"`
	BanScope   string          `json:"ban_scope"`
	Permanent  bool            `json:"permanent"`
	RawData    json.RawMessage `json:"reason_data"`
	TTL        time.Duration   // Remaining ban time from Redis TTL
	Events     []BanEvent      `json:"-"` // Parsed from RawData
}

// Time returns the ban's timestamp as a time.Time.
func (b *Ban) Time() time.Time {
	sec := int64(b.DateUnix)
	nsec := int64((b.DateUnix - float64(sec)) * 1e9)
	return time.Unix(sec, nsec)
}

// ExpiresAt returns when the ban expires.
func (b *Ban) ExpiresAt() time.Time {
	return time.Now().Add(b.TTL)
}

// ParseData attempts to unmarshal the RawData field into Events.
func (b *Ban) ParseData() {
	if len(b.RawData) == 0 {
		return
	}
	var events []BanEvent
	if err := json.Unmarshal(b.RawData, &events); err == nil {
		b.Events = events
	}
}

// BanEvent is a single event that contributed to a ban.
type BanEvent struct {
	ID           string  `json:"id"`
	IP           string  `json:"ip"`
	Date         float64 `json:"date"`
	Country      string  `json:"country"`
	Method       string  `json:"method"`
	URL          string  `json:"url"`
	Status       string  `json:"status"`
	ServerName   string  `json:"server_name"`
	SecurityMode string  `json:"security_mode"`
	BanScope     string  `json:"ban_scope"`
	BanTime      int     `json:"ban_time"`
	CountTime    int     `json:"count_time"`
	Threshold    int     `json:"threshold"`
}

// Time returns the event's timestamp as a time.Time.
func (e *BanEvent) Time() time.Time {
	sec := int64(e.Date)
	nsec := int64((e.Date - float64(sec)) * 1e9)
	return time.Unix(sec, nsec)
}

// LoadBans fetches all active bans from Redis.
func (c *Client) LoadBans(ctx context.Context) ([]Ban, error) {
	keys, err := c.rdb.Keys(ctx, "bans_*").Result()
	if err != nil {
		return nil, fmt.Errorf("scanning ban keys: %w", err)
	}

	var bans []Ban
	for _, key := range keys {
		ban, err := c.loadBan(ctx, key)
		if err != nil {
			continue
		}
		bans = append(bans, *ban)
	}

	return bans, nil
}

func (c *Client) loadBan(ctx context.Context, key string) (*Ban, error) {
	// Get value and TTL in a pipeline
	pipe := c.rdb.Pipeline()
	getCmd := pipe.Get(ctx, key)
	ttlCmd := pipe.TTL(ctx, key)
	_, err := pipe.Exec(ctx)
	if err != nil && err != goredis.Nil {
		return nil, fmt.Errorf("loading ban %s: %w", key, err)
	}

	val := getCmd.Val()
	if val == "" {
		return nil, fmt.Errorf("ban key %s not found", key)
	}

	var ban Ban
	if err := json.Unmarshal([]byte(val), &ban); err != nil {
		return nil, fmt.Errorf("parsing ban %s: %w", key, err)
	}

	ban.Key = key
	ban.TTL = ttlCmd.Val()
	ban.IP = parseIPFromKey(key)
	ban.ParseData()

	return &ban, nil
}

// parseIPFromKey extracts the IP from a ban key like "bans_service_example.com_ip_1.2.3.4"
func parseIPFromKey(key string) string {
	parts := strings.Split(key, "_ip_")
	if len(parts) == 2 {
		return parts[1]
	}
	return key
}
