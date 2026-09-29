// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers"
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

	interceptor := TenantUnaryInterceptor(resolver, nil)

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

	interceptor := TenantUnaryInterceptor(resolver, nil)

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

	interceptor := TenantUnaryInterceptor(resolver, nil)

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

	interceptor := TenantUnaryInterceptor(resolver, nil)

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

// recordingEnsurer records the tenants it is asked to start workers for and
// answers with err.
type recordingEnsurer struct {
	tenants []string
	err     error
}

func (r *recordingEnsurer) EnsureWorkers(_ context.Context, tenantID string) error {
	r.tenants = append(r.tenants, tenantID)

	return r.err
}

func TestTenantUnaryInterceptor_EnsuresWorkersForTheResolvedTenant(t *testing.T) {
	tests := []struct {
		name          string
		ensureErr     error
		wantCode      codes.Code
		wantHandlerOK bool
	}{
		{name: "workers started", wantCode: codes.OK, wantHandlerOK: true},
		{name: "ensure failure still serves the request", ensureErr: errors.New("spawn failed"), wantCode: codes.OK, wantHandlerOK: true},
		{name: "tenant cap reached is unavailable", ensureErr: fmt.Errorf("ensure: %w", workers.ErrTenantCapReached), wantCode: codes.Unavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := seamtenant.NewResolverWithPool(
				func(context.Context, string) (dbresolver.DB, error) { return stubPoolDB(t), nil },
				true,
			)
			ensurer := &recordingEnsurer{err: tt.ensureErr}

			interceptor := TenantUnaryInterceptor(resolver, ensurer)

			ctx := metadata.NewIncomingContext(
				context.Background(),
				metadata.Pairs(seamtenant.MetadataKey, interceptorTenantID),
			)

			handlerCalled := false
			handler := func(context.Context, any) (any, error) {
				handlerCalled = true
				return "ok", nil
			}

			_, err := interceptor(ctx, nil, unaryInfo(), handler)

			require.Equal(t, []string{interceptorTenantID}, ensurer.tenants)
			require.Equal(t, tt.wantCode, status.Code(err))
			require.Equal(t, tt.wantHandlerOK, handlerCalled)

			if tt.wantCode == codes.Unavailable {
				require.Equal(t, constant.ErrTenantCapReached.Error(), status.Convert(err).Message())
			}
		})
	}
}

func TestTenantUnaryInterceptor_SingleTenantNeverEnsuresWorkers(t *testing.T) {
	ensurer := &recordingEnsurer{}

	interceptor := TenantUnaryInterceptor(seamtenant.NewResolver(nil, false), ensurer)

	_, err := interceptor(context.Background(), nil, unaryInfo(), func(context.Context, any) (any, error) {
		return "ok", nil
	})
	require.NoError(t, err)
	require.Empty(t, ensurer.tenants)
}

func TestTenantUnaryInterceptor_ResolveFailureMapping(t *testing.T) {
	tests := []struct {
		name        string
		poolErr     error
		wantCode    codes.Code
		wantMessage string
	}{
		{
			name:        "suspended tenant is unavailable",
			poolErr:     &tmcore.TenantSuspendedError{TenantID: interceptorTenantID, Status: "suspended"},
			wantCode:    codes.Unavailable,
			wantMessage: constant.ErrReservationTenantInactive.Error(),
		},
		{
			name:        "unprovisioned tenant is unavailable",
			poolErr:     fmt.Errorf("get connection: %w", tmcore.ErrTenantNotProvisioned),
			wantCode:    codes.Unavailable,
			wantMessage: constant.ErrReservationTenantInactive.Error(),
		},
		{
			name:        "other pool failure is internal",
			poolErr:     errors.New("pool down"),
			wantCode:    codes.Internal,
			wantMessage: constant.ErrInternalServer.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := seamtenant.NewResolverWithPool(
				func(context.Context, string) (dbresolver.DB, error) { return nil, tt.poolErr },
				true,
			)
			ensurer := &recordingEnsurer{}

			interceptor := TenantUnaryInterceptor(resolver, ensurer)

			ctx := metadata.NewIncomingContext(
				context.Background(),
				metadata.Pairs(seamtenant.MetadataKey, interceptorTenantID),
			)

			handlerCalled := false
			handler := func(context.Context, any) (any, error) {
				handlerCalled = true
				return "ok", nil
			}

			resp, err := interceptor(ctx, nil, unaryInfo(), handler)
			require.Nil(t, resp)
			require.False(t, handlerCalled)
			require.Empty(t, ensurer.tenants)

			st, ok := status.FromError(err)
			require.True(t, ok)
			require.Equal(t, tt.wantCode, st.Code())
			require.Equal(t, tt.wantMessage, st.Message())
		})
	}
}
