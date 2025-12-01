// Package helpers provides test utilities for Midaz integration and E2E tests.
//
// # Purpose
//
// This package provides Docker container management utilities for tests that need
// to control service lifecycle, simulate failures, and inspect container state.
//
// # Docker Operations
//
// Container lifecycle management:
//   - ComposeUpBackend: Start full backend stack via Makefile
//   - ComposeDownBackend: Stop backend stack
//   - DockerAction: Generic container actions (stop/start/restart/pause/unpause)
//   - RestartWithWait: Restart with configurable recovery delay
//
// Network operations:
//   - DockerNetwork: Connect/disconnect containers from networks
//
// Container inspection:
//   - DockerExec: Execute commands inside running containers
//   - DockerLogsSince: Retrieve logs since a timestamp
//
// # Usage
//
//	// Start backend for tests
//	if err := helpers.ComposeUpBackend(); err != nil {
//	    t.Fatal(err)
//	}
//	defer helpers.ComposeDownBackend()
//
//	// Simulate database failure
//	if err := helpers.DockerAction("stop", "midaz-postgres"); err != nil {
//	    t.Fatal(err)
//	}
//
//	// Check container logs after failure
//	logs, err := helpers.DockerLogsSince("midaz-onboarding", startTime, 100)
//
// # Thread Safety
//
// These functions execute shell commands and are not thread-safe.
// Tests using these helpers should not run in parallel.
package helpers

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ComposeUpBackend brings infra + onboarding + transaction online using root Makefile.
//
// This function executes `make up-backend` which starts all backend services
// including databases, message queues, and application containers.
//
// # Process
//
//	Step 1: Execute `make up-backend` command
//	Step 2: Wait for command completion
//	Step 3: Return error if command fails
//
// # Returns
//
//   - nil: Backend started successfully
//   - error: Command failed with output details
func ComposeUpBackend() error {
	cmd := exec.Command("make", "up-backend")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("make up-backend failed: %v\n%s", err, string(out))
	}

	return nil
}

// ComposeDownBackend stops services started with ComposeUpBackend.
//
// This function executes `make down-backend` to cleanly stop all backend
// containers and release resources.
//
// # Returns
//
//   - nil: Backend stopped successfully
//   - error: Command failed with output details
func ComposeDownBackend() error {
	cmd := exec.Command("make", "down-backend")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("make down-backend failed: %v\n%s", err, string(out))
	}

	return nil
}

// DockerAction performs a docker container action like stop/start/restart/pause/unpause.
//
// # Parameters
//
//   - action: Docker command (stop, start, restart, pause, unpause, kill)
//   - container: Container name or ID
//   - extraArgs: Additional docker command arguments
//
// # Returns
//
//   - nil: Action completed successfully
//   - error: Docker command failed with output details
//
// # Example
//
//	// Stop a container
//	err := DockerAction("stop", "midaz-postgres")
//
//	// Kill with signal
//	err := DockerAction("kill", "midaz-onboarding", "-s", "SIGTERM")
func DockerAction(action, container string, extraArgs ...string) error {
	args := append([]string{action, container}, extraArgs...)
	cmd := exec.Command("docker", args...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s %s failed: %v\n%s", action, strings.Join(args, " "), err, string(out))
	}

	return nil
}

// RestartWithWait restarts a container and waits a bit for it to come back.
//
// This is useful for tests that need to restart a service and wait for it
// to be ready before continuing.
//
// # Parameters
//
//   - container: Container name or ID to restart
//   - wait: Duration to sleep after restart command completes
//
// # Returns
//
//   - nil: Restart completed and wait finished
//   - error: Docker restart command failed
func RestartWithWait(container string, wait time.Duration) error {
	if err := DockerAction("restart", container); err != nil {
		return err
	}

	time.Sleep(wait)

	return nil
}

// DockerNetwork connects or disconnects a container to/from a Docker network.
//
// This is useful for testing network partition scenarios.
//
// # Parameters
//
//   - action: "connect" or "disconnect"
//   - network: Network name
//   - container: Container name or ID
//
// # Returns
//
//   - nil: Network operation completed
//   - error: Docker network command failed
//
// # Example
//
//	// Simulate network partition
//	err := DockerNetwork("disconnect", "midaz_default", "midaz-postgres")
//	// ... test partition handling ...
//	err = DockerNetwork("connect", "midaz_default", "midaz-postgres")
func DockerNetwork(action, network, container string) error {
	cmd := exec.Command("docker", "network", action, network, container)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker network %s %s %s failed: %v\n%s", action, network, container, err, string(out))
	}

	return nil
}

// DockerExec runs a command inside a running container and returns its combined output.
//
// # Parameters
//
//   - container: Container name or ID
//   - args: Command and arguments to execute
//
// # Returns
//
//   - string: Combined stdout and stderr output
//   - error: nil on success, error with output on failure
//
// # Example
//
//	// Check database connectivity
//	out, err := DockerExec("midaz-postgres", "pg_isready", "-U", "postgres")
func DockerExec(container string, args ...string) (string, error) {
	full := append([]string{"exec", container}, args...)
	cmd := exec.Command("docker", full...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker exec %s %v failed: %v\n%s", container, args, err, string(out))
	}

	return string(out), nil
}

// DockerLogsSince returns docker logs for a container since the provided RFC3339 timestamp.
//
// # Parameters
//
//   - container: Container name or ID
//   - since: RFC3339 timestamp (e.g., "2025-01-01T00:00:00Z")
//   - tail: Maximum lines to return (0 for unlimited)
//
// # Returns
//
//   - string: Container logs
//   - error: nil on success, error with partial output on failure
//
// # Example
//
//	startTime := time.Now().Format(time.RFC3339)
//	// ... perform operation ...
//	logs, err := DockerLogsSince("midaz-onboarding", startTime, 50)
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
