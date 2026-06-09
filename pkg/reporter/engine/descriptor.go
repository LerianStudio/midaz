// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
)

// hostAttrTenantID is the HostAttributes key under which the host stamps the
// engine TenantContext.TenantID before building a connector. The engine carries
// HostAttributes verbatim from descriptor through the connection lifecycle
// without interpreting any key, which is exactly the documented channel for a
// host to round-trip its own identity through the (tenant-scoped) connector
// contract. Reading it back at Build time is how this factory learns which
// tenant's database to resolve, without the connector contract itself growing a
// tenant field.
const hostAttrTenantID = "midaz.reporter.tenant_id"

// hostAttrSchemas optionally overrides the resolved PostgreSQL schema list for
// the connector via HostAttributes. When absent, the resolver's configured
// schema list (env-derived) is used. It exists so a host that already knows the
// schema set for a request can avoid a second env lookup; it is purely
// additive.
const hostAttrSchemas = "midaz.reporter.schemas"

// WithTenantID returns a copy of descriptor carrying the tenant ID in
// HostAttributes so the factory can resolve the correct per-tenant database at
// Build time.
//
// For ExecuteExtraction the engine resolves the descriptor handed to
// ConnectorFactory.Build from ConnectionStore.FindConnection(ctx, tenant,
// configName) — it never assembles a host descriptor itself. So the Phase 2
// ConnectionStore adapter is responsible for calling WithTenantID inside
// FindConnection, stamping the engine's TenantContext.TenantID into
// HostAttributes; that is the only path that feeds tenantIDFromDescriptor for an
// extraction step. WithTenantID is also used directly by connection-ops/test
// paths that assemble a descriptor outside the store. In single-tenant mode the
// stamped tenantID may be empty, which the single-tenant resolver ignores; in
// multi-tenant mode a missing stamp makes tenantIDFromDescriptor return empty,
// which requireTenant rejects — a wiring miss fails closed as denial, never a
// cross-tenant read.
func WithTenantID(descriptor fetcher.ConnectionDescriptor, tenantID string) fetcher.ConnectionDescriptor {
	attrs := make(map[string]any, len(descriptor.HostAttributes)+1)
	for k, v := range descriptor.HostAttributes {
		attrs[k] = v
	}

	attrs[hostAttrTenantID] = tenantID
	descriptor.HostAttributes = attrs

	return descriptor
}

// tenantIDFromDescriptor reads the tenant ID stamped into the descriptor's
// HostAttributes. It returns the empty string when no tenant was stamped (the
// single-tenant path), which is valid only for the single-tenant resolver.
func tenantIDFromDescriptor(descriptor fetcher.ConnectionDescriptor) string {
	if descriptor.HostAttributes == nil {
		return ""
	}

	if v, ok := descriptor.HostAttributes[hostAttrTenantID].(string); ok {
		return v
	}

	return ""
}

// schemaOverrideFromDescriptor reads an optional PostgreSQL schema list from the
// descriptor's HostAttributes. It returns nil when none was supplied, and the
// resolver-configured schemas are used instead.
func schemaOverrideFromDescriptor(descriptor fetcher.ConnectionDescriptor) []string {
	if descriptor.HostAttributes == nil {
		return nil
	}

	switch v := descriptor.HostAttributes[hostAttrSchemas].(type) {
	case []string:
		if len(v) == 0 {
			return nil
		}

		out := make([]string, len(v))
		copy(out, v)

		return out
	case []any:
		out := make([]string, 0, len(v))

		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}

		if len(out) == 0 {
			return nil
		}

		return out
	default:
		return nil
	}
}
