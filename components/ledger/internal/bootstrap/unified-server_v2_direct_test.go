// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"bytes"
	"encoding/json"
	"io"
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

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"tokenless v2 direct request must be blocked by the transactions:post auth chain")
}

// TestNewUnifiedServer_V2DirectRouteReachesRealHandler proves the mounted route→terminal
// composition end-to-end: an AUTHENTICATED (auth disabled) POST to the CONCRETE /v2
// direct path traverses the full Fiber middleware chain (auth passthrough + tenant
// post-auth + ParseUUIDPathParameters) and dispatches to the REAL handler, which decodes
// the flat v2 body. An empty `{}` body is missing the required asset/amount/from/to
// fields, so http.DecodeAndValidate rejects it with the canonical 400 RFC 9457
// problem+json (never a panic, never the removed 501 stub). This exercises the seam the
// in-memory handler unit test cannot: the Fiber chain and the Huma terminal are wired to
// the SAME path and hand off correctly. The committed money path is the Task 1.3.3
// integration+parity test.
func TestNewUnifiedServer_V2DirectRouteReachesRealHandler(t *testing.T) {
	t.Parallel()

	server := newV2DirectServer(t, &middleware.AuthClient{Enabled: false})

	const concretePath = "/v2/organizations/00000000-0000-0000-0000-000000000001/ledgers/00000000-0000-0000-0000-000000000002/transactions/direct"

	// Empty body + Content-Type: Huma's request parse succeeds (RawBody), dispatch
	// reaches the real handler, and DecodeAndValidate rejects the missing required
	// fields with the canonical 400.
	req, err := http.NewRequest(http.MethodPost, concretePath, bytes.NewReader([]byte("{}")))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")

	resp, err := server.app.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"authenticated v2 direct request must reach the real handler and 400 on the empty body through the mounted chain")
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/problem+json",
		"the handler must serialize the RFC 9457 problem+json envelope")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var problem map[string]any
	require.NoError(t, json.Unmarshal(body, &problem), "body should decode as RFC 9457 JSON")
	assert.Equal(t, float64(http.StatusBadRequest), problem["status"],
		"RFC 9457 envelope should carry status:400")
}

// TestNewUnifiedServer_V2SpecNotServedWhenDocsDisabled asserts the swaggerEnabled
// NEGATIVE gate for v2: with LEDGER_HUMA_DOCS_ENABLED off, GET /v2/openapi.json is
// NOT served (404). Every other v2-spec test enables the flag; this covers the
// gated-off branch.
func TestNewUnifiedServer_V2SpecNotServedWhenDocsDisabled(t *testing.T) {
	// Explicitly disable the docs gate. t.Setenv precludes t.Parallel here.
	t.Setenv("LEDGER_HUMA_DOCS_ENABLED", "false")

	server := newV2DirectServer(t, &middleware.AuthClient{Enabled: false})

	req, err := http.NewRequest(http.MethodGet, "/v2/openapi.json", nil)
	require.NoError(t, err)

	resp, err := server.app.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"v2 openapi.json must not be served when LEDGER_HUMA_DOCS_ENABLED is off")
}

// keysOf returns the keys of a decoded JSON object for diagnostic messages.
func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}
