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

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
)

// DefaultTenantID is the literal tenant ID stamped onto outbound CloudEvents
// when the service is running in single-tenant mode (or any path where the
// multi-tenant middleware did not populate the context).
//
// Convention: the value "default" signals "no specific tenant scope" to
// downstream consumers. Real multi-tenant deployments emit a resolved UUID;
// the literal "default" can never collide with one. Stamping an explicit
// "default" keeps every emission carrying a stable, greppable ce-tenantid
// header rather than an empty one.
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
