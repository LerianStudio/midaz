// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package seamtenant_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const authorizeTenantID = "tenant-007"

func authorizeStubDB(t *testing.T) dbresolver.DB {
	t.Helper()

	sqlDB, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	return dbresolver.New(dbresolver.WithPrimaryDBs(sqlDB))
}

func producerCtx(ctx context.Context) context.Context {
	return producerauth.WithProducer(ctx, producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaToken})
}

func TestAuthorizeTenantNestsPoolResolutionUnderTheAuthorizeSpan(t *testing.T) {
	t.Parallel()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	ctx := producerCtx(libObservability.ContextWithTracer(context.Background(), provider.Tracer("seamtenant-test")))
	db := authorizeStubDB(t)

	var poolParent trace.SpanContext

	resolver := seamtenant.NewResolverWithPool(func(ctx context.Context, _ string) (dbresolver.DB, error) {
		poolParent = trace.SpanContextFromContext(ctx)

		return db, nil
	}, true)
	authz := producerauth.NewTenantAuthorizer(func(context.Context, string, string) error { return nil }, true)

	resolved, err := seamtenant.AuthorizeTenant(ctx, authz, resolver, authorizeTenantID)
	require.NoError(t, err)
	require.Equal(t, authorizeTenantID, tmcore.GetTenantIDContext(resolved))
	require.Equal(t, db, tmcore.GetPGContext(resolved))

	var authorizeSpan sdktrace.ReadOnlySpan

	for _, span := range recorder.Ended() {
		if span.Name() == "middleware.reservations.authorize_tenant" {
			authorizeSpan = span
		}
	}

	require.NotNil(t, authorizeSpan)
	require.Equal(t, authorizeSpan.SpanContext().SpanID(), poolParent.SpanID(), "pool resolution runs inside the authorize span")
	require.NotEqual(t, authorizeSpan.SpanContext().SpanID(), trace.SpanContextFromContext(resolved).SpanID(),
		"the handler context does not inherit the ended authorize span")
}

func TestAuthorizeTenantReturnsOneBareSentinel(t *testing.T) {
	t.Parallel()

	suspended := &tmcore.TenantSuspendedError{TenantID: authorizeTenantID, Status: "suspended"}

	for _, tc := range []struct {
		name      string
		tenantID  string
		lookupErr error
		poolErr   error
		want      error
	}{
		{name: "missing tenant", want: constant.ErrReservationTenantRequired},
		{name: "tenant denied", tenantID: authorizeTenantID, lookupErr: fmt.Errorf("list: %w", tmcore.ErrTenantNotFound), want: constant.ErrInsufficientPrivileges},
		{name: "tenant-manager down", tenantID: authorizeTenantID, lookupErr: errors.New("connection refused"), want: constant.ErrTenantServiceUnavailable},
		{name: "lookup canceled", tenantID: authorizeTenantID, lookupErr: context.Canceled, want: context.Canceled},
		{name: "lookup timed out", tenantID: authorizeTenantID, lookupErr: context.DeadlineExceeded, want: context.DeadlineExceeded},
		{name: "pool suspended", tenantID: authorizeTenantID, poolErr: suspended, want: constant.ErrInsufficientPrivileges},
		{name: "pool canceled", tenantID: authorizeTenantID, poolErr: context.Canceled, want: context.Canceled},
		{name: "pool timed out", tenantID: authorizeTenantID, poolErr: context.DeadlineExceeded, want: context.DeadlineExceeded},
		{name: "pool unavailable", tenantID: authorizeTenantID, poolErr: errors.New("dial"), want: constant.ErrTenantServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resolver := seamtenant.NewResolverWithPool(func(context.Context, string) (dbresolver.DB, error) {
				if tc.poolErr != nil {
					return nil, tc.poolErr
				}

				return authorizeStubDB(t), nil
			}, true)
			authz := producerauth.NewTenantAuthorizer(func(context.Context, string, string) error { return tc.lookupErr }, true)

			ctx := producerCtx(context.Background())
			resolved, err := seamtenant.AuthorizeTenant(ctx, authz, resolver, tc.tenantID)
			require.Equal(t, tc.want, err, "the sentinel itself, without its cause")
			require.Equal(t, ctx, resolved)
		})
	}
}

func TestAuthorizeTenantDeploymentDefectsFailClosed(t *testing.T) {
	t.Parallel()

	active := producerauth.NewTenantAuthorizer(func(context.Context, string, string) error { return nil }, true)
	pool := seamtenant.NewResolverWithPool(func(context.Context, string) (dbresolver.DB, error) { return nil, nil }, true)

	for name, call := range map[string]func() error{
		"no producer": func() error {
			_, err := seamtenant.AuthorizeTenant(context.Background(), active, pool, authorizeTenantID)
			return err
		},
		"authorizer without resolver": func() error {
			_, err := seamtenant.AuthorizeTenant(producerCtx(context.Background()), active, seamtenant.NewResolverWithPool(nil, true), authorizeTenantID)
			return err
		},
		"unconfigured lookup": func() error {
			_, err := seamtenant.AuthorizeTenant(producerCtx(context.Background()), producerauth.NewTenantAuthorizer(nil, true), pool, authorizeTenantID)
			return err
		},
	} {
		require.Equal(t, constant.ErrContextPolicyUnavailable, call(), name)
	}
}
