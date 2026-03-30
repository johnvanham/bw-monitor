package redis

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/johnvanham/bw-monitor/internal/k8s"
)

// Reconnector manages the port-forward and Redis client lifecycle,
// automatically reconnecting when the connection drops.
type Reconnector struct {
	k8s       *k8s.Client
	pf        *k8s.PortForward
	client    *Client
	redisPod  string
	mu        sync.Mutex
	lastRetry time.Time
}

// NewReconnector creates a Reconnector with an existing connection.
func NewReconnector(k8sClient *k8s.Client, redisPod string, pf *k8s.PortForward, client *Client) *Reconnector {
	return &Reconnector{
		k8s:      k8sClient,
		pf:       pf,
		client:   client,
		redisPod: redisPod,
	}
}

// Client returns the current Redis client.
func (r *Reconnector) Client() *Client {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.client
}

// Reconnect tears down the existing connection and establishes a new one.
// Returns an error if reconnection fails. Safe to call concurrently.
func (r *Reconnector) Reconnect() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Rate limit reconnection attempts
	if time.Since(r.lastRetry) < 3*time.Second {
		return fmt.Errorf("reconnecting (rate limited)")
	}
	r.lastRetry = time.Now()

	// Close old connection
	if r.client != nil {
		r.client.Close()
	}
	if r.pf != nil {
		r.pf.Close()
	}

	// Re-discover the Redis pod (it may have been rescheduled)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pod, err := r.k8s.FindRedisPod(ctx)
	if err != nil {
		return fmt.Errorf("finding Redis pod: %w", err)
	}
	r.redisPod = pod

	// Start new port-forward
	pf, err := r.k8s.StartPortForward(pod, 6379)
	if err != nil {
		return fmt.Errorf("port-forward: %w", err)
	}

	// Create new Redis client
	addr := fmt.Sprintf("127.0.0.1:%d", pf.LocalPort)
	client := NewClient(addr)

	if err := client.Ping(ctx); err != nil {
		pf.Close()
		client.Close()
		return fmt.Errorf("ping after reconnect: %w", err)
	}

	// Preserve highwater mark from old client
	client.highwater = r.client.highwater

	r.pf = pf
	r.client = client

	return nil
}

// Close cleans up the port-forward and Redis client.
func (r *Reconnector) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client != nil {
		r.client.Close()
	}
	if r.pf != nil {
		r.pf.Close()
	}
}
