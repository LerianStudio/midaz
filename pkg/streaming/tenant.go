// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package streaming provides midaz-side glue for the LerianStudio/lib-streaming
// producer library. It owns the helpers that map midaz request context into the
// CloudEvents envelope fields required by lib-streaming.
//
// The single helper currently exposed is ResolveTenantID — every Emit call site
// MUST pass its return value as EmitRequest.TenantID so that emissions carry a
// valid ce-tenantid header in both multi-tenant and single-tenant deployments.
package streaming

import (
	"context"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
)

// DefaultTenantID is the literal tenant ID stamped onto outbound CloudEvents
// when the service is running in single-tenant mode (or any path where the
// multi-tenant middleware did not populate the context).
//
// Convention: the value "default" signals "no specific tenant scope" to
// downstream consumers. Real multi-tenant deployments emit a resolved UUID;
// the literal "default" can never collide with one. midaz stamps this
// explicit marker rather than leaving the tenant empty so every emission
// carries a self-describing ce-tenantid header.
//
// THROUGHPUT CEILING, single-tenant deployments only. lib-streaming's partition
// key falls back system → tenant → subject → event id, so in single-tenant mode
// every event carries this one literal tenant and the whole application stream
// hashes to ONE partition of lerian.streaming.<app>. Consumer parallelism on that
// stream is therefore capped at one, however many partitions the topic has.
//
// This is a ceiling, not a correctness defect: a single partition makes ordering
// STRONGER (total order across the stream rather than per-tenant), and no event is
// lost or misrouted. Multi-tenant deployments spread across partitions by resolved
// tenant already. The fix belongs in lib-streaming as a platform-wide default —
// tenant+subject is the likely shape — and is being decided there rather than
// worked around per service, which is why midaz wires no Builder.PartitionKey
// override: a local override would fragment the fleet's partitioning contract for
// the sake of one deployment mode.
const DefaultTenantID = "default"

// ResolveTenantID returns the request-scoped tenant ID for streaming
// emissions. In multi-tenant deployments the value comes from the
// lib-commons multitenancy middleware via tmcore.GetTenantIDContext;
// in single-tenant deployments and tenantless background workers
// (e.g. relays, drainers, startup probes) it returns DefaultTenantID
// so every emission carries a valid ce-tenantid header.
//
// A nil context is treated as an empty context — the helper does NOT
// panic. Callers should still pass a real request context whenever one
// is available so trace propagation and tenant-aware logging remain
// intact.
func ResolveTenantID(ctx context.Context) string {
	if ctx == nil {
		return DefaultTenantID
	}

	if v := tmcore.GetTenantIDContext(ctx); v != "" {
		return v
	}

	return DefaultTenantID
}
