// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const interceptorTenantID = "tenant-007"

func stubPoolDB(t *testing.T) dbresolver.DB {
	t.Helper()

	sqlDB, _, err := sqlmock.New()
	require.NoError(t, err)

	t.Cleanup(func() { _ = sqlDB.Close() })

	return dbresolver.New(dbresolver.WithPrimaryDBs(sqlDB))
}

func unaryInfo() *grpc.UnaryServerInfo {
	return &grpc.UnaryServerInfo{FullMethod: "/reservation.v1.ReservationService/Reserve"}
}

func TestTenantUnaryInterceptor_PresentMetadataBindsPool(t *testing.T) {
	stub := stubPoolDB(t)

	resolver := seamtenant.NewResolverWithPool(
		func(context.Context, string) (dbresolver.DB, error) { return stub, nil },
		true,
	)

	interceptor := TenantUnaryInterceptor(resolver)

	ctx := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs(seamtenant.MetadataKey, interceptorTenantID),
	)

	var handlerCtx context.Context

	handler := func(c context.Context, _ any) (any, error) {
		handlerCtx = c
		return "ok", nil
	}

	resp, err := interceptor(ctx, nil, unaryInfo(), handler)
	require.NoError(t, err)
	require.Equal(t, "ok", resp)

	// The handler runs with the tenant id and resolved pool bound to its ctx.
	require.Equal(t, interceptorTenantID, tmcore.GetTenantIDContext(handlerCtx))
	require.Equal(t, stub, tmcore.GetPGContext(handlerCtx))
}

func TestTenantUnaryInterceptor_MissingMetadataUnderMTFailsInvalidArgument(t *testing.T) {
	called := false

	resolver := seamtenant.NewResolverWithPool(
		func(context.Context, string) (dbresolver.DB, error) {
			called = true
			return stubPoolDB(t), nil
		},
		true,
	)

	interceptor := TenantUnaryInterceptor(resolver)

	// No incoming metadata at all.
	handler := func(context.Context, any) (any, error) {
		return "ok", nil
	}

	resp, err := interceptor(context.Background(), nil, unaryInfo(), handler)
	require.Nil(t, resp)
	require.False(t, called, "missing tenant key must never resolve a pool")

	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.InvalidArgument, st.Code())
	require.Equal(t, constant.ErrReservationTenantRequired.Error(), st.Message())
}

func TestTenantUnaryInterceptor_EmptyMetadataValueFailsInvalidArgument(t *testing.T) {
	resolver := seamtenant.NewResolverWithPool(
		func(context.Context, string) (dbresolver.DB, error) { return stubPoolDB(t), nil },
		true,
	)

	interceptor := TenantUnaryInterceptor(resolver)

	ctx := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs(seamtenant.MetadataKey, ""),
	)

	handler := func(context.Context, any) (any, error) {
		return "ok", nil
	}

	resp, err := interceptor(ctx, nil, unaryInfo(), handler)
	require.Nil(t, resp)

	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.InvalidArgument, st.Code())
}

func TestTenantUnaryInterceptor_SingleTenantNoOpPassesThrough(t *testing.T) {
	// nil pool ⇒ no-op resolver; the interceptor calls the handler with the
	// untouched ctx even when no tenant metadata is present.
	resolver := seamtenant.NewResolver(nil, true)
	require.False(t, resolver.Active())

	interceptor := TenantUnaryInterceptor(resolver)

	var handlerCtx context.Context

	handler := func(c context.Context, _ any) (any, error) {
		handlerCtx = c
		return "ok", nil
	}

	resp, err := interceptor(context.Background(), nil, unaryInfo(), handler)
	require.NoError(t, err)
	require.Equal(t, "ok", resp)
	require.Empty(t, tmcore.GetTenantIDContext(handlerCtx))
	require.Nil(t, tmcore.GetPGContext(handlerCtx))
}

func TestTenantUnaryInterceptor_MetadataKeyMatchesLedgerClient(t *testing.T) {
	// The gRPC metadata key must be the lower-cased X-Tenant-Id header the
	// ledger client appends, or propagation silently breaks.
	require.Equal(t, "x-tenant-id", seamtenant.MetadataKey)
}
