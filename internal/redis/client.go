package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/johnvanham/bw-monitor/internal/k8s"
)

// Client reads block reports from BunkerWeb's Redis instance via pod exec.
type Client struct {
	k8s     *k8s.Client
	podName string
	// highwater tracks the last known length of the requests list
	highwater int
}

// NewClient creates a Redis client that communicates via kubectl exec.
func NewClient(k8sClient *k8s.Client, podName string) *Client {
	return &Client{
		k8s:     k8sClient,
		podName: podName,
	}
}

// LoadInitial loads up to maxEntries reports from Redis.
// Reports are returned newest-first (index 0 in Redis is newest).
func (c *Client) LoadInitial(ctx context.Context, maxEntries int) ([]BlockReport, error) {
	// Get list length
	lenStr, err := c.k8s.ExecRedis(ctx, c.podName, "LLEN", "requests")
	if err != nil {
		return nil, fmt.Errorf("LLEN: %w", err)
	}
	total, err := strconv.Atoi(strings.TrimSpace(lenStr))
	if err != nil {
		return nil, fmt.Errorf("parsing LLEN result %q: %w", lenStr, err)
	}

	end := total - 1
	start := 0
	if maxEntries > 0 && total > maxEntries {
		start = 0
		end = maxEntries - 1
	}

	reports, err := c.fetchRange(ctx, start, end)
	if err != nil {
		return nil, err
	}

	// Set highwater to the number of entries we've seen
	if maxEntries > 0 && total > maxEntries {
		c.highwater = total
	} else {
		c.highwater = total
	}

	return reports, nil
}

// PollNew fetches any new reports added since the last poll.
// Returns new reports (newest first) and any error.
func (c *Client) PollNew(ctx context.Context) ([]BlockReport, error) {
	lenStr, err := c.k8s.ExecRedis(ctx, c.podName, "LLEN", "requests")
	if err != nil {
		return nil, fmt.Errorf("LLEN: %w", err)
	}
	total, err := strconv.Atoi(strings.TrimSpace(lenStr))
	if err != nil {
		return nil, fmt.Errorf("parsing LLEN result: %w", err)
	}

	newCount := total - c.highwater
	if newCount <= 0 {
		return nil, nil
	}

	// New entries are prepended at index 0, so fetch 0 to newCount-1
	reports, err := c.fetchRange(ctx, 0, newCount-1)
	if err != nil {
		return nil, err
	}

	c.highwater = total
	return reports, nil
}

func (c *Client) fetchRange(ctx context.Context, start, end int) ([]BlockReport, error) {
	output, err := c.k8s.ExecRedis(ctx, c.podName,
		"LRANGE", "requests", strconv.Itoa(start), strconv.Itoa(end))
	if err != nil {
		return nil, fmt.Errorf("LRANGE requests %d %d: %w", start, end, err)
	}

	return parseReports(output)
}

func parseReports(output string) ([]BlockReport, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var reports []BlockReport

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var report BlockReport
		if err := json.Unmarshal([]byte(line), &report); err != nil {
			// Skip unparseable lines
			continue
		}
		report.ParseData()
		reports = append(reports, report)
	}

	return reports, nil
}
