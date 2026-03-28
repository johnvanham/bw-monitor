package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/johnvanham/bw-monitor/internal/k8s"
	"github.com/johnvanham/bw-monitor/internal/model"
	"github.com/johnvanham/bw-monitor/internal/redis"
)

func main() {
	namespace := flag.String("namespace", "bunkerweb", "Kubernetes namespace for BunkerWeb")
	maxEntries := flag.Int("max-entries", 1000, "Maximum number of initial reports to load")
	flag.Parse()

	ctx := context.Background()

	// Connect to Kubernetes
	k8sClient, err := k8s.NewClient(*namespace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to Kubernetes: %v\n", err)
		os.Exit(1)
	}

	// Find the Redis pod
	redisPod, err := k8sClient.FindRedisPod(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find Redis pod: %v\n", err)
		os.Exit(1)
	}

	// Verify Redis connectivity
	pong, err := k8sClient.ExecRedis(ctx, redisPod, "PING")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to Redis: %v\n", err)
		os.Exit(1)
	}
	if pong[:4] != "PONG" {
		fmt.Fprintf(os.Stderr, "Unexpected Redis response: %s\n", pong)
		os.Exit(1)
	}

	// Create Redis client
	redisClient := redis.NewClient(k8sClient, redisPod)

	// Create and run TUI
	m := model.New(redisClient, *maxEntries)
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
