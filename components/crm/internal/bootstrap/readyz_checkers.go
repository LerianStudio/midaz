// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"time"

	libMongo "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	libLog "github.com/LerianStudio/lib-observability/log"
)

// MongoChecker probes a MongoDB connection using ping command.
type MongoChecker struct {
	name       string
	client     *libMongo.Client
	uri        string
	tlsEnabled bool
}

// NewMongoChecker creates a new MongoDB health checker.
func NewMongoChecker(name string, client *libMongo.Client, uri string) *MongoChecker {
	tlsEnabled, _ := detectMongoTLS(uri)

	return &MongoChecker{
		name:       name,
		client:     client,
		uri:        uri,
		tlsEnabled: tlsEnabled,
	}
}

// NewMongoCheckerWithLogger creates a new MongoDB health checker that logs TLS detection errors.
// If logger is nil, errors are silently ignored (same behavior as NewMongoChecker).
func NewMongoCheckerWithLogger(name string, client *libMongo.Client, uri string, logger libLog.Logger) *MongoChecker {
	tlsEnabled, err := detectMongoTLS(uri)
	if err != nil && logger != nil {
		logger.Log(context.Background(), libLog.LevelDebug,
			"Failed to detect MongoDB TLS configuration",
			libLog.String("checker", name),
			libLog.Err(err))
	}

	return &MongoChecker{
		name:       name,
		client:     client,
		uri:        uri,
		tlsEnabled: tlsEnabled,
	}
}

// Name returns the checker identifier.
func (c *MongoChecker) Name() string {
	return c.name
}

// TLSEnabled returns whether TLS is enabled for this connection.
func (c *MongoChecker) TLSEnabled() bool {
	return c.tlsEnabled
}

// Check probes the MongoDB connection.
func (c *MongoChecker) Check(ctx context.Context) DependencyCheck {
	if c.client == nil {
		return DependencyCheck{
			Status: StatusSkipped,
			Reason: "MongoDB client not configured",
		}
	}

	start := time.Now()

	err := c.client.Ping(ctx)
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: &latencyMs,
			Error:     fmt.Sprintf("ping failed: %v", err),
		}
	}

	return DependencyCheck{
		Status:    StatusUp,
		LatencyMs: &latencyMs,
	}
}

// NAChecker is a placeholder checker that always returns n/a status.
// Used in multi-tenant mode for dependencies that are tenant-scoped.
type NAChecker struct {
	name       string
	reason     string
	tlsEnabled bool
}

// NewNAChecker creates a checker that always returns n/a status.
func NewNAChecker(name, reason string, tlsEnabled bool) *NAChecker {
	return &NAChecker{
		name:       name,
		reason:     reason,
		tlsEnabled: tlsEnabled,
	}
}

// Name returns the checker identifier.
func (c *NAChecker) Name() string {
	return c.name
}

// TLSEnabled returns whether TLS is enabled for this dependency.
func (c *NAChecker) TLSEnabled() bool {
	return c.tlsEnabled
}

// Check always returns n/a status.
func (c *NAChecker) Check(_ context.Context) DependencyCheck {
	return DependencyCheck{
		Status: StatusNA,
		Reason: c.reason,
	}
}

// VaultHealthChecker is the interface for checking Vault health status.
// This allows testing without a real vault.Client.
type VaultHealthChecker interface {
	// HealthCheck verifies Vault server availability via sys/health endpoint.
	// Returns nil if Vault is healthy, error otherwise.
	HealthCheck(ctx context.Context) error
}

// VaultChecker probes Vault availability using the sys/health endpoint.
// This is consistent with other checkers (Mongo, Postgres, Redis) that make network calls.
type VaultChecker struct {
	name       string
	client     VaultHealthChecker
	addr       string
	tlsEnabled bool
}

// NewVaultChecker creates a new Vault health checker using a real vault.Client.
// The addr parameter is used for TLS detection.
func NewVaultChecker(name string, client VaultHealthChecker, addr string) *VaultChecker {
	return &VaultChecker{
		name:       name,
		client:     client,
		addr:       addr,
		tlsEnabled: detectVaultTLS(addr),
	}
}

// NewVaultCheckerWithClient creates a new Vault health checker with a custom health checker.
// This is useful for testing.
func NewVaultCheckerWithClient(name string, client VaultHealthChecker, addr string) *VaultChecker {
	return NewVaultChecker(name, client, addr)
}

// Name returns the checker identifier.
func (c *VaultChecker) Name() string {
	return c.name
}

// TLSEnabled returns whether TLS is enabled for this connection.
func (c *VaultChecker) TLSEnabled() bool {
	return c.tlsEnabled
}

// Check probes Vault availability via the sys/health endpoint.
// This is consistent with other checkers (Mongo, Postgres, Redis) that make network calls.
func (c *VaultChecker) Check(ctx context.Context) DependencyCheck {
	if c.client == nil {
		return DependencyCheck{
			Status: StatusSkipped,
			Reason: "Vault client not configured",
		}
	}

	start := time.Now()

	err := c.client.HealthCheck(ctx)
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: &latencyMs,
			Error:     fmt.Sprintf("health check failed: %v", err),
		}
	}

	return DependencyCheck{
		Status:    StatusUp,
		LatencyMs: &latencyMs,
	}
}

// detectVaultTLS determines if the Vault address uses TLS.
func detectVaultTLS(addr string) bool {
	if addr == "" {
		return false
	}

	// Check for https scheme (case-insensitive)
	return len(addr) >= 8 && (addr[:8] == "https://" || addr[:8] == "HTTPS://")
}
