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

// directOpV2Path is the key the v2 `direct` operation registers under in the single
// OpenAPI document. The /v2 prefix lives IN the key — the Huma Group writes it into
// op.Path — so the path is fully qualified here, not group-relative. It names no
// organization and no ledger: a v2 create is scoped by its request body.
const directOpV2Path = "/v2/transactions/direct"

// newV2DirectServer builds a unified server that mounts ONLY the `direct` transaction
// op under the /v2 prefix via the production seam httpin.RegisterTransactionV2RoutesToApp,
// using the supplied auth client. The v1 registrar is nil, so a single contract still
// backs the one root document — this is the one-registrar case that proves
// mountHumaContracts builds the root document whenever ANY version has a registrar. A
// zero-value TransactionHandler is safe because registration never invokes the handler.
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

// newNoContractServer builds a unified server with NO Huma registrars: both the v1
// and v2 mounts are nil. mountHumaContracts short-circuits before assembling any
// contract, so no OpenAPI document exists and no spec route is served — the all-nil
// counterpart to newV2DirectServer's one-registrar case.
func newNoContractServer(t *testing.T) *UnifiedServer {
	t.Helper()

	logger := newTestLogger()
	telemetry := &libOpentelemetry.Telemetry{}

	server := NewUnifiedServer(":0", "test-version", logger, telemetry, nil, nil, nil)
	require.NotNil(t, server, "NewUnifiedServer should return a non-nil server")
	require.NotNil(t, server.app, "server should hold a Fiber app")

	return server
}

// TestNewUnifiedServer_V2DirectRouteInContract asserts the `direct` operation is
// registered on the ONE root document under its fully-qualified /v2 key, with
// OperationID createTransactionDirectV2 and method POST. The one-registrar subtest
// (v1 nil, v2 mounted) proves mountHumaContracts still builds the root document from a
// single contract; the both-nil subtest proves it builds no document at all — even
// with the docs gate ON.
func TestNewUnifiedServer_V2DirectRouteInContract(t *testing.T) {
	// ServeSpec is gated on OPENAPI_DOCS_ENABLED; enable it so /openapi.json is mounted
	// when a document exists. t.Setenv precludes t.Parallel here.
	t.Setenv("OPENAPI_DOCS_ENABLED", "true")

	t.Run("one registrar builds the root document", func(t *testing.T) {
		server := newV2DirectServer(t, &middleware.AuthClient{Enabled: false})

		doc := fetchOpenAPISpec(t, server.app, "/openapi.json")

		paths, ok := doc["paths"].(map[string]any)
		require.True(t, ok, "the single spec should carry a paths object")

		pathItem, ok := paths[directOpV2Path].(map[string]any)
		require.Truef(t, ok, "the single document should register the direct op key %q; paths=%v", directOpV2Path, keysOf(paths))

		post, ok := pathItem["post"].(map[string]any)
		require.True(t, ok, "direct op key should carry a POST operation")

		assert.Equal(t, "createTransactionDirectV2", post["operationId"],
			"direct op should advertise OperationID createTransactionDirectV2")
	})

	t.Run("both registrars nil builds no document", func(t *testing.T) {
		server := newNoContractServer(t)

		req, err := http.NewRequest(http.MethodGet, "/openapi.json", nil)
		require.NoError(t, err)

		resp, err := server.app.Test(req)
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode,
			"with both registrars nil there is no document, so /openapi.json 404s even with the gate ON")
	})
}

// TestNewUnifiedServer_V2DirectRouteRequiresAuth asserts the v2
// direct route is guarded by the SAME protected chain as v1 (protectedMidaz,
// transactions:post). With auth enabled and no bearer token, the request is
// rejected before reaching the handler — never public.
func TestNewUnifiedServer_V2DirectRouteRequiresAuth(t *testing.T) {
	// Address must be non-empty so Authorize enforces the token check (it is never
	// dialed: a missing token short-circuits with 401 first).
	server := newV2DirectServer(t, &middleware.AuthClient{Enabled: true, Address: "http://auth.invalid"})

	const concretePath = "/v2/transactions/direct"

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
// post-auth + the body-size guard) and dispatches to the REAL handler, which decodes
// the flat v2 body. An empty `{}` body is missing the required asset/amount/from/to
// fields, so http.DecodeAndValidate rejects it with the canonical 400 RFC 9457
// problem+json (never a panic, never the removed 501 stub). This exercises the seam the
// in-memory handler unit test cannot: the Fiber chain and the Huma terminal are wired to
// the SAME path and hand off correctly. The committed money path is the
// integration+parity test.
func TestNewUnifiedServer_V2DirectRouteReachesRealHandler(t *testing.T) {
	t.Parallel()

	server := newV2DirectServer(t, &middleware.AuthClient{Enabled: false})

	const concretePath = "/v2/transactions/direct"

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

// TestNewUnifiedServer_V2SpecNotServedWhenDocsDisabled asserts the openAPIDocsEnabled
// NEGATIVE gate on the parse branch: with OPENAPI_DOCS_ENABLED explicitly "false", a
// document is assembled but ServeSpec never runs, so GET /openapi.json is NOT served
// (404). This covers the explicit-false branch; the gate-absent deploy posture is
// covered separately by TestNewUnifiedServer_SpecNotServedWhenDocsGateUnset.
func TestNewUnifiedServer_V2SpecNotServedWhenDocsDisabled(t *testing.T) {
	// Explicitly disable the docs gate. t.Setenv precludes t.Parallel here.
	t.Setenv("OPENAPI_DOCS_ENABLED", "false")

	server := newV2DirectServer(t, &middleware.AuthClient{Enabled: false})

	req, err := http.NewRequest(http.MethodGet, "/openapi.json", nil)
	require.NoError(t, err)

	resp, err := server.app.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"openapi.json must not be served when OPENAPI_DOCS_ENABLED is explicitly false")
}

// keysOf returns the keys of a decoded JSON object for diagnostic messages.
func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}
