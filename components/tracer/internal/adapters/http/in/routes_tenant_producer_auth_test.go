// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// tracerConnectionsFake answers the tenant-manager's tracer connection lookup
// with a fixed status and counts the calls, so a test sees whether the
// lib-commons tenant middleware reached it. With a PostgreSQL port it answers
// 200 with a tracer pool config pointing at that port.
type tracerConnectionsFake struct {
	server *httptest.Server
	status atomic.Int32
	pgPort int
	mu     sync.Mutex
	paths  []string
}

func startTracerConnectionsFake(t *testing.T, status int) *tracerConnectionsFake {
	t.Helper()

	return newTracerConnectionsFake(t, status, 0)
}

// startTracerPoolFake serves every tenant a tracer pool on a PostgreSQL wire
// fake, so the production pool manager resolves it.
func startTracerPoolFake(t *testing.T) *tracerConnectionsFake {
	t.Helper()

	return newTracerConnectionsFake(t, http.StatusOK, testutil.StartFakePostgres(t).Port())
}

func newTracerConnectionsFake(t *testing.T, status, pgPort int) *tracerConnectionsFake {
	t.Helper()

	fake := &tracerConnectionsFake{pgPort: pgPort}
	fake.status.Store(int32(status))
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		fake.paths = append(fake.paths, r.URL.Path)
		fake.mu.Unlock()

		code := int(fake.status.Load())
		if code != http.StatusOK || fake.pgPort == 0 {
			http.Error(w, http.StatusText(code), code)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tracerPoolConfig(r.URL.Path, fake.pgPort))
	}))
	t.Cleanup(fake.server.Close)

	return fake
}

// tracerPoolConfig is the tenant-manager tracer connection document for the
// tenant named in path, whose pool is served on pgPort.
func tracerPoolConfig(path string, pgPort int) map[string]any {
	parts := strings.Split(strings.Trim(path, "/"), "/")

	tenantID := ""
	if len(parts) > 2 {
		tenantID = parts[2]
	}

	return map[string]any{
		"id": tenantID, "tenantSlug": tenantID, "service": trcConstant.ApplicationName, "status": "active", "isolationMode": "isolated",
		"databases": map[string]any{trcConstant.ModuleName: map[string]any{"postgresql": map[string]any{
			"host": "127.0.0.1", "port": pgPort, "database": "tracer_" + tenantID, "username": "tracer", "password": "tracer",
		}}},
	}
}

func (f *tracerConnectionsFake) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.paths...)
}

// multiTenantReservationRoutes builds the production reservation routes under
// multi-tenancy: the guard authorizes through accessManager, the authorizer
// answers lookup, and the real lib-commons tenant middleware resolves pools
// from connections.
func multiTenantReservationRoutes(t *testing.T, accessManager *accessManagerFake, connections *tracerConnectionsFake, lookup producerauth.TenantLookup) *fiber.App {
	t.Helper()

	return multiTenantRoutes(t, accessManager, connections, lookup, nil)
}

// multiTenantRoutes is multiTenantReservationRoutes with mutate applied to the
// route dependencies before NewRoutes builds them.
func multiTenantRoutes(t *testing.T, accessManager *accessManagerFake, connections *tracerConnectionsFake, lookup producerauth.TenantLookup, mutate func(*RoutesDeps)) *fiber.App {
	t.Helper()

	client, err := tmclient.NewClient(connections.server.URL, testutil.NewMockLogger(), tmclient.WithServiceAPIKey("svc-api-key"), tmclient.WithAllowInsecureHTTP())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	pgManager := tmpostgres.NewManager(client, trcConstant.ApplicationName, tmpostgres.WithModule(trcConstant.ModuleName))
	t.Cleanup(func() { _ = pgManager.Close(context.Background()) })

	deps := contextReservationRoutesDeps(t)
	deps.Guard = producerAuthGuard(accessManager, false)
	deps.MultiTenantEnabled = true
	deps.PgManager = pgManager
	deps.ContextReservationTenants = producerauth.NewTenantAuthorizer(lookup, true)

	if mutate != nil {
		mutate(&deps)
	}

	app, err := NewRoutes(deps)
	require.NoError(t, err, "multi-tenant reservations need no producer roster")

	return app
}

func TestNewRoutes_MultiTenantReservationChainOrder(t *testing.T) {
	t.Parallel()

	paths := []string{
		"/v1/reservations",
		"/v1/reservations/transaction/0195d3b4-5a01-7000-8000-000000000001/confirm",
		"/v1/reservations/transaction/0195d3b4-5a01-7000-8000-000000000001/release",
		"/v1/reservations/0195d3b4-5a01-7000-8000-000000000001/confirm",
		"/v1/reservations/0195d3b4-5a01-7000-8000-000000000001/release",
	}

	for name, tc := range map[string]struct {
		token            func(*testing.T) string
		header           http.Header
		associated       bool
		connectionStatus int
		status           int
		code             string
		lookups          int
		connections      int
	}{
		"user token stops at the claim check": {
			token: func(t *testing.T) string {
				return tenantProducerToken(t, func(c jwt.MapClaims) { c["type"] = "normal-user"; c["owner"] = "lerian" })
			},
			status: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error(),
		},
		"requested tenant other than the token tenant stops at the claim check": {
			token:  func(t *testing.T) string { return tenantProducerToken(t, nil) },
			header: requestedTenant("0195d3b45a0170008000000000000008"), associated: true, connectionStatus: http.StatusOK,
			status: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error(),
		},
		"tenant without a ledger association stops before any pool": {
			token:  func(t *testing.T) string { return tenantProducerToken(t, nil) },
			status: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error(), lookups: 1,
		},
		"tenant without a tracer pool is forbidden": {
			token: func(t *testing.T) string { return tenantProducerToken(t, nil) }, associated: true, connectionStatus: http.StatusNotFound,
			status: http.StatusForbidden, code: constant.ErrInsufficientPrivileges.Error(), lookups: 1, connections: 1,
		},
		"tenant-manager outage on the pool is unavailable": {
			token: func(t *testing.T) string { return tenantProducerToken(t, nil) }, associated: true, connectionStatus: http.StatusServiceUnavailable,
			status: http.StatusServiceUnavailable, code: constant.ErrTenantServiceUnavailable.Error(), lookups: 1, connections: 1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for _, path := range paths {
				accessManager := startAccessManagerFake(t)
				connections := startTracerConnectionsFake(t, tc.connectionStatus)

				var lookups []string

				app := multiTenantReservationRoutes(t, accessManager, connections, func(_ context.Context, tenantID, service string) error {
					lookups = append(lookups, service+":"+tenantID)

					if tc.associated {
						return nil
					}

					return fmt.Errorf("tenant is not active for %s: %w", service, tmcore.ErrTenantNotFound)
				})

				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+tc.token(t))

				for name, values := range tc.header {
					req.Header[name] = values
				}

				resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
				require.NoError(t, err)

				response := readProducerAuthResponse(t, resp)
				requireProblem(t, response, tc.status, tc.code, "")

				require.Len(t, accessManager.recorded(), 1, "%s: the guard decided first", path)
				require.Len(t, lookups, tc.lookups, "%s: the association is checked only after the claims", path)

				for _, lookup := range lookups {
					require.Equal(t, producerauth.ServiceLedger+":"+mtTokenTenant, lookup, "the token tenant is looked up for the ledger")
				}

				require.Len(t, connections.recorded(), tc.connections, "%s: a pool is resolved only for an associated tenant", path)

				for _, connection := range connections.recorded() {
					require.Contains(t, connection, "/tenants/"+mtTokenTenant+"/", "the pool is resolved for the token tenant")
				}
			}
		})
	}
}

func TestNewRoutes_MultiTenantContextReservationRequiresTenantPool(t *testing.T) {
	t.Parallel()

	deps := withProducerVerifier(t, contextReservationRoutesDeps(t), startAccessManagerFake(t))
	deps.MultiTenantEnabled = true
	deps.ContextReservationProducers = nil
	deps.ContextReservationTenants = producerauth.NewTenantAuthorizer(func(context.Context, string, string) error { return nil }, true)

	app, err := NewRoutes(deps)
	require.ErrorContains(t, err, "multi-tenant context reservations require the tenant pool manager")
	require.Nil(t, app)
}

func TestNewRoutes_MultiTenantContextReservationRefusesUnverifiedProducers(t *testing.T) {
	t.Parallel()

	deps := contextReservationRoutesDeps(t)
	deps.MultiTenantEnabled = true
	deps.ContextReservationUnverifiedProducers = true
	deps.ContextReservationProducers = testProducerRegistry(t)

	app, err := NewRoutes(deps)
	require.ErrorContains(t, err, "context reservations require a guard that authorizes callers against the Access Manager")
	require.Nil(t, app)
}

func readProducerAuthResponse(t *testing.T, resp *http.Response) producerAuthResponse {
	t.Helper()

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return producerAuthResponse{status: resp.StatusCode, body: body, contentType: resp.Header.Get("Content-Type")}
}
