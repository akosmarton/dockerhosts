package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
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
	var log *zap.Logger

	ctx := context.Background()

	debug := os.Getenv("DEBUG")

	domain = os.Getenv("DOMAIN")
	if domain == "" {
		domain = "docker"
	}

	filename := os.Getenv("HOSTS_FILE")
	if filename == "" {
		filename = "/etc/hosts"
	}

	if debug == "" {
		if l, err := zap.NewProduction(); err != nil {
			panic(err)
		} else {
			log = l
		}
	} else {
		if l, err := zap.NewDevelopment(); err != nil {
			panic(err)
		} else {
			log = l
		}
	}

	log.Info("env", zap.String("debug", debug), zap.String("DOCKER_HOST", os.Getenv("DOCKER_HOST")), zap.String("DOMAIN", domain), zap.String("FILENAME", filename))

	client, err := client.NewEnvClient()
	if err != nil {
		log.Fatal(err.Error())
		return
	}

	hosts := hostsFile{
		Filename: filename,
	}

	filters := filters.NewArgs()
	filters.Add("type", "network")
	msgs, errs := client.Events(ctx, types.EventsOptions{Filters: filters})

	entries, err := getEntris(ctx, client)
	if err != nil {
		log.Fatal(err.Error())
		return
	}
	if err := hosts.update(entries); err != nil {
		log.Fatal(err.Error())
		return
	}

	for {
		select {
		case msg := <-msgs:
			if msg.Type == "network" && (msg.Action == "connect" || msg.Action == "disconnect") {
				entries, err := getEntris(ctx, client)
				if err != nil {
					log.Fatal(err.Error())
					return
				}
				if err := hosts.update(entries); err != nil {
					log.Fatal(err.Error())
					return
				}
			}
		case err := <-errs:
			log.Fatal(err.Error())
			return
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
		network, err := client.NetworkInspect(ctx, n.ID)
		if err != nil {
			return nil, err
		}
		for _, container := range network.Containers {
			ip := container.IPv4Address[:strings.Index(container.IPv4Address, "/")]
			if ip == "" {
				continue
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
