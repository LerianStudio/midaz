// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
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

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers"
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

// newReservationTenantApp wires the middleware ahead of a terminal handler that
// records the resolved request context for assertion.
func newReservationTenantApp(resolver *seamtenant.Resolver, captured *context.Context) *fiber.App {
	app := fiber.New()
	app.Post("/v1/reservations", reservationTenantMiddleware(resolver, nil), func(c fiber.Ctx) error {
		*captured = c.Context()
		return c.SendStatus(http.StatusCreated)
	})

	return app
}

func TestReservationTenantMiddleware_PresentHeaderBindsPool(t *testing.T) {
	stub := mwStubDB(t)

	var gotTenant string

	resolver := seamtenant.NewResolverWithPool(
		func(_ context.Context, tenantID string) (dbresolver.DB, error) {
			gotTenant = tenantID
			return stub, nil
		},
		true,
	)

	var captured context.Context

	app := newReservationTenantApp(resolver, &captured)

	req := httptest.NewRequest(http.MethodPost, "/v1/reservations", nil)
	req.Header.Set(seamtenant.HeaderName, mwTenantID)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.Equal(t, mwTenantID, gotTenant)
	require.Equal(t, mwTenantID, tmcore.GetTenantIDContext(captured))
	require.Equal(t, stub, tmcore.GetPGContext(captured))
}

func TestReservationTenantMiddleware_MissingHeaderUnderMTFails4xx(t *testing.T) {
	called := false

	resolver := seamtenant.NewResolverWithPool(
		func(context.Context, string) (dbresolver.DB, error) {
			called = true
			return mwStubDB(t), nil
		},
		true,
	)

	var captured context.Context

	app := newReservationTenantApp(resolver, &captured)

	req := httptest.NewRequest(http.MethodPost, "/v1/reservations", nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// 4xx, never the terminal handler, never a resolved pool.
	require.GreaterOrEqual(t, resp.StatusCode, 400)
	require.Less(t, resp.StatusCode, 500)
	require.False(t, called, "missing header must never resolve a pool")
	require.Nil(t, captured, "terminal handler must not run on a missing trusted tenant")
}

func TestReservationTenantMiddleware_SingleTenantNoOpPassesThrough(t *testing.T) {
	resolver := seamtenant.NewResolver(nil, true)
	require.False(t, resolver.Active())

	var captured context.Context

	app := newReservationTenantApp(resolver, &captured)

	// No header, single-tenant mode: passes through to the terminal handler.
	req := httptest.NewRequest(http.MethodPost, "/v1/reservations", nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, captured)
	require.Empty(t, tmcore.GetTenantIDContext(captured))
	require.Nil(t, tmcore.GetPGContext(captured))
}

// failingEnsurer answers every EnsureWorkers call with err, recording the tenant.
type failingEnsurer struct {
	tenants []string
	err     error
}

func (f *failingEnsurer) EnsureWorkers(_ context.Context, tenantID string) error {
	f.tenants = append(f.tenants, tenantID)

	return f.err
}

func TestReservationTenantMiddleware_EnsuresWorkersForTheResolvedTenant(t *testing.T) {
	tests := []struct {
		name           string
		ensureErr      error
		wantStatus     int
		wantRetryAfter bool
	}{
		{name: "workers started", wantStatus: http.StatusCreated},
		{name: "ensure failure still serves the request", ensureErr: errors.New("spawn failed"), wantStatus: http.StatusCreated},
		{name: "tenant cap reached is 503", ensureErr: fmt.Errorf("ensure: %w", workers.ErrTenantCapReached), wantStatus: http.StatusServiceUnavailable, wantRetryAfter: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := seamtenant.NewResolverWithPool(
				func(context.Context, string) (dbresolver.DB, error) { return mwStubDB(t), nil },
				true,
			)
			ensurer := &failingEnsurer{err: tt.ensureErr}

			handlerCalled := false

			app := fiber.New()
			app.Post("/v1/reservations", reservationTenantMiddleware(resolver, ensurer), func(c fiber.Ctx) error {
				handlerCalled = true
				return c.SendStatus(http.StatusCreated)
			})

			req := httptest.NewRequest(http.MethodPost, "/v1/reservations", nil)
			req.Header.Set(seamtenant.HeaderName, mwTenantID)

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, []string{mwTenantID}, ensurer.tenants)
			require.Equal(t, tt.wantStatus, resp.StatusCode)
			require.Equal(t, tt.wantStatus == http.StatusCreated, handlerCalled)

			if tt.wantRetryAfter {
				require.NotEmpty(t, resp.Header.Get("Retry-After"))

				body, readErr := io.ReadAll(resp.Body)
				require.NoError(t, readErr)
				require.Contains(t, string(body), constant.ErrTenantCapReached.Error())
			}
		})
	}
}

func TestReservationTenantMiddleware_SingleTenantNeverEnsuresWorkers(t *testing.T) {
	ensurer := &recordingEnsurer{}

	app := fiber.New()
	app.Post("/v1/reservations", reservationTenantMiddleware(seamtenant.NewResolver(nil, false), ensurer), func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusCreated)
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/v1/reservations", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.Zero(t, ensurer.callCount())
}
