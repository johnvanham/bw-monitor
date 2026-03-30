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
	maxEntries := flag.Int("max-entries", 0, "Maximum number of initial reports to load (0 = all)")
	flag.Parse()

	ctx := context.Background()

	// Connect to Kubernetes
	fmt.Fprintf(os.Stderr, "Connecting to Kubernetes...\n")
	k8sClient, err := k8s.NewClient(*namespace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to Kubernetes: %v\n", err)
		os.Exit(1)
	}

	// Find the Redis pod
	fmt.Fprintf(os.Stderr, "Finding Redis pod...\n")
	redisPod, err := k8sClient.FindRedisPod(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find Redis pod: %v\n", err)
		os.Exit(1)
	}

	// Start port-forward to Redis
	fmt.Fprintf(os.Stderr, "Starting port-forward to %s...\n", redisPod)
	pf, err := k8sClient.StartPortForward(redisPod, 6379)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start port-forward: %v\n", err)
		os.Exit(1)
	}
	defer pf.Close()

	// Connect to Redis via port-forward
	addr := fmt.Sprintf("127.0.0.1:%d", pf.LocalPort)
	redisClient := redis.NewClient(addr)
	defer redisClient.Close()

	if err := redisClient.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to ping Redis: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Connected to Redis via port-forward (localhost:%d)\n", pf.LocalPort)

	// Create and run TUI
	m := model.New(redisClient, *maxEntries)
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
