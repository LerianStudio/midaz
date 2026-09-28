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
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const interceptorTenantID = "tenant-007"

var interceptorProducer = producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaCert}

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

// tenantFixture records every tenant-manager lookup and pool resolution so a
// test can prove which steps ran and in which order.
type tenantFixture struct {
	lookupErr error
	poolErr   error
	pool      dbresolver.DB
	steps     []string
	services  []string
}

func (f *tenantFixture) authorizer(mtEnabled bool) *producerauth.TenantAuthorizer {
	return producerauth.NewTenantAuthorizer(func(_ context.Context, tenantID, service string) error {
		f.steps = append(f.steps, "lookup:"+tenantID)
		f.services = append(f.services, service)

		return f.lookupErr
	}, mtEnabled)
}

func (f *tenantFixture) resolver(mtEnabled bool) *seamtenant.Resolver {
	return seamtenant.NewResolverWithPool(func(_ context.Context, tenantID string) (dbresolver.DB, error) {
		f.steps = append(f.steps, "pool:"+tenantID)

		if f.poolErr != nil {
			return nil, f.poolErr
		}

		return f.pool, nil
	}, mtEnabled)
}

func producerContext(tenantID string) context.Context {
	ctx := producerauth.WithProducer(context.Background(), interceptorProducer)
	ctx = contextutil.WithIntegrationIdentity(ctx, contextutil.IntegrationIdentity{ID: interceptorProducer.Service})

	if tenantID == "" {
		return ctx
	}

	return metadata.NewIncomingContext(ctx, metadata.Pairs(seamtenant.MetadataKey, tenantID))
}

func TestTenantUnaryInterceptor_AssociatedTenantBindsPool(t *testing.T) {
	t.Parallel()

	fixture := &tenantFixture{pool: stubPoolDB(t)}

	var handlerCtx context.Context

	resp, err := TenantUnaryInterceptor(fixture.authorizer(true), fixture.resolver(true))(producerContext(interceptorTenantID), nil, unaryInfo(), func(c context.Context, _ any) (any, error) {
		handlerCtx = c

		return "ok", nil
	})
	require.NoError(t, err)
	require.Equal(t, "ok", resp)
	require.Equal(t, []string{"lookup:" + interceptorTenantID, "pool:" + interceptorTenantID}, fixture.steps)
	require.Equal(t, []string{producerauth.ServiceLedger}, fixture.services)
	require.Equal(t, interceptorTenantID, tmcore.GetTenantIDContext(handlerCtx))
	require.Equal(t, fixture.pool, tmcore.GetPGContext(handlerCtx))

	identity, ok := contextutil.GetIntegrationIdentity(handlerCtx)
	require.True(t, ok)
	require.Equal(t, contextutil.IntegrationIdentity{ID: producerauth.ServiceLedger}, identity)
}

func TestTenantUnaryInterceptor_ClassifiesEveryOutcome(t *testing.T) {
	t.Parallel()

	networkErr := errors.New("tenant-manager unreachable")

	for _, tc := range []struct {
		name      string
		tenantID  string
		lookupErr error
		poolErr   error
		want      codes.Code
		message   string
		steps     int
	}{
		{name: "missing tenant", want: codes.InvalidArgument, message: constant.ErrReservationTenantRequired.Error()},
		{name: "invalid tenant", tenantID: "tenant with space", want: codes.InvalidArgument, message: constant.ErrReservationTenantRequired.Error()},
		{name: "tenant not found for producer", tenantID: interceptorTenantID, lookupErr: fmt.Errorf("lookup: %w", tmcore.ErrTenantNotFound), want: codes.PermissionDenied, message: constant.ErrInsufficientPrivileges.Error(), steps: 1},
		{name: "producer service not associated", tenantID: interceptorTenantID, lookupErr: fmt.Errorf("lookup: %w", tmcore.ErrTenantServiceAccessDenied), want: codes.PermissionDenied, message: constant.ErrInsufficientPrivileges.Error(), steps: 1},
		{name: "tenant-manager unavailable", tenantID: interceptorTenantID, lookupErr: networkErr, want: codes.Unavailable, message: constant.ErrTenantServiceUnavailable.Error(), steps: 1},
		{name: "tenant-manager breaker open", tenantID: interceptorTenantID, lookupErr: tmcore.ErrCircuitBreakerOpen, want: codes.Unavailable, message: constant.ErrTenantServiceUnavailable.Error(), steps: 1},
		{name: "tenant lookup canceled", tenantID: interceptorTenantID, lookupErr: context.Canceled, want: codes.Canceled, message: context.Canceled.Error(), steps: 1},
		{name: "tenant lookup timed out", tenantID: interceptorTenantID, lookupErr: context.DeadlineExceeded, want: codes.DeadlineExceeded, message: context.DeadlineExceeded.Error(), steps: 1},
		{name: "tracer pool not found", tenantID: interceptorTenantID, poolErr: tmcore.ErrTenantNotFound, want: codes.PermissionDenied, message: constant.ErrInsufficientPrivileges.Error(), steps: 2},
		{name: "tracer pool suspended", tenantID: interceptorTenantID, poolErr: &tmcore.TenantSuspendedError{TenantID: interceptorTenantID, Status: "suspended"}, want: codes.PermissionDenied, message: constant.ErrInsufficientPrivileges.Error(), steps: 2},
		{name: "tracer pool unavailable", tenantID: interceptorTenantID, poolErr: networkErr, want: codes.Unavailable, message: constant.ErrTenantServiceUnavailable.Error(), steps: 2},
		{name: "tracer pool canceled", tenantID: interceptorTenantID, poolErr: context.Canceled, want: codes.Canceled, message: context.Canceled.Error(), steps: 2},
		{name: "tracer pool timed out", tenantID: interceptorTenantID, poolErr: context.DeadlineExceeded, want: codes.DeadlineExceeded, message: context.DeadlineExceeded.Error(), steps: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := &tenantFixture{lookupErr: tc.lookupErr, poolErr: tc.poolErr, pool: stubPoolDB(t)}

			resp, err := TenantUnaryInterceptor(fixture.authorizer(true), fixture.resolver(true))(producerContext(tc.tenantID), nil, unaryInfo(), func(context.Context, any) (any, error) {
				t.Fatal("rejected tenant reached the handler")

				return nil, nil
			})
			require.Nil(t, resp)
			require.Equal(t, tc.want, status.Code(err))
			require.Equal(t, tc.message, status.Convert(err).Message())
			require.Len(t, fixture.steps, tc.steps)
		})
	}
}

func TestTenantUnaryInterceptor_MissingProducerIsUnavailable(t *testing.T) {
	t.Parallel()

	fixture := &tenantFixture{pool: stubPoolDB(t)}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(seamtenant.MetadataKey, interceptorTenantID))

	for name, mtEnabled := range map[string]bool{"multi-tenant": true, "single-tenant": false} {
		t.Run(name, func(t *testing.T) {
			_, err := TenantUnaryInterceptor(fixture.authorizer(mtEnabled), fixture.resolver(mtEnabled))(ctx, nil, unaryInfo(), func(context.Context, any) (any, error) {
				t.Fatal("unauthenticated call reached the handler")

				return nil, nil
			})
			require.Equal(t, codes.Unavailable, status.Code(err))
			require.Equal(t, constant.ErrContextPolicyUnavailable.Error(), status.Convert(err).Message())
			require.Empty(t, fixture.steps)
		})
	}
}

func TestTenantUnaryInterceptor_MismatchedTenancyFailsClosed(t *testing.T) {
	t.Parallel()

	fixture := &tenantFixture{pool: stubPoolDB(t)}

	for name, interceptor := range map[string]grpc.UnaryServerInterceptor{
		"resolver without authorizer": TenantUnaryInterceptor(fixture.authorizer(false), fixture.resolver(true)),
		"authorizer without resolver": TenantUnaryInterceptor(fixture.authorizer(true), fixture.resolver(false)),
		"nil authorizer":              TenantUnaryInterceptor(nil, fixture.resolver(true)),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := interceptor(producerContext(interceptorTenantID), nil, unaryInfo(), func(context.Context, any) (any, error) {
				t.Fatal("mismatched tenancy reached the handler")

				return nil, nil
			})
			require.Equal(t, codes.Unavailable, status.Code(err))
			require.Equal(t, constant.ErrContextPolicyUnavailable.Error(), status.Convert(err).Message())
			require.Empty(t, fixture.steps)
		})
	}
}

func TestTenantUnaryInterceptor_SingleTenantIgnoresMetadata(t *testing.T) {
	t.Parallel()

	fixture := &tenantFixture{pool: stubPoolDB(t)}

	var handlerCtx context.Context

	resp, err := TenantUnaryInterceptor(fixture.authorizer(false), seamtenant.NewResolver(nil, true))(producerContext(interceptorTenantID), nil, unaryInfo(), func(c context.Context, _ any) (any, error) {
		handlerCtx = c

		return "ok", nil
	})
	require.NoError(t, err)
	require.Equal(t, "ok", resp)
	require.Empty(t, fixture.steps)
	require.Empty(t, tmcore.GetTenantIDContext(handlerCtx))
	require.Nil(t, tmcore.GetPGContext(handlerCtx))
}

func TestContextReservationUnaryInterceptor_AuthenticatesBeforeTenant(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		uri   string
		want  codes.Code
		steps []string
	}{
		{name: "unknown certificate", uri: "spiffe://example.test/service/unknown", want: codes.PermissionDenied},
		{name: "mapped certificate", uri: producerCertURI, want: codes.OK, steps: []string{"lookup:" + interceptorTenantID, "pool:" + interceptorTenantID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := &tenantFixture{pool: stubPoolDB(t)}
			ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(seamtenant.MetadataKey, interceptorTenantID, "x-integration-id", "forged"))
			ctx = peer.NewContext(ctx, &peer.Peer{AuthInfo: verifiedTLSInfo(t, tc.uri)})

			var handlerCtx context.Context

			_, err := ContextReservationUnaryInterceptor(certRegistry(t), fixture.authorizer(true), fixture.resolver(true))(ctx, nil, unaryInfo(), func(c context.Context, _ any) (any, error) {
				handlerCtx = c

				return "ok", nil
			})
			require.Equal(t, tc.want, status.Code(err))
			require.Equal(t, tc.steps, fixture.steps)

			if tc.want != codes.OK {
				require.Nil(t, handlerCtx)

				return
			}

			require.Equal(t, interceptorTenantID, tmcore.GetTenantIDContext(handlerCtx))
			require.Equal(t, fixture.pool, tmcore.GetPGContext(handlerCtx))

			identity, ok := contextutil.GetIntegrationIdentity(handlerCtx)
			require.True(t, ok)
			require.Equal(t, contextutil.IntegrationIdentity{ID: producerauth.ServiceLedger}, identity)
		})
	}
}

func TestTenantUnaryInterceptor_MetadataKeyMatchesLedgerClient(t *testing.T) {
	t.Parallel()

	// The gRPC metadata key must be the lower-cased X-Tenant-Id header the
	// ledger client appends, or propagation silently breaks.
	require.Equal(t, "x-tenant-id", seamtenant.MetadataKey)
}
