package model

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const excludesFileName = ".bw-monitor-excludes"

// ExcludeList manages a persistent list of excluded IP addresses.
type ExcludeList struct {
	ips  map[string]bool
	path string
}

// NewExcludeList creates an ExcludeList and loads from disk.
func NewExcludeList() *ExcludeList {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	el := &ExcludeList{
		ips:  make(map[string]bool),
		path: filepath.Join(home, excludesFileName),
	}
	el.load()
	return el
}

// Contains returns true if the IP is excluded.
func (el *ExcludeList) Contains(ip string) bool {
	return el.ips[ip]
}

// Add adds an IP to the exclude list and saves to disk.
func (el *ExcludeList) Add(ip string) {
	el.ips[ip] = true
	el.save()
}

// Remove removes an IP from the exclude list and saves to disk.
func (el *ExcludeList) Remove(ip string) {
	delete(el.ips, ip)
	el.save()
}

// List returns all excluded IPs in a stable sorted order.
func (el *ExcludeList) List() []string {
	var ips []string
	for ip := range el.ips {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	return ips
}

// Count returns the number of excluded IPs.
func (el *ExcludeList) Count() int {
	return len(el.ips)
}

func (el *ExcludeList) load() {
	f, err := os.Open(el.path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		ip := strings.TrimSpace(scanner.Text())
		if ip != "" && !strings.HasPrefix(ip, "#") {
			el.ips[ip] = true
		}
	}
}

func (el *ExcludeList) save() {
	f, err := os.Create(el.path)
	if err != nil {
		return
	}
	defer f.Close()

	for ip := range el.ips {
		f.WriteString(ip + "\n")
	}
}
