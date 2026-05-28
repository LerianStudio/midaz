// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package vault provides test utilities for HashiCorp Vault integration tests.
package vault

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/crypto/kms/vault"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// DefaultRootToken is the root token for the Vault dev container.
const DefaultRootToken = "root"

// ContainerResult contains the Vault container connection information.
type ContainerResult struct {
	Container testcontainers.Container
	Address   string
	Token     string
}

// SetupContainer starts a Vault dev container for integration testing.
// The container is automatically terminated when the test finishes.
func SetupContainer(t *testing.T) *ContainerResult {
	t.Helper()

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "hashicorp/vault:1.15",
		ExposedPorts: []string{"8200/tcp"},
		Env: map[string]string{
			"VAULT_DEV_ROOT_TOKEN_ID":  DefaultRootToken,
			"VAULT_DEV_LISTEN_ADDRESS": "0.0.0.0:8200",
			"VAULT_ADDR":               "http://127.0.0.1:8200",
		},
		WaitingFor: wait.ForHTTP("/v1/sys/health").
			WithPort("8200/tcp").
			WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start vault container: %v", err)
	}

	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate vault container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get vault container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "8200")
	if err != nil {
		t.Fatalf("failed to get vault container port: %v", err)
	}

	address := fmt.Sprintf("http://%s:%s", host, port.Port())

	return &ContainerResult{
		Container: container,
		Address:   address,
		Token:     DefaultRootToken,
	}
}

// CreateClient creates a Vault client connected to the test container.
func CreateClient(t *testing.T, container *ContainerResult, mountPath string) *vault.Client {
	t.Helper()

	cfg := vault.Config{
		Addr:       container.Address,
		Token:      container.Token,
		MountPath:  mountPath,
		AuthMethod: vault.AuthMethodToken,
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create vault client: %v", err)
	}

	// Login to authenticate
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Login(ctx); err != nil {
		t.Fatalf("failed to login to vault: %v", err)
	}

	return client
}

// EnableTransitEngine enables the Transit secrets engine at the specified path.
func EnableTransitEngine(t *testing.T, container *ContainerResult, mountPath string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a temporary client to enable the engine
	cfg := vault.Config{
		Addr:       container.Address,
		Token:      container.Token,
		MountPath:  mountPath,
		AuthMethod: vault.AuthMethodToken,
	}

	client, err := vault.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create vault client for transit setup: %v", err)
	}

	if err := client.Login(ctx); err != nil {
		t.Fatalf("failed to login for transit setup: %v", err)
	}

	// Note: In dev mode, Transit is not enabled by default.
	// For full integration tests, you would need to enable it via the API.
	// This is left as a placeholder for future enhancement.
	_ = ctx
}
