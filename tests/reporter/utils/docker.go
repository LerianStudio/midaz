// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"fmt"
	"os/exec"
	"time"
)

// RestartWithWait restarts a container by name and waits a small delay.
func RestartWithWait(container string, delay time.Duration) error {
	if container == "" {
		return fmt.Errorf("empty container name")
	}

	cmd := exec.Command("docker", "restart", container)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker restart %s: %v, out=%s", container, err, string(out))
	}

	if delay > 0 {
		time.Sleep(delay)
	}

	return nil
}

// StopContainer stops a container by name.
func StopContainer(container string) error {
	if container == "" {
		return fmt.Errorf("empty container name")
	}

	cmd := exec.Command("docker", "stop", container)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker stop %s: %v, out=%s", container, err, string(out))
	}

	return nil
}

// StartContainer starts a stopped container by name.
func StartContainer(container string) error {
	if container == "" {
		return fmt.Errorf("empty container name")
	}

	cmd := exec.Command("docker", "start", container)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker start %s: %v, out=%s", container, err, string(out))
	}

	// Give container time to initialize
	time.Sleep(5 * time.Second)

	return nil
}

// StartWithWait starts a container and waits for a specified delay.
func StartWithWait(container string, delay time.Duration) error {
	if container == "" {
		return fmt.Errorf("empty container name")
	}

	cmd := exec.Command("docker", "start", container)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker start %s: %v, out=%s", container, err, string(out))
	}

	if delay > 0 {
		time.Sleep(delay)
	}

	return nil
}
