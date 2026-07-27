// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"net/http"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
)

// directOpV2Path is the GROUP-RELATIVE op path the v2 `direct` operation registers
// under (the /v2 prefix rides the OpenAPI servers entry, so it is absent here).
const directOpV2Path = "/organizations/{organization_id}/ledgers/{ledger_id}/transactions/direct"

// newV2DirectServer builds a unified server whose /v2 contract mounts ONLY the
// `direct` transaction op via the production seam httpin.RegisterTransactionV2RoutesToApp,
// using the supplied auth client. The /v1 contract is left unmounted (nil humaMount):
// this test isolates the v2 direct route + its Fiber auth chain. A zero-value
// TransactionHandler is safe because registration never invokes the handler.
func newV2DirectServer(t *testing.T, auth *middleware.AuthClient) *UnifiedServer {
	t.Helper()

	logger := newTestLogger()
	telemetry := &libOpentelemetry.Telemetry{}

	humaMountV2 := func(group fiber.Router, api huma.API) {
		httpin.RegisterTransactionV2RoutesToApp(group, api, auth, &httpin.TransactionHandler{}, nil)
	}

	server := NewUnifiedServer(":0", "test-version", logger, telemetry, nil, nil, humaMountV2)
	require.NotNil(t, server, "NewUnifiedServer should return a non-nil server")
	require.NotNil(t, server.app, "server should hold a Fiber app")

	return server
}

// TestNewUnifiedServer_V2DirectRouteInContract asserts Task 1.1.2 (a): the `direct`
// operation is registered on the INDEPENDENT v2 contract and surfaces in
// /v2/openapi.json with OperationID createTransactionDirectV2, method POST, at the
// group-relative direct path.
func TestNewUnifiedServer_V2DirectRouteInContract(t *testing.T) {
	// ServeSpec is gated on LEDGER_HUMA_DOCS_ENABLED; enable it so /v2/openapi.json
	// is mounted. t.Setenv precludes t.Parallel here.
	t.Setenv("LEDGER_HUMA_DOCS_ENABLED", "true")

	server := newV2DirectServer(t, &middleware.AuthClient{Enabled: false})

	v2doc := fetchOpenAPISpec(t, server.app, "/v2/openapi.json")

	paths, ok := v2doc["paths"].(map[string]any)
	require.True(t, ok, "v2 spec should carry a paths object")

	pathItem, ok := paths[directOpV2Path].(map[string]any)
	require.Truef(t, ok, "v2 spec should register the direct op path %q; paths=%v", directOpV2Path, keysOf(paths))

	post, ok := pathItem["post"].(map[string]any)
	require.True(t, ok, "direct op path should carry a POST operation")

	assert.Equal(t, "createTransactionDirectV2", post["operationId"],
		"direct op should advertise OperationID createTransactionDirectV2")
}

// TestNewUnifiedServer_V2DirectRouteRequiresAuth asserts Task 1.1.2 (b): the v2
// direct route is guarded by the SAME protected chain as v1 (protectedMidaz,
// transactions:post). With auth enabled and no bearer token, the request is
// rejected before reaching the handler — never public.
func TestNewUnifiedServer_V2DirectRouteRequiresAuth(t *testing.T) {
	// Address must be non-empty so Authorize enforces the token check (it is never
	// dialed: a missing token short-circuits with 401 first).
	server := newV2DirectServer(t, &middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"})

	const concretePath = "/v2/organizations/00000000-0000-0000-0000-000000000001/ledgers/00000000-0000-0000-0000-000000000002/transactions/direct"

	req, err := http.NewRequest(http.MethodPost, concretePath, nil)
	require.NoError(t, err)

	resp, err := server.app.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, resp.StatusCode,
		"tokenless v2 direct request must be blocked by the transactions:post auth chain")
}

// keysOf returns the keys of a decoded JSON object for diagnostic messages.
func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}
