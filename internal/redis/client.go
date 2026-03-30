package redis

import (
	"context"
	"encoding/json"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

// Client reads block reports from BunkerWeb's Redis instance.
type Client struct {
	rdb *goredis.Client
	// highwater tracks the last known length of the requests list
	highwater int64
}

// NewClient creates a Redis client connected to the given address.
func NewClient(addr string) *Client {
	rdb := goredis.NewClient(&goredis.Options{
		Addr: addr,
	})
	return &Client{rdb: rdb}
}

// Ping verifies the Redis connection.
func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// Close closes the Redis connection.
func (c *Client) Close() error {
	return c.rdb.Close()
}

// LoadInitial loads the most recent reports from Redis.
// If maxEntries <= 0, all entries are loaded.
// The Redis list is append-ordered: index 0 = oldest, index -1 = newest.
// Reports are returned newest-first for display.
func (c *Client) LoadInitial(ctx context.Context, maxEntries int) ([]BlockReport, error) {
	total, err := c.rdb.LLen(ctx, "requests").Result()
	if err != nil {
		return nil, fmt.Errorf("LLEN: %w", err)
	}

	// Fetch the last maxEntries (newest) from the list, or all if <= 0
	var start int64
	if maxEntries > 0 {
		start = total - int64(maxEntries)
		if start < 0 {
			start = 0
		}
	}

	reports, err := c.fetchRange(ctx, start, -1)
	if err != nil {
		return nil, err
	}

	// Reverse so newest is first
	reverseReports(reports)

	c.highwater = total
	return reports, nil
}

// PollNew fetches any new reports appended since the last poll.
// Returns new reports newest-first.
func (c *Client) PollNew(ctx context.Context) ([]BlockReport, error) {
	total, err := c.rdb.LLen(ctx, "requests").Result()
	if err != nil {
		return nil, fmt.Errorf("LLEN: %w", err)
	}

	if total <= c.highwater {
		return nil, nil
	}

	// Fetch entries from highwater to end (new entries appended at the end)
	reports, err := c.fetchRange(ctx, c.highwater, -1)
	if err != nil {
		return nil, err
	}

	// Reverse so newest is first
	reverseReports(reports)

	c.highwater = total
	return reports, nil
}

func (c *Client) fetchRange(ctx context.Context, start, end int64) ([]BlockReport, error) {
	vals, err := c.rdb.LRange(ctx, "requests", start, end).Result()
	if err != nil {
		return nil, fmt.Errorf("LRANGE requests %d %d: %w", start, end, err)
	}

	var reports []BlockReport
	for _, val := range vals {
		var report BlockReport
		if err := json.Unmarshal([]byte(val), &report); err != nil {
			continue
		}
		report.ParseData()
		reports = append(reports, report)
	}

	return reports, nil
}

func reverseReports(reports []BlockReport) {
	for i, j := 0, len(reports)-1; i < j; i, j = i+1, j-1 {
		reports[i], reports[j] = reports[j], reports[i]
	}
}
