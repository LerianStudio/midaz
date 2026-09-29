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
	"google.golang.org/grpc/metadata"
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
// TRUSTED x-tenant-id metadata the ledger forwards and binds it into the
// request context BEFORE the reservation handler runs. The tenant key is
// trusted because the gRPC peer is mTLS-verified (or sits behind a verified
// mesh sidecar); this interceptor is registered ONLY on the reservation gRPC
// server, which is unreachable without that verified peer.
//
// Under multi-tenant mode a missing/empty/invalid tenant key fails with
// codes.InvalidArgument and never resolves a default/wrong pool. In
// single-tenant (no-op) mode the resolver passes through and the key is ignored.
//
// Once the tenant resolves, ensurer starts that tenant's workers so a tenant
// whose first traffic is a reservation still gets its rule cache loaded. A
// reached tenant cap answers codes.Unavailable so the ledger backs off; any
// other ensure failure is logged and the request proceeds. A nil ensurer skips
// the step.
func TenantUnaryInterceptor(resolver *seamtenant.Resolver, ensurer WorkerEnsurer) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !resolver.Active() {
			return handler(ctx, req)
		}

		resolvedCtx, err := resolver.Resolve(ctx, tenantIDFromMetadata(ctx))
		if err != nil {
			if errors.Is(err, constant.ErrReservationTenantRequired) {
				return nil, status.Error(codes.InvalidArgument, constant.ErrReservationTenantRequired.Error())
			}

			return nil, status.Error(codes.Internal, constant.ErrInternalServer.Error())
		}

		if err := ensureTenantWorkers(resolvedCtx, ensurer, tmcore.GetTenantIDContext(resolvedCtx)); err != nil {
			return nil, err
		}

		return handler(resolvedCtx, req)
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

// tenantIDFromMetadata reads the trusted tenant id from incoming gRPC metadata.
// Returns an empty string when absent; the resolver maps empty to the clean
// missing-tenant failure under MT.
func tenantIDFromMetadata(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	values := md.Get(seamtenant.MetadataKey)
	if len(values) == 0 {
		return ""
	}

	return values[0]
}
