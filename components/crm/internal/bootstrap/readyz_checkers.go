// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"time"

	libMongo "github.com/LerianStudio/lib-commons/v4/commons/mongo"
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
