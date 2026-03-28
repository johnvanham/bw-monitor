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

// LoadInitial loads up to maxEntries reports from Redis.
// Reports are returned newest-first (index 0 in Redis is newest).
func (c *Client) LoadInitial(ctx context.Context, maxEntries int) ([]BlockReport, error) {
	total, err := c.rdb.LLen(ctx, "requests").Result()
	if err != nil {
		return nil, fmt.Errorf("LLEN: %w", err)
	}

	end := total - 1
	start := int64(0)
	if maxEntries > 0 && total > int64(maxEntries) {
		end = int64(maxEntries) - 1
	}

	reports, err := c.fetchRange(ctx, start, end)
	if err != nil {
		return nil, err
	}

	c.highwater = total
	return reports, nil
}

// PollNew fetches any new reports added since the last poll.
func (c *Client) PollNew(ctx context.Context) ([]BlockReport, error) {
	total, err := c.rdb.LLen(ctx, "requests").Result()
	if err != nil {
		return nil, fmt.Errorf("LLEN: %w", err)
	}

	newCount := total - c.highwater
	if newCount <= 0 {
		return nil, nil
	}

	reports, err := c.fetchRange(ctx, 0, newCount-1)
	if err != nil {
		return nil, err
	}

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
