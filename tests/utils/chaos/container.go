//go:build integration || chaos

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package chaos

import (
	"context"
	"time"

	docker "github.com/moby/moby/client"
)

// ContainerChaosConfig holds configuration for container chaos operations.
type ContainerChaosConfig struct {
	// StopTimeout is the timeout for graceful container stop.
	StopTimeout time.Duration
	// RestartTimeout is the timeout for container restart operations.
	RestartTimeout time.Duration
}

// DefaultContainerChaosConfig returns the default container chaos configuration.
func DefaultContainerChaosConfig() ContainerChaosConfig {
	return ContainerChaosConfig{
		StopTimeout:    10 * time.Second,
		RestartTimeout: 30 * time.Second,
	}
}

// PauseContainer pauses a running container.
// A paused container is frozen - all processes are suspended.
// This simulates a hung/unresponsive service.
func (o *Orchestrator) PauseContainer(ctx context.Context, containerID string) error {
	o.t.Helper()
	o.t.Logf("Chaos: pausing container %s", containerID)

	_, err := o.docker.ContainerPause(ctx, containerID, docker.ContainerPauseOptions{})

	return err
}

// UnpauseContainer resumes a paused container.
func (o *Orchestrator) UnpauseContainer(ctx context.Context, containerID string) error {
	o.t.Helper()
	o.t.Logf("Chaos: unpausing container %s", containerID)

	_, err := o.docker.ContainerUnpause(ctx, containerID, docker.ContainerUnpauseOptions{})

	return err
}

// StopContainer stops a running container gracefully.
// This simulates a graceful shutdown (SIGTERM then SIGKILL).
func (o *Orchestrator) StopContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	o.t.Helper()
	o.t.Logf("Chaos: stopping container %s (timeout: %v)", containerID, timeout)

	timeoutSeconds := int(timeout.Seconds())

	_, err := o.docker.ContainerStop(ctx, containerID, docker.ContainerStopOptions{
		Timeout: &timeoutSeconds,
	})

	return err
}

// StartContainer starts a stopped container.
func (o *Orchestrator) StartContainer(ctx context.Context, containerID string) error {
	o.t.Helper()
	o.t.Logf("Chaos: starting container %s", containerID)

	_, err := o.docker.ContainerStart(ctx, containerID, docker.ContainerStartOptions{})

	return err
}

// RestartContainer restarts a container.
// This simulates a service crash and recovery.
func (o *Orchestrator) RestartContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	o.t.Helper()
	o.t.Logf("Chaos: restarting container %s (timeout: %v)", containerID, timeout)

	timeoutSeconds := int(timeout.Seconds())

	_, err := o.docker.ContainerRestart(ctx, containerID, docker.ContainerRestartOptions{
		Timeout: &timeoutSeconds,
	})

	return err
}

// KillContainer sends a signal to a container.
// Default signal is SIGKILL if empty string is passed.
// This simulates an abrupt crash (no graceful shutdown).
func (o *Orchestrator) KillContainer(ctx context.Context, containerID string, signal string) error {
	o.t.Helper()

	if signal == "" {
		signal = "SIGKILL"
	}

	o.t.Logf("Chaos: killing container %s (signal: %s)", containerID, signal)

	_, err := o.docker.ContainerKill(ctx, containerID, docker.ContainerKillOptions{Signal: signal})

	return err
}

// IsContainerRunning checks if a container is in running state.
func (o *Orchestrator) IsContainerRunning(ctx context.Context, containerID string) (bool, error) {
	o.t.Helper()

	inspect, err := o.docker.ContainerInspect(ctx, containerID, docker.ContainerInspectOptions{})
	if err != nil {
		return false, err
	}
	if inspect.Container.State == nil {
		return false, nil
	}

	return inspect.Container.State.Running, nil
}

// IsContainerPaused checks if a container is in paused state.
func (o *Orchestrator) IsContainerPaused(ctx context.Context, containerID string) (bool, error) {
	o.t.Helper()

	inspect, err := o.docker.ContainerInspect(ctx, containerID, docker.ContainerInspectOptions{})
	if err != nil {
		return false, err
	}
	if inspect.Container.State == nil {
		return false, nil
	}

	return inspect.Container.State.Paused, nil
}

// WaitForContainerRunning waits for a container to be in running state.
func (o *Orchestrator) WaitForContainerRunning(ctx context.Context, containerID string, timeout time.Duration) error {
	o.t.Helper()

	return o.WaitForRecovery(ctx, func() error {
		running, err := o.IsContainerRunning(ctx, containerID)
		if err != nil {
			return err
		}

		if !running {
			return ErrContainerNotRunning
		}

		return nil
	}, timeout)
}
