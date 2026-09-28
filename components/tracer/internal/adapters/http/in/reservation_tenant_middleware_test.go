// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const mwTenantID = "tenant-007"

func mwStubDB(t *testing.T) dbresolver.DB {
	t.Helper()

	sqlDB, _, err := sqlmock.New()
	require.NoError(t, err)

	t.Cleanup(func() { _ = sqlDB.Close() })

	return dbresolver.New(dbresolver.WithPrimaryDBs(sqlDB))
}

// mwTenantFixture records every tenant-manager lookup and pool resolution so
// a test can prove which steps ran and in which order.
type mwTenantFixture struct {
	lookupErr error
	poolErr   error
	pool      dbresolver.DB
	steps     []string
	services  []string
}

func (f *mwTenantFixture) authorizer(mtEnabled bool) *producerauth.TenantAuthorizer {
	return producerauth.NewTenantAuthorizer(func(_ context.Context, tenantID, service string) error {
		f.steps = append(f.steps, "lookup:"+tenantID)
		f.services = append(f.services, service)

		return f.lookupErr
	}, mtEnabled)
}

func (f *mwTenantFixture) resolver(mtEnabled bool) *seamtenant.Resolver {
	return seamtenant.NewResolverWithPool(func(_ context.Context, tenantID string) (dbresolver.DB, error) {
		f.steps = append(f.steps, "pool:"+tenantID)

		if f.poolErr != nil {
			return nil, f.poolErr
		}

		return f.pool, nil
	}, mtEnabled)
}

// injectProducer stands in for the producer authentication middleware.
func injectProducer(c fiber.Ctx) error {
	producer := producerauth.Producer{Service: producerauth.ServiceLedger, Via: producerauth.ViaToken}
	ctx := producerauth.WithProducer(c.Context(), producer)
	c.SetContext(contextutil.WithIntegrationIdentity(ctx, contextutil.IntegrationIdentity{ID: producer.Service}))

	return c.Next()
}

// newReservationTenantApp wires the middleware ahead of a terminal handler that
// records the resolved request context for assertion.
func newReservationTenantApp(tenantMW fiber.Handler, withProducer bool, captured *context.Context) *fiber.App {
	app := fiber.New()

	handlers := []any{tenantMW, func(c fiber.Ctx) error {
		*captured = c.Context()

		return c.SendStatus(http.StatusCreated)
	}}
	if withProducer {
		handlers = append([]any{injectProducer}, handlers...)
	}

	app.Post("/v1/reservations", handlers[0], handlers[1:]...)

	return app
}

func postReservation(t *testing.T, app *fiber.App, tenantID string) (int, string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/v1/reservations", nil)
	if tenantID != "" {
		req.Header.Set(seamtenant.HeaderName, tenantID)
	}

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	if resp.StatusCode < http.StatusBadRequest {
		return resp.StatusCode, ""
	}

	var envelope struct {
		Code string `json:"code"`
	}

	require.NoError(t, json.Unmarshal(body, &envelope), string(body))

	return resp.StatusCode, envelope.Code
}

func TestReservationTenantMiddleware_AssociatedTenantBindsPool(t *testing.T) {
	t.Parallel()

	fixture := &mwTenantFixture{pool: mwStubDB(t)}

	var captured context.Context

	app := newReservationTenantApp(reservationTenantMiddleware(fixture.authorizer(true), fixture.resolver(true)), true, &captured)

	status, _ := postReservation(t, app, mwTenantID)
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, []string{"lookup:" + mwTenantID, "pool:" + mwTenantID}, fixture.steps)
	require.Equal(t, []string{producerauth.ServiceLedger}, fixture.services)
	require.Equal(t, mwTenantID, tmcore.GetTenantIDContext(captured))
	require.Equal(t, fixture.pool, tmcore.GetPGContext(captured))

	identity, ok := contextutil.GetIntegrationIdentity(captured)
	require.True(t, ok)
	require.Equal(t, contextutil.IntegrationIdentity{ID: producerauth.ServiceLedger}, identity)
}

func TestReservationTenantMiddleware_ClassifiesEveryOutcome(t *testing.T) {
	t.Parallel()

	networkErr := errors.New("tenant-manager unreachable")

	for _, tc := range []struct {
		name      string
		tenantID  string
		lookupErr error
		poolErr   error
		status    int
		code      string
		steps     int
	}{
		{name: "missing tenant", status: http.StatusBadRequest, code: constant.ErrReservationTenantRequired.Error()},
		{name: "invalid tenant", tenantID: "tenant with space", status: http.StatusBadRequest, code: constant.ErrReservationTenantRequired.Error()},
		{name: "tenant not found for producer", tenantID: mwTenantID, lookupErr: fmt.Errorf("lookup: %w", tmcore.ErrTenantNotFound), status: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error(), steps: 1},
		{name: "producer service not associated", tenantID: mwTenantID, lookupErr: fmt.Errorf("lookup: %w", tmcore.ErrTenantServiceAccessDenied), status: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error(), steps: 1},
		{name: "tenant-manager unavailable", tenantID: mwTenantID, lookupErr: networkErr, status: http.StatusServiceUnavailable, code: constant.ErrTenantServiceUnavailable.Error(), steps: 1},
		{name: "tenant-manager breaker open", tenantID: mwTenantID, lookupErr: tmcore.ErrCircuitBreakerOpen, status: http.StatusServiceUnavailable, code: constant.ErrTenantServiceUnavailable.Error(), steps: 1},
		{name: "tenant lookup canceled", tenantID: mwTenantID, lookupErr: context.Canceled, status: http.StatusServiceUnavailable, code: constant.ErrContextCancelled.Error(), steps: 1},
		{name: "tenant lookup timed out", tenantID: mwTenantID, lookupErr: context.DeadlineExceeded, status: http.StatusGatewayTimeout, code: constant.ErrValidationTimeout.Error(), steps: 1},
		{name: "tracer pool not found", tenantID: mwTenantID, poolErr: tmcore.ErrTenantNotFound, status: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error(), steps: 2},
		{name: "tracer pool suspended", tenantID: mwTenantID, poolErr: &tmcore.TenantSuspendedError{TenantID: mwTenantID, Status: "suspended"}, status: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error(), steps: 2},
		{name: "tracer pool unavailable", tenantID: mwTenantID, poolErr: networkErr, status: http.StatusServiceUnavailable, code: constant.ErrTenantServiceUnavailable.Error(), steps: 2},
		{name: "tracer pool canceled", tenantID: mwTenantID, poolErr: context.Canceled, status: http.StatusServiceUnavailable, code: constant.ErrContextCancelled.Error(), steps: 2},
		{name: "tracer pool deadline", tenantID: mwTenantID, poolErr: context.DeadlineExceeded, status: http.StatusGatewayTimeout, code: constant.ErrValidationTimeout.Error(), steps: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := &mwTenantFixture{lookupErr: tc.lookupErr, poolErr: tc.poolErr, pool: mwStubDB(t)}

			var captured context.Context

			app := newReservationTenantApp(reservationTenantMiddleware(fixture.authorizer(true), fixture.resolver(true)), true, &captured)

			status, code := postReservation(t, app, tc.tenantID)
			require.Equal(t, tc.status, status)
			require.Equal(t, tc.code, code)
			require.Len(t, fixture.steps, tc.steps)
			require.Nil(t, captured, "terminal handler must not run on a rejected tenant")
		})
	}
}

func TestReservationTenantMiddleware_MissingProducerIsUnavailable(t *testing.T) {
	t.Parallel()

	for name, mtEnabled := range map[string]bool{"multi-tenant": true, "single-tenant": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := &mwTenantFixture{pool: mwStubDB(t)}

			var captured context.Context

			app := newReservationTenantApp(reservationTenantMiddleware(fixture.authorizer(mtEnabled), fixture.resolver(mtEnabled)), false, &captured)

			status, code := postReservation(t, app, mwTenantID)
			require.Equal(t, http.StatusServiceUnavailable, status)
			require.Equal(t, constant.ErrContextPolicyUnavailable.Error(), code)
			require.Empty(t, fixture.steps)
			require.Nil(t, captured)
		})
	}
}

func TestReservationTenantMiddleware_MismatchedTenancyFailsClosed(t *testing.T) {
	t.Parallel()

	fixture := &mwTenantFixture{pool: mwStubDB(t)}

	for name, tenantMW := range map[string]fiber.Handler{
		"resolver without authorizer": reservationTenantMiddleware(fixture.authorizer(false), fixture.resolver(true)),
		"authorizer without resolver": reservationTenantMiddleware(fixture.authorizer(true), fixture.resolver(false)),
		"nil authorizer":              reservationTenantMiddleware(nil, fixture.resolver(true)),
	} {
		t.Run(name, func(t *testing.T) {
			var captured context.Context

			status, code := postReservation(t, newReservationTenantApp(tenantMW, true, &captured), mwTenantID)
			require.Equal(t, http.StatusServiceUnavailable, status)
			require.Equal(t, constant.ErrContextPolicyUnavailable.Error(), code)
			require.Empty(t, fixture.steps)
			require.Nil(t, captured)
		})
	}
}

func TestReservationTenantMiddleware_SingleTenantIgnoresHeader(t *testing.T) {
	t.Parallel()

	fixture := &mwTenantFixture{pool: mwStubDB(t)}

	var captured context.Context

	app := newReservationTenantApp(reservationTenantMiddleware(fixture.authorizer(false), seamtenant.NewResolver(nil, true)), true, &captured)

	status, _ := postReservation(t, app, mwTenantID)
	require.Equal(t, http.StatusCreated, status)
	require.Empty(t, fixture.steps)
	require.NotNil(t, captured)
	require.Empty(t, tmcore.GetTenantIDContext(captured))
	require.Nil(t, tmcore.GetPGContext(captured))
}
