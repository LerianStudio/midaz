// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"sort"
	"strconv"

	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
)

// DatasourceLookup is the narrow read surface the ConnectionStore needs from the
// reporter's env-configured datasource registry: resolve one datasource's safe
// descriptor fields by config name. It is satisfied by an adapter over
// pkg/reporter.SafeDataSources declared in the worker bootstrap, so this package
// does not import the bootstrap-heavy datasource map (mirroring the
// SingleTenantDatasources seam in resolver.go).
type DatasourceLookup interface {
	// LookupDatasource returns the secret-free connection fields for the named
	// datasource and whether it exists. It never returns credentials.
	LookupDatasource(configName string) (DatasourceConnection, bool)
	// DatasourceConfigNames returns the config names of every registered
	// datasource, backing List (which sorts them for deterministic order).
	DatasourceConfigNames() []string
}

// DatasourceConnection is the secret-free projection of a reporter DataSource the
// store maps onto a fetcher.ConnectionDescriptor. It deliberately carries no
// password: the engine descriptor contract is credential-free, and connectors
// resolve their live connection through the tenant resolver, not from these
// fields.
type DatasourceConnection struct {
	ConfigName   string
	Type         string
	Host         string
	Port         string
	DatabaseName string
	Username     string
	SSLMode      string
	Schemas      []string
}

// store is the read-mostly ConnectionStore over the reporter's env-configured
// datasources. The reporter's internal datasources are configured via
// DATASOURCE_* env vars, not user-CRUD, so every write method fails closed with
// an unsupported CategoryValidation error.
type store struct {
	datasources DatasourceLookup
}

// Compile-time check that store satisfies the engine's optional ConnectionStore
// port.
var _ fetcher.ConnectionStore = (*store)(nil)

// NewConnectionStore builds a read-mostly ConnectionStore backed by the
// reporter's env-configured datasources. It resolves descriptors for
// Plan/validate and extraction; persistence operations are unsupported.
func NewConnectionStore(datasources DatasourceLookup) fetcher.ConnectionStore {
	return &store{datasources: datasources}
}

// FindConnection resolves the named datasource to a secret-free descriptor and
// stamps the engine's TenantContext.TenantID into HostAttributes via
// WithTenantID. That stamp is the load-bearing link: ExecuteExtraction resolves
// the descriptor here, then hands it to ConnectorFactory.Build, which reads the
// tenant back from HostAttributes to resolve the per-tenant database. An unknown
// config name reports found=false (not an error) so the engine can surface a
// datasource-not-found validation failure rather than a technical fault.
func (s *store) FindConnection(_ context.Context, tenant fetcher.TenantContext, configName string) (fetcher.ConnectionDescriptor, bool, error) {
	conn, ok := s.datasources.LookupDatasource(configName)
	if !ok {
		return fetcher.ConnectionDescriptor{}, false, nil
	}

	descriptor, err := toDescriptor(conn)
	if err != nil {
		return fetcher.ConnectionDescriptor{}, false, err
	}

	return WithTenantID(descriptor, tenant.TenantID), true, nil
}

// FindByID treats the opaque id as the config name: the reporter addresses its
// env-configured datasources by config name, so there is no separate id space.
func (s *store) FindByID(ctx context.Context, tenant fetcher.TenantContext, id string) (fetcher.ConnectionDescriptor, bool, error) {
	return s.FindConnection(ctx, tenant, id)
}

// List returns the descriptors for every env-configured datasource in
// deterministic (config-name-sorted) order, each stamped with the tenant. It
// skips any name that fails to resolve so a single malformed config does not
// fail the whole listing.
func (s *store) List(_ context.Context, tenant fetcher.TenantContext) ([]fetcher.ConnectionDescriptor, error) {
	names := s.datasources.DatasourceConfigNames()
	sort.Strings(names)

	descriptors := make([]fetcher.ConnectionDescriptor, 0, len(names))

	for _, name := range names {
		conn, ok := s.datasources.LookupDatasource(name)
		if !ok {
			continue
		}

		descriptor, err := toDescriptor(conn)
		if err != nil {
			continue
		}

		descriptors = append(descriptors, WithTenantID(descriptor, tenant.TenantID))
	}

	return descriptors, nil
}

// Create is unsupported: reporter datasources are env-configured, not user-CRUD.
func (s *store) Create(_ context.Context, _ fetcher.TenantContext, _ fetcher.ConnectionDescriptor, _ *fetcher.ProtectedCredential) error {
	return errConnectionStoreReadOnly()
}

// Update is unsupported: reporter datasources are env-configured, not user-CRUD.
func (s *store) Update(_ context.Context, _ fetcher.TenantContext, _ fetcher.ConnectionDescriptor, _ *fetcher.ProtectedCredential) error {
	return errConnectionStoreReadOnly()
}

// Delete is unsupported: reporter datasources are env-configured, not user-CRUD.
func (s *store) Delete(_ context.Context, _ fetcher.TenantContext, _ string) error {
	return errConnectionStoreReadOnly()
}

// UpdateByID is unsupported: reporter datasources are env-configured, not
// user-CRUD.
func (s *store) UpdateByID(_ context.Context, _ fetcher.TenantContext, _ string, _ fetcher.ConnectionDescriptor, _ *fetcher.ProtectedCredential) error {
	return errConnectionStoreReadOnly()
}

// DeleteByID is unsupported: reporter datasources are env-configured, not
// user-CRUD.
func (s *store) DeleteByID(_ context.Context, _ fetcher.TenantContext, _ string) error {
	return errConnectionStoreReadOnly()
}

// ListPaged is unsupported: reporter datasources are env-configured, not
// user-CRUD. Read listing uses List.
func (s *store) ListPaged(_ context.Context, _ fetcher.TenantContext, _ fetcher.ConnectionListParams) (fetcher.ConnectionPage, error) {
	return fetcher.ConnectionPage{}, errConnectionStoreReadOnly()
}

// errConnectionStoreReadOnly is the single mint point for the unsupported-write
// error so every write method returns an identical, classified value.
func errConnectionStoreReadOnly() *fetcher.EngineError {
	return NewEngineValidationError("connection persistence is not supported: reporter datasources are configured via DATASOURCE_* environment variables, not user CRUD")
}

// toDescriptor maps a secret-free reporter datasource projection to a fetcher
// ConnectionDescriptor, mapping the reporter's type string to the registry key
// and parsing the port string to an int. A non-numeric port is a configuration
// fault and fails as CategoryValidation.
func toDescriptor(conn DatasourceConnection) (fetcher.ConnectionDescriptor, error) {
	port, err := parsePort(conn.Port)
	if err != nil {
		return fetcher.ConnectionDescriptor{}, err
	}

	descriptor := fetcher.ConnectionDescriptor{
		ConfigName:   conn.ConfigName,
		Type:         mapDatasourceType(conn.Type),
		Host:         conn.Host,
		Port:         port,
		DatabaseName: conn.DatabaseName,
		Username:     conn.Username,
		SSLMode:      conn.SSLMode,
	}

	// Carry the env-configured schema list through HostAttributes so the
	// connector factory can prefer it over a second env lookup. Absent schemas
	// fall back to the resolver's configured list at Build time.
	if len(conn.Schemas) > 0 {
		schemas := make([]string, len(conn.Schemas))
		copy(schemas, conn.Schemas)
		descriptor.HostAttributes = map[string]any{hostAttrSchemas: schemas}
	}

	return descriptor, nil
}

// parsePort converts the datasource port string to an int. An empty string maps
// to 0 (the engine treats it as unset); a non-numeric value fails closed.
func parsePort(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}

	port, err := strconv.Atoi(raw)
	if err != nil {
		return 0, NewEngineValidationError("datasource port is not a valid integer: " + raw)
	}

	return port, nil
}

// mapDatasourceType maps the reporter's datasource type string onto the engine
// registry key. The reporter already uses the same "postgresql"/"mongodb"
// literals as the registry, so this is an identity mapping today; it is a named
// seam so a future divergence has one place to bridge.
func mapDatasourceType(reporterType string) string {
	switch reporterType {
	case DatasourceTypePostgres:
		return DatasourceTypePostgres
	case DatasourceTypeMongo:
		return DatasourceTypeMongo
	default:
		return reporterType
	}
}
