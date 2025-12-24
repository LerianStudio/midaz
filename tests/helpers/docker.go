package helpers

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	dockerCommandTimeout = 120 * time.Second
)

// ComposeUpBackend brings infra + onboarding + transaction online using root Makefile.
func ComposeUpBackend() error {
	ctx, cancel := context.WithTimeout(context.Background(), dockerCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "make", "up-backend")

	out, err := cmd.CombinedOutput()
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return fmt.Errorf("make up-backend failed: %w\n%s", err, string(out))
	}

	return nil
}

// ComposeDownBackend stops services started with ComposeUpBackend.
func ComposeDownBackend() error {
	ctx, cancel := context.WithTimeout(context.Background(), dockerCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "make", "down-backend")

	out, err := cmd.CombinedOutput()
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return fmt.Errorf("make down-backend failed: %w\n%s", err, string(out))
	}

	return nil
}

// DockerAction performs a docker container action like stop/start/restart/pause/unpause.
func DockerAction(action, container string, extraArgs ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), dockerCommandTimeout)
	defer cancel()

	args := append([]string{action, container}, extraArgs...)
	cmd := exec.CommandContext(ctx, "docker", args...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return fmt.Errorf("docker %s %s failed: %w\n%s", action, strings.Join(args, " "), err, string(out))
	}

	return nil
}

// RestartWithWait restarts a container and waits a bit for it to come back.
func RestartWithWait(container string, wait time.Duration) error {
	if err := DockerAction("restart", container); err != nil {
		return err
	}

	time.Sleep(wait)

	return nil
}

// DockerNetwork connects or disconnects a container to/from a Docker network.
// action should be "connect" or "disconnect".
func DockerNetwork(action, network, container string) error {
	ctx, cancel := context.WithTimeout(context.Background(), dockerCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "network", action, network, container)

	out, err := cmd.CombinedOutput()
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return fmt.Errorf("docker network %s %s %s failed: %w\n%s", action, network, container, err, string(out))
	}

	return nil
}

// DockerExec runs a command inside a running container and returns its combined output.
func DockerExec(container string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dockerCommandTimeout)
	defer cancel()

	full := append([]string{"exec", container}, args...)
	cmd := exec.CommandContext(ctx, "docker", full...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker exec %s %v failed: %w\n%s", container, args, err, string(out))
	}

	return string(out), nil
}

// DockerLogsSince returns docker logs for a container since the provided RFC3339 timestamp.
// If tail > 0, limits the number of lines returned.
func DockerLogsSince(container, since string, tail int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dockerCommandTimeout)
	defer cancel()

	args := []string{"logs", "--since", since}
	if tail > 0 {
		args = append(args, "--tail", strconv.Itoa(tail))
	}

	args = append(args, container)
	cmd := exec.CommandContext(ctx, "docker", args...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker logs failed: %w\n%s", err, string(out))
	}

	return string(out), nil
}
