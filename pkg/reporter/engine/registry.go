// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
)

// Datasource type identifiers, matching the reporter's existing
// pkg/reporter.{PostgreSQLType,MongoDBType} values. The engine resolves a
// ConnectorFactory by this string, so the registry keys must match the type the
// host stamps on each ConnectionDescriptor.
const (
	// DatasourceTypePostgres is the registry key for PostgreSQL datasources.
	DatasourceTypePostgres = "postgresql"
	// DatasourceTypeMongo is the registry key for MongoDB datasources.
	DatasourceTypeMongo = "mongodb"
)

// Registry maps a datasource type identifier to its ConnectorFactory. It
// satisfies fetcher.ConnectorRegistry: resolution is deterministic by type and
// performs no I/O, so the engine can inspect connectors before deciding to
// connect. Building and connecting happen later through the resolved factory.
type Registry struct {
	factories map[string]fetcher.ConnectorFactory
}

// Compile-time check that Registry satisfies the engine's required port.
var _ fetcher.ConnectorRegistry = (*Registry)(nil)

// NewRegistry builds a ConnectorRegistry over the given TenantResolver, wiring
// the PostgreSQL and MongoDB connector factories. Both factories share the
// resolver so single-tenant vs multi-tenant resolution is decided once at
// bootstrap. The CircuitBreaker is optional (may be nil): when present, every
// connectivity check and query is run through the per-datasource breaker,
// reusing the reporter's existing resilience policy.
func NewRegistry(resolver TenantResolver, breaker CircuitBreaker) *Registry {
	if breaker == nil {
		breaker = noopBreaker{}
	}

	return &Registry{
		factories: map[string]fetcher.ConnectorFactory{
			DatasourceTypePostgres: &postgresFactory{resolver: resolver, breaker: breaker},
			DatasourceTypeMongo:    &mongoFactory{resolver: resolver, breaker: breaker},
		},
	}
}

// Connector returns the ConnectorFactory registered for the datasource type and
// reports whether one exists. It performs no I/O.
func (r *Registry) Connector(datasourceType string) (fetcher.ConnectorFactory, bool) {
	f, ok := r.factories[datasourceType]
	return f, ok
}
