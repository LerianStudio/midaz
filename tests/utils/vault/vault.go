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
	vaultapi "github.com/hashicorp/vault/api"
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
func CreateClient(t *testing.T, container *ContainerResult) *vault.Client {
	t.Helper()

	cfg := vault.Config{
		Addr:       container.Address,
		Token:      container.Token,
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

// EnableTransitMount enables a Transit secrets engine at mountPath in the test
// container's Vault. Callers enable a single path per call (e.g. "transit-mt" or
// "transit-st"); mounting the same path twice is an error in Vault.
// It builds its own raw api.Client from the container address and root token so
// it does not depend on the vault.Client unexported internals. No cleanup is
// registered: the container teardown from SetupContainer disposes the Vault.
func EnableTransitMount(t *testing.T, container *ContainerResult, mountPath string) {
	t.Helper()

	client, err := vaultapi.NewClient(&vaultapi.Config{Address: container.Address})
	if err != nil {
		t.Fatalf("failed to create vault api client: %v", err)
	}

	client.SetToken(container.Token)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Sys().MountWithContext(ctx, mountPath, &vaultapi.MountInput{Type: "transit"}); err != nil {
		t.Fatalf("failed to enable transit mount at %q: %v", mountPath, err)
	}
}
