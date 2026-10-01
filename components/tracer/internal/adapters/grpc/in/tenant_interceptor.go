// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// WorkerEnsurer starts the per-tenant background workers (the rule cache sync
// among them) for a tenant on its first request. Satisfied by
// *workers.WorkerSupervisor.
type WorkerEnsurer interface {
	EnsureWorkers(ctx context.Context, tenantID string) error
}

// TenantUnaryInterceptor resolves the per-tenant PostgreSQL pool from the
// x-tenant-id metadata the ledger forwards and binds it into the request
// context BEFORE the reservation handler runs. It is the tenant carrier of
// every seam identity except the Access Manager token: the transport
// (mtls/mesh), the API key and no identity, all of which run single-tenant.
//
// Under multi-tenant mode a missing/empty/invalid tenant key fails with
// codes.InvalidArgument and never resolves a default/wrong pool. In
// single-tenant (no-op) mode the resolver passes through and the key is ignored.
//
// A tenant the tenant manager reports as not provisioned, suspended or purged
// answers codes.Unavailable with ErrReservationTenantInactive; any other
// resolution failure answers codes.Internal.
//
// Once the tenant resolves, ensurer starts that tenant's workers so a tenant
// whose first traffic is a reservation still gets its rule cache loaded. A
// reached tenant cap answers codes.Unavailable so the ledger backs off; any
// other ensure failure is logged and the request proceeds. A nil ensurer skips
// the step.
func TenantUnaryInterceptor(resolver *seamtenant.Resolver, ensurer WorkerEnsurer) grpc.UnaryServerInterceptor {
	return tenantUnaryInterceptor(resolver, ensurer, seamtenant.MetadataKey, nil)
}

// TokenTenantUnaryInterceptor is TenantUnaryInterceptor for the Access Manager
// token identity: it resolves the tenant from md-tenant-id, the value lib-auth
// copied from the authorized token's tenantId claim, and ignores x-tenant-id
// for resolution, so the tenant always comes from the caller's credential. The
// claim is resolved in its lib-commons canonical form (tmcore.CanonicalTenantID),
// the key the HTTP tenant middleware resolves, so both listeners share one pool
// and one worker set per tenant; a claim that does not canonicalize is a
// missing tenant. It MUST run after the lib-auth interceptor and
// SeamPrincipalInterceptor.
func TokenTenantUnaryInterceptor(resolver *seamtenant.Resolver, ensurer WorkerEnsurer) grpc.UnaryServerInterceptor {
	return tenantUnaryInterceptor(resolver, ensurer, TokenTenantMetadataKey, canonicalTenantOrEmpty)
}

// tenantUnaryInterceptor resolves the tenant read from the metadataKey value,
// mapped through normalize when it is non-nil.
func tenantUnaryInterceptor(
	resolver *seamtenant.Resolver,
	ensurer WorkerEnsurer,
	metadataKey string,
	normalize func(string) string,
) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !resolver.Active() {
			return handler(ctx, req)
		}

		tenantID := firstMetadataValue(ctx, metadataKey)
		if normalize != nil {
			tenantID = normalize(tenantID)
		}

		resolvedCtx, err := resolver.Resolve(ctx, tenantID)
		if err != nil {
			return nil, resolveFailureStatus(ctx, tenantID, err)
		}

		if err := ensureTenantWorkers(resolvedCtx, ensurer, tmcore.GetTenantIDContext(resolvedCtx)); err != nil {
			return nil, err
		}

		return handler(resolvedCtx, req)
	}
}

// canonicalTenantOrEmpty returns the canonical form of tenantID, or "" when it
// does not canonicalize, which Resolve answers as a missing tenant.
func canonicalTenantOrEmpty(tenantID string) string {
	canonical, err := tmcore.CanonicalTenantID(tenantID)
	if err != nil {
		return ""
	}

	return canonical
}

// resolveFailureStatus maps a Resolve failure onto the gRPC status the caller
// receives.
func resolveFailureStatus(ctx context.Context, tenantID string, err error) error {
	switch {
	case errors.Is(err, constant.ErrReservationTenantRequired):
		return status.Error(codes.InvalidArgument, constant.ErrReservationTenantRequired.Error())
	case errors.Is(err, constant.ErrReservationTenantInactive):
		logger := libObservability.NewLoggerFromContext(ctx)
		logger.Log(ctx, libLog.LevelWarn, "Tenant is not active for reservations; answering unavailable",
			libLog.String("operation", "grpc.reservations.resolve_tenant"),
			libLog.String("tenant_id", tenantID),
			libLog.Err(err))

		return status.Error(codes.Unavailable, constant.ErrReservationTenantInactive.Error())
	default:
		return status.Error(codes.Internal, constant.ErrInternalServer.Error())
	}
}

// ensureTenantWorkers starts tenantID's workers through ensurer. It returns a
// gRPC status error only when the tenant cap is reached; any other failure is
// logged at Warn and swallowed so the reservation is still served.
func ensureTenantWorkers(ctx context.Context, ensurer WorkerEnsurer, tenantID string) error {
	if ensurer == nil || tenantID == "" {
		return nil
	}

	err := ensurer.EnsureWorkers(ctx, tenantID)
	if err == nil {
		return nil
	}

	logger := libObservability.NewLoggerFromContext(ctx)

	if errors.Is(err, workers.ErrTenantCapReached) {
		logger.Log(ctx, libLog.LevelWarn, "Tenant worker cap reached; answering unavailable so the caller backs off",
			libLog.String("operation", "grpc.reservations.lazy_spawn_workers"),
			libLog.String("tenant_id", tenantID),
			libLog.Err(err))

		return status.Error(codes.Unavailable, constant.ErrTenantCapReached.Error())
	}

	logger.Log(ctx, libLog.LevelWarn, "Failed to ensure workers for tenant; reservation proceeds but background sync may be unavailable",
		libLog.String("operation", "grpc.reservations.lazy_spawn_workers"),
		libLog.String("tenant_id", tenantID),
		libLog.Err(err))

	return nil
}
