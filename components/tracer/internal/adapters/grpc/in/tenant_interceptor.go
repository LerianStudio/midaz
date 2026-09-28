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

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TenantUnaryInterceptor accepts the x-tenant-id a platform producer asks for
// only after the tenant-manager confirms the tenant uses that producer's
// service, and then binds the tenant's PostgreSQL pool into the context. It
// must follow IdentityUnaryInterceptor: a call without a resolved Producer is a
// wiring error and fails with codes.Unavailable.
//
// Outcomes: a missing or malformed tenant under multi-tenancy is
// codes.InvalidArgument (0487); a tenant the tenant-manager does not associate
// with the producer, or whose Tracer pool it denies, is codes.PermissionDenied
// (0043); a tenant-manager or pool that cannot answer is codes.Unavailable
// (0161), an availability failure the caller's fail posture governs; a
// cancelled call or passed deadline keeps its own code. Single-tenant mode
// ignores the metadata. A missing producer, and an authorizer and resolver that
// disagree on multi-tenancy, are deployment defects that fail closed with
// codes.Unavailable (0527).
func TenantUnaryInterceptor(authz *producerauth.TenantAuthorizer, resolver *seamtenant.Resolver) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resolvedCtx, err := seamtenant.AuthorizeTenant(ctx, authz, resolver, tenantIDFromMetadata(ctx))
		if err != nil {
			return nil, tenantStatusError(err)
		}

		return handler(resolvedCtx, req)
	}
}

// ContextReservationUnaryInterceptor authenticates the producer, then
// authorizes its requested tenant and binds the tenant pool. Bootstrap and
// transport tests share this exact chain.
func ContextReservationUnaryInterceptor(identity *producerauth.Registry, authz *producerauth.TenantAuthorizer, tenant *seamtenant.Resolver) grpc.UnaryServerInterceptor {
	authenticate := IdentityUnaryInterceptor(identity)
	authorize := TenantUnaryInterceptor(authz, tenant)

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return authenticate(ctx, req, info, func(verified context.Context, input any) (any, error) {
			return authorize(verified, input, info, handler)
		})
	}
}

// tenantStatusError maps a seamtenant.AuthorizeTenant sentinel onto its gRPC
// status.
func tenantStatusError(err error) error {
	switch {
	case errors.Is(err, constant.ErrReservationTenantRequired):
		return status.Error(codes.InvalidArgument, constant.ErrReservationTenantRequired.Error())
	case errors.Is(err, constant.ErrInsufficientPrivileges):
		return status.Error(codes.PermissionDenied, constant.ErrInsufficientPrivileges.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, context.Canceled.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, context.DeadlineExceeded.Error())
	case errors.Is(err, constant.ErrTenantServiceUnavailable):
		return status.Error(codes.Unavailable, constant.ErrTenantServiceUnavailable.Error())
	default:
		return status.Error(codes.Unavailable, constant.ErrContextPolicyUnavailable.Error())
	}
}

// tenantIDFromMetadata reads the tenant id from incoming gRPC metadata.
// Returns an empty string when absent; authorization maps empty to the clean
// missing-tenant failure under multi-tenancy.
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
