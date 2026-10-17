//go:build bench

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The owned promotion topology: a Compose project of its own beside midaz-bench-engine, on fixed loopback ports.
const (
	failoverProject  = "midaz-bench-failover"
	failoverNetwork  = failoverProject + "_default"
	ownedPrimaryPort = "25432"
	ownedClientPort  = "25434"
	ownedStandbyPort = "25435"
	ownedAPIPort     = "28474"
)

// ownedService is one owned container: its service, fixed name, published container ports and memory cap.
type ownedService struct {
	service, container string
	ports              map[string]string
	memory             int64
}

var ownedServices = [...]ownedService{
	{"bench-pg-primary", "bench-failover-pg-primary", map[string]string{"5432/tcp": ownedPrimaryPort}, 8 << 30},
	{"bench-toxiproxy", "bench-failover-toxiproxy", map[string]string{"5434/tcp": ownedClientPort, "8474/tcp": ownedAPIPort}, 256 << 20},
	{"bench-pg-standby", "bench-failover-pg-standby", map[string]string{"5432/tcp": ownedStandbyPort}, 8 << 30},
}

// docker runs the CLI on the default context, so no config file picks another daemon; the override's port variables are
// appended last and win. WaitDelay bounds the wait on a compose child that outlives a cancelled CLI: the daemon may
// still finish that work.
func docker(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"--context", "default"}, args...)...)
	cmd.Env = append(os.Environ(), "BENCH_PG_PRIMARY_PORT="+ownedPrimaryPort, "BENCH_PG_CLIENT_PROXY_PORT="+ownedClientPort,
		"BENCH_PG_STANDBY_PORT="+ownedStandbyPort, "BENCH_TOXIPROXY_PORT="+ownedAPIPort)
	cmd.WaitDelay = 5 * time.Second

	var stdout, stderr bytes.Buffer

	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker %s: %w: %s %s", strings.Join(args, " "), err, bytes.TrimSpace(stdout.Bytes()), bytes.TrimSpace(stderr.Bytes()))
	}

	return stdout.Bytes(), nil
}

// compose runs one command on the owned project: the base file, then the override, from dir.
func compose(ctx context.Context, dir string, args ...string) error {
	_, err := docker(ctx, append([]string{
		"compose", "-p", failoverProject, "--project-directory", dir,
		"-f", filepath.Join(dir, "docker-compose.yml"), "-f", filepath.Join(dir, "docker-compose.failover.yml"),
	}, args...)...)

	return err
}

// ownedResources lists, as sorted "kind id name project" rows, every container, volume and network that carries the owned
// project label or an owned name, so a foreign resource squatting an owned name shows too.
func ownedResources(ctx context.Context) ([]string, error) {
	var rows []string

	for _, q := range [...][]string{
		{"container", "ps", "-a", "--no-trunc", "--format", `{{.ID}} {{.Names}} {{.Label "com.docker.compose.project"}}`},
		{"volume", "volume", "ls", "--format", `{{.Name}} {{.Name}} {{.Label "com.docker.compose.project"}}`},
		{"network", "network", "ls", "--no-trunc", "--format", `{{.ID}} {{.Name}} {{.Label "com.docker.compose.project"}}`},
	} {
		out, err := docker(ctx, q[1:]...)
		if err != nil {
			return nil, err
		}

		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 && (strings.HasPrefix(f[1], "bench-failover-") || strings.HasPrefix(f[1], failoverProject+"_") ||
				len(f) == 3 && f[2] == failoverProject) {
				rows = append(rows, q[0]+" "+line)
			}
		}
	}

	slices.Sort(rows)

	return rows, nil
}

// dockerContainer holds the inspect fields the binding checks and records.
type dockerContainer struct {
	ID, Name, Image string
	RestartCount    int
	Config          struct{ Labels map[string]string }
	State           struct {
		Status    string
		OOMKilled bool
		Pid       int
		ExitCode  int
		StartedAt string
		Health    *struct{ Status string }
	}
	HostConfig struct {
		Memory, MemorySwap int64
		RestartPolicy      struct{ Name string }
	}
	NetworkSettings struct {
		Ports    map[string][]struct{ HostIP, HostPort string }
		Networks map[string]struct{ NetworkID, IPAddress string }
	}
}

func inspectContainer(ctx context.Context, ref string) (dockerContainer, error) {
	out, err := docker(ctx, "inspect", "--type", "container", ref)
	if err != nil {
		return dockerContainer{}, err
	}

	var got []dockerContainer
	if err := json.Unmarshal(out, &got); err != nil || len(got) != 1 {
		return dockerContainer{}, fmt.Errorf("inspect %s: %d containers, decode error %v", ref, len(got), err)
	}

	return got[0], nil
}

// bound checks c is svc's container in the owned project: running, healthy when required, never restarted or OOM-killed,
// without restart policy, capped at svc.memory without swap, published only on its loopback ports and attached only to
// the owned network. The standby's healthcheck encodes its standby role, which a cut or a promotion ends.
func (c dockerContainer) bound(svc ownedService, healthy bool) error {
	var errs []error

	check := func(ok bool, format string, args ...any) {
		if !ok {
			errs = append(errs, fmt.Errorf(format, args...))
		}
	}

	check(c.Name == "/"+svc.container && c.Config.Labels["com.docker.compose.project"] == failoverProject &&
		c.Config.Labels["com.docker.compose.service"] == svc.service, "name %q labels %v", c.Name, c.Config.Labels)
	check(c.State.Status == "running" && !c.State.OOMKilled && c.RestartCount == 0 &&
		(!healthy || c.State.Health != nil && c.State.Health.Status == "healthy"), "state %+v restarts %d", c.State, c.RestartCount)
	check(slices.Contains([]string{"", "no"}, c.HostConfig.RestartPolicy.Name) && c.HostConfig.Memory == svc.memory &&
		c.HostConfig.MemorySwap == svc.memory, "host config %+v, want memory and swap %d", c.HostConfig, svc.memory)
	check(len(c.NetworkSettings.Ports) == len(svc.ports), "ports %+v", c.NetworkSettings.Ports)

	for port, host := range svc.ports {
		b := c.NetworkSettings.Ports[port]
		check(len(b) == 1 && b[0].HostIP == "127.0.0.1" && b[0].HostPort == host, "%s bindings %+v, want 127.0.0.1:%s", port, b, host)
	}

	n, ok := c.NetworkSettings.Networks[failoverNetwork]
	check(len(c.NetworkSettings.Networks) == 1 && ok && n.NetworkID != "" && n.IPAddress != "", "networks %+v", c.NetworkSettings.Networks)

	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("%s %s: %w", svc.service, c.ID, err)
	}

	return nil
}

// scopeMemory is a container's cgroup reading: limits, swap in use, peak and the sum of oom, oom_kill and oom_group_kill.
type scopeMemory struct{ max, swapMax, swapCurrent, peak, ooms int64 }

// readScope fails closed: a missing file, an unparsable value such as "max" or a missing OOM counter is an error.
func readScope(id string) (scopeMemory, error) {
	dir := "/sys/fs/cgroup/system.slice/docker-" + id + ".scope/"

	var s scopeMemory

	for name, dst := range map[string]*int64{"memory.max": &s.max, "memory.swap.max": &s.swapMax, "memory.swap.current": &s.swapCurrent, "memory.peak": &s.peak} {
		b, err := os.ReadFile(dir + name)
		if err != nil {
			return s, err
		}

		if *dst, err = strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err != nil {
			return s, fmt.Errorf("%s%s: %w", dir, name, err)
		}
	}

	events, err := os.ReadFile(dir + "memory.events")
	if err != nil {
		return s, err
	}

	seen := 0

	for _, line := range strings.Split(strings.TrimSpace(string(events)), "\n") {
		key, value, _ := strings.Cut(line, " ")
		if key == "oom" || key == "oom_kill" || key == "oom_group_kill" {
			v, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return s, fmt.Errorf("%smemory.events %q: %w", dir, line, err)
			}

			s.ooms += v
			seen++
		}
	}

	if seen != 3 {
		return s, fmt.Errorf("%smemory.events lacks an OOM counter: %q", dir, events)
	}

	return s, nil
}

// capped checks the cgroup enforces svc's cap without swap and saw no OOM.
func (s scopeMemory) capped(svc ownedService) error {
	if s.max != svc.memory || s.swapMax != 0 || s.swapCurrent != 0 || s.ooms != 0 {
		return fmt.Errorf("%s cgroup %+v, want max %d, no swap and no OOM", svc.service, s, svc.memory)
	}

	return nil
}
