//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	auth "github.com/LerianStudio/lib-auth/v4/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	problem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamidentity"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestLimitAssetRouteRequiresProducerAndAdministrator(t *testing.T) {
	for _, scenario := range []string{"allowed", "denied", "api key only", "unknown producer", "forged namespace", "too large", "conflict", "unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			permissions := make(chan map[string]any, 1)
			accessManager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					w.WriteHeader(http.StatusOK)
					return
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				permissions <- payload
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(auth.AuthResponse{Authorized: scenario != "denied"})
			}))
			t.Cleanup(accessManager.Close)
			guard := middleware.NewAuthGuard(middleware.AuthGuardConfig{AppName: "tracer", PluginAuthEnabled: true, APIKeyEnabled: true, APIKey: "validation-key"}, auth.NewAuthClient(accessManager.URL, true, libLog.NewNop()))
			binder := NewMockLimitAssetBinder(gomock.NewController(t))
			resolver, err := seamidentity.NewResolver([]seamidentity.Binding{{URI: "spiffe://example.test/ledger", IntegrationID: "ledger", AssetNamespace: "ledger"}}, 256)
			require.NoError(t, err)
			h, err := NewLimitAssetHandler(binder, resolver, assetAdminBounds(), 4096)
			require.NoError(t, err)
			app, api := limitAssetAuthApp(t, h, guard)
			security := api.OpenAPI().Paths["/limits/{id}/asset-reference"].Put.Security
			require.Len(t, security, 1)
			require.Contains(t, security[0], "BearerAuth")
			require.Contains(t, security[0], "ProducerMTLS")
			producerURI := "spiffe://example.test/ledger"
			if scenario == "unknown producer" {
				producerURI = "spiffe://example.test/unknown"
			}
			endpoint, client := serveLimitAssetTLS(t, app, producerURI)
			id := testutil.MustDeterministicUUID(89201)
			facts := []tracercontract.AccountAsset{{AccountID: testutil.MustDeterministicUUID(89202), Asset: tracercontract.AssetRef{Namespace: "ledger", ID: "asset", Code: "wBTC"}}}
			expected := http.StatusOK
			switch scenario {
			case "allowed", "conflict", "unavailable":
				binder.EXPECT().Execute(gomock.Any(), id, facts).DoAndReturn(func(ctx context.Context, _ uuid.UUID, _ []tracercontract.AccountAsset) (*tracercontract.AssetRef, error) {
					identity, ok := contextutil.GetIntegrationIdentity(ctx)
					require.True(t, ok)
					require.Equal(t, "ledger", identity.AssetNamespace)
					principal, ok := contextutil.GetPrincipal(ctx)
					require.True(t, ok)
					require.Equal(t, "asset-admin", principal.ID)
					require.Equal(t, "verified-tenant", tmcore.GetTenantIDContext(ctx))
					if scenario == "conflict" {
						return nil, constant.ErrLimitAssetReferenceConflict
					}
					if scenario == "unavailable" {
						return nil, constant.ErrContextLimitsUnavailable
					}
					return &facts[0].Asset, nil
				})
				if scenario == "conflict" {
					expected = http.StatusConflict
				}
				if scenario == "unavailable" {
					expected = http.StatusServiceUnavailable
				}
			case "denied", "unknown producer":
				expected = http.StatusForbidden
			case "api key only":
				expected = http.StatusUnauthorized
			case "forged namespace":
				facts[0].Asset.Namespace = "forged"
				expected = http.StatusBadRequest
			case "too large":
				expected = http.StatusRequestEntityTooLarge
			}
			raw, err := json.Marshal(LimitAssetDocument{AccountAssets: facts})
			require.NoError(t, err)
			if scenario == "too large" {
				raw = bytes.Repeat([]byte(" "), 5000)
			}
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPut, endpoint+"/v1/limits/"+id.String()+"/asset-reference", bytes.NewReader(raw))
			require.NoError(t, err)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-API-Key", "validation-key")
			request.Header.Set("X-Asset-Namespace", "forged")
			if scenario != "api key only" {
				token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"type": "normal-user", "sub": "asset-admin", "owner": "owner"}).SignedString([]byte("test-only-key"))
				require.NoError(t, err)
				request.Header.Set("Authorization", "Bearer "+token)
			}
			response, err := client.Do(request)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, expected, response.StatusCode)
			if scenario != "api key only" && scenario != "unknown producer" {
				select {
				case permission := <-permissions:
					require.Equal(t, "limit-asset-references", permission["resource"])
					require.Equal(t, "put", permission["action"])
				default:
					t.Fatal("Access Manager was not consulted")
				}
			}
		})
	}
}

func limitAssetAuthApp(t *testing.T, h *LimitAssetHandler, guard *middleware.AuthGuard) (*fiber.App, huma.API) {
	t.Helper()
	problem.Install()
	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	group := app.Group("/v1", func(c fiber.Ctx) error {
		c.SetContext(tmcore.ContextWithTenantID(c.Context(), "verified-tenant"))
		return c.Next()
	})
	api := openapi.New(app, group, openapi.Config{Title: "asset admin", Version: "test"})
	registerTracerHumaRoutes(group, api, tracerHumaHandlers{Guard: guard, LimitAssetAdmin: h, Rule: &Handler{}, Limit: &LimitHandler{}, Validation: &ValidationHandler{}, TransactionValidation: &TransactionValidationHandler{}, AuditEvent: &AuditEventHandler{}})
	return app, api
}

func serveLimitAssetTLS(t *testing.T, app *fiber.App, uri string) (string, *http.Client) {
	t.Helper()
	fixture := testutil.GenerateMTLSFixture(t, uri)
	serverCert, err := tls.X509KeyPair(fixture.ServerCertPEM, fixture.ServerKeyPEM)
	require.NoError(t, err)
	clientCert, err := tls.X509KeyPair(fixture.ClientCertPEM, fixture.ClientKeyPEM)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(fixture.CACertPEM))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serverTLS := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{serverCert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	done := make(chan error, 1)
	go func() {
		done <- app.Listener(tls.NewListener(listener, serverTLS), fiber.ListenConfig{DisableStartupMessage: true})
	}()
	t.Cleanup(func() { require.NoError(t, app.Shutdown()); require.NoError(t, <-done) })
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{clientCert}, RootCAs: roots, ServerName: "localhost"}}
	t.Cleanup(transport.CloseIdleConnections)
	return "https://" + listener.Addr().String(), &http.Client{Transport: transport, Timeout: 5 * time.Second}
}
