package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

type hostEntry struct {
	Domain    string
	Hostname  string
	IPAddress string
}

type hostsFile struct {
	Filename string
}

const COMMENT = "# DOCKERHOSTS"

var domain string

func main() {
	var err error

	ctx := context.Background()
	docker_host := os.Getenv("DOCKER_HOST")
	domain = os.Getenv("DOMAIN")
	if domain == "" {
		domain = "docker"
	}
	filename := os.Getenv("HOSTS_FILE")
	if filename == "" {
		filename = "/etc/hosts"
	}

	slog.Info("env", "DOCKER_HOST", docker_host, "DOMAIN", domain, "HOSTS_FILE", filename)
	client, err := client.NewClientWithOpts(client.WithAPIVersionNegotiation(), client.WithHost(docker_host))
	if err != nil {
		panic(err)
	}

	hosts := hostsFile{
		Filename: filename,
	}

	filters := filters.NewArgs()
	filters.Add("type", "network")
	msgs, errs := client.Events(ctx, types.EventsOptions{Filters: filters})

	entries, err := getEntris(ctx, client)
	if err != nil {
		panic(err)
	}
	if err := hosts.update(entries); err != nil {
		panic(err)
	}

	for {
		select {
		case msg := <-msgs:
			if msg.Type == "network" && (msg.Action == "connect" || msg.Action == "disconnect") {
				entries, err := getEntris(ctx, client)
				if err != nil {
					panic(err)
				}
				if err := hosts.update(entries); err != nil {
					panic(err)
				}
			}
		case err := <-errs:
			panic(err)
		default:
			time.Sleep(1000 * time.Millisecond)
		}
	}
}

func (h *hostsFile) update(entries []hostEntry) error {
	f, err := os.OpenFile(h.Filename, os.O_RDWR, 0755)
	if err != nil {
		return err
	}
	defer f.Close()

	lines := make([]string, 0)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, COMMENT) {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf("%s %s.%s %s", entry.IPAddress, entry.Hostname, entry.Domain, COMMENT))
		slog.Info("update", "ip", entry.IPAddress, "hostname", entry.Hostname, "domain", entry.Domain)
	}

	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	if err := f.Truncate(0); err != nil {
		return err
	}

	w := bufio.NewWriter(f)

	for _, line := range lines {
		if _, err := w.WriteString(line); err != nil {
			return err
		}
		if _, err := w.WriteString("\n"); err != nil {
			return err
		}
	}

	if err := w.Flush(); err != nil {
		return err
	}

	return f.Sync()
}

func getEntris(ctx context.Context, client *client.Client) ([]hostEntry, error) {
	entries := make([]hostEntry, 0)
	networks, err := client.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return nil, err
	}
	for _, n := range networks {
		network, err := client.NetworkInspect(ctx, n.ID, types.NetworkInspectOptions{Verbose: true})
		if err != nil {
			return nil, err
		}
		for _, container := range network.Containers {
			ip := container.IPv4Address
			if ip == "" {
				continue
			}
			if i := strings.Index(ip, "/"); i != -1 {
				ip = ip[:i]
			}
			entries = append(entries, hostEntry{
				Domain:    domain,
				Hostname:  container.Name,
				IPAddress: ip,
			})
		}
	}
	return entries, nil
}
