package model

import (
	"github.com/johnvanham/bw-monitor/internal/redis"
)

// InitialLoadMsg carries the initial batch of reports loaded from Redis.
type InitialLoadMsg struct {
	Reports []redis.BlockReport
}

// NewReportsMsg carries newly polled reports.
type NewReportsMsg struct {
	Reports []redis.BlockReport
}

// PollTickMsg triggers a new poll cycle.
type PollTickMsg struct{}

// ErrMsg carries an error to display.
type ErrMsg struct {
	Err error
}

// DNSResultMsg carries the result of an async reverse DNS lookup.
type DNSResultMsg struct {
	IP    string
	Names []string
	Err   error
}
