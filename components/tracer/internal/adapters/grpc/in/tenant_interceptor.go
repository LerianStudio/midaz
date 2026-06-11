// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

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
func TenantUnaryInterceptor(resolver *seamtenant.Resolver) grpc.UnaryServerInterceptor {
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

		return handler(resolvedCtx, req)
	}
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
