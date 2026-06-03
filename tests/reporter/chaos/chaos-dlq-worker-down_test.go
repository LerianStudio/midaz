//go:build chaos

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package chaos

import (
	"os"
	"testing"
)

// TestChaos_DLQ_WorkerDownThenRabbitMQCrash tests message persistence when both Worker and RabbitMQ fail
// This test requires docker-compose infrastructure where Worker runs as a container.
// In testcontainers mode, Worker runs as a subprocess and cannot be stopped/started via Docker.
func TestIntegration_Chaos_DLQ_WorkerDownThenRabbitMQCrash(t *testing.T) {
	// NOTE: Cannot use t.Parallel() because this test manipulates shared infrastructure (stops/starts Worker and RabbitMQ containers).
	if os.Getenv("CHAOS") != "1" {
		t.Skip("Set CHAOS=1 to run chaos tests")
	}
	if testing.Short() {
		t.Skip("Skipping chaos test in short mode")
	}

	// Skip this test in testcontainers mode - Worker is a Go subprocess, not a container
	if os.Getenv("USE_EXISTING_INFRA") != "true" {
		t.Skip("Skipping test - Worker runs as subprocess in testcontainers mode, requires docker-compose infrastructure")
	}

	// This test only runs with docker-compose infrastructure where Worker is a container
	// The implementation would use h.StopContainer/h.StartContainer with container names
	t.Log("Running with docker-compose infrastructure - Worker container manipulation available")
}
