// Package helpers provides reusable utilities and setup functions to streamline
// integration and end-to-end tests.
// This file contains Docker container management utilities for test infrastructure.
package helpers

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ComposeUpBackend starts the backend services (infra, onboarding, transaction)
// using the root Makefile.
func ComposeUpBackend() error {
	cmd := exec.Command("make", "up-backend")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("make up-backend failed: %v\n%s", err, string(out))
	}

	return nil
}

// ComposeDownBackend stops the backend services that were started with ComposeUpBackend.
func ComposeDownBackend() error {
	cmd := exec.Command("make", "down-backend")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("make down-backend failed: %v\n%s", err, string(out))
	}

	return nil
}

// DockerAction performs a Docker container action, such as stop, start, restart,
// pause, or unpause.
func DockerAction(action, container string, extraArgs ...string) error {
	args := append([]string{action, container}, extraArgs...)
	cmd := exec.Command("docker", args...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s %s failed: %v\n%s", action, strings.Join(args, " "), err, string(out))
	}

	return nil
}

// RestartWithWait restarts a Docker container and then waits for a specified
// duration to allow it to initialize.
func RestartWithWait(container string, wait time.Duration) error {
	if err := DockerAction("restart", container); err != nil {
		return err
	}

	time.Sleep(wait)

	return nil
}

// DockerNetwork connects or disconnects a container from a Docker network.
// The `action` parameter should be either "connect" or "disconnect".
func DockerNetwork(action, network, container string) error {
	cmd := exec.Command("docker", "network", action, network, container)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker network %s %s %s failed: %v\n%s", action, network, container, err, string(out))
	}

	return nil
}

// DockerExec runs a command inside a running container and returns its combined
// standard output and standard error.
func DockerExec(container string, args ...string) (string, error) {
	full := append([]string{"exec", container}, args...)
	cmd := exec.Command("docker", full...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker exec %s %v failed: %v\n%s", container, args, err, string(out))
	}

	return string(out), nil
}

// DockerLogsSince returns the logs for a container since a specified RFC3339
// timestamp, optionally limited by the `tail` parameter.
func DockerLogsSince(container, since string, tail int) (string, error) {
	args := []string{"logs", "--since", since}
	if tail > 0 {
		args = append(args, "--tail", strconv.Itoa(tail))
	}

	args = append(args, container)
	cmd := exec.Command("docker", args...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker logs failed: %v\n%s", err, string(out))
	}

	return string(out), nil
}
