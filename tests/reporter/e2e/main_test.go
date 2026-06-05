// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	shared "github.com/LerianStudio/midaz/v4/tests/reporter/e2e/shared"
)

var (
	apiClient *shared.ManagerClient
	env       *shared.E2EEnv
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var code int

	defer func() {
		shared.Teardown(ctx, env)
		os.Exit(code)
	}()

	var err error

	env, err = shared.Setup(ctx)
	if err != nil {
		log.Fatalf("Setup failed: %v", err)
	}

	// Create API client from Manager base URL.
	apiClient = shared.NewManagerClient(env.ManagerBaseURL)

	// Wait for Manager to be healthy.
	if err := apiClient.WaitHealthy(ctx, 90*time.Second); err != nil {
		log.Fatalf("Manager health check failed: %v", err)
	}

	code = m.Run()
}
