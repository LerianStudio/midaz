// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamtenant"
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
	app.Post("/v1/reservations", reservationTenantMiddleware(resolver), func(c *fiber.Ctx) error {
		*captured = c.UserContext()
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
