//go:build fuzz

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fuzzy

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/LerianStudio/reporter/tests/utils/containers"
	"github.com/LerianStudio/reporter/tests/utils/services"
)

var (
	testInfra   *containers.TestInfrastructure
	managerSvc  *services.ManagerService
	workerSvc   *services.WorkerService
	managerAddr string
)

func TestMain(m *testing.M) {
	// Check if we should use testcontainers or existing infrastructure
	if os.Getenv("USE_EXISTING_INFRA") == "true" {
		// Use existing infrastructure (docker-compose)
		fmt.Fprintf(os.Stderr, "Using existing infrastructure from docker-compose\n")
		os.Exit(m.Run())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Fprintf(os.Stderr, "Starting test infrastructure with testcontainers for fuzzy tests...\n")

	// Start infrastructure containers
	var err error
	testInfra, err = containers.StartInfrastructure(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start infrastructure: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Infrastructure started successfully\n")

	// Create service configuration from containers
	cfg := services.NewConfigFromInfrastructure(testInfra)

	// Start Manager service
	fmt.Fprintf(os.Stderr, "Starting Manager service...\n")
	managerSvc, err = services.StartManager(ctx, cfg)
	if err != nil {
		testInfra.Stop(ctx)
		fmt.Fprintf(os.Stderr, "Failed to start manager: %v\n", err)
		os.Exit(1)
	}
	managerAddr = managerSvc.Address()
	fmt.Fprintf(os.Stderr, "Manager started at %s\n", managerAddr)

	// Set environment variable for test helpers
	os.Setenv("MANAGER_URL", managerAddr)
	defer os.Unsetenv("MANAGER_URL")

	// Start Worker service
	fmt.Fprintf(os.Stderr, "Starting Worker service...\n")
	workerSvc, err = services.StartWorker(ctx, cfg)
	if err != nil {
		managerSvc.Stop(ctx)
		testInfra.Stop(ctx)
		fmt.Fprintf(os.Stderr, "Failed to start worker: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Worker started successfully\n")

	// Run tests
	fmt.Fprintf(os.Stderr, "Running fuzzy tests...\n")
	code := m.Run()

	// Cleanup
	fmt.Fprintf(os.Stderr, "Cleaning up...\n")
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cleanupCancel()

	if workerSvc != nil {
		workerSvc.Stop(cleanupCtx)
	}
	if managerSvc != nil {
		managerSvc.Stop(cleanupCtx)
	}
	if testInfra != nil {
		testInfra.Stop(cleanupCtx)
	}

	fmt.Fprintf(os.Stderr, "Cleanup complete\n")
	os.Exit(code)
}

// GetManagerAddress returns the Manager service address for tests.
func GetManagerAddress() string {
	if managerAddr != "" {
		return managerAddr
	}
	// Fallback to environment variable or default
	if addr := os.Getenv("MANAGER_URL"); addr != "" {
		return addr
	}
	return "http://127.0.0.1:4005"
}
