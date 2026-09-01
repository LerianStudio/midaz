// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// TestAuthz_FeeResources_AuthorizeUnderMidazAppName pins the authz application slug of
// every fee and billing route to the ledger core's own "midaz" slug (midazName). Fee and
// billing are a product embedded in the ledger V4 binary, and BOLA "one identity, one
// slug" in the declaration receiver (plugin-identity :4001) forbids a route serving a
// second slug behind the same identity.
//
// It is TestAuthz_RoutingResources_AuthorizeUnderMidazAppName for the fee surface: the
// four PRODUCTION registrars are driven through a capturing authz server that records
// the forwarded product, so a registrar repointed at a fees-specific slug makes the
// recorded product diverge. All twelve ops are swept, including the two single-verb
// compute endpoints the five-verb table in routes_test.go cannot express.
//
// The capturing server always denies (authorized=false). The auth middleware forwards
// the product to the authz service BEFORE reading the decision, so the product is
// captured on every request while the 403 short-circuits the chain — the business
// terminals never run, so zero-value handlers are sufficient.
//
// NOT parallel: libProblem.Install swaps a process-global huma.NewError hook and Huma
// validation uses process-global sync.Pools; concurrent builds cross-contaminate.
func TestAuthz_FeeResources_AuthorizeUnderMidazAppName(t *testing.T) {
	orgID := uuid.New()
	ledgerID := uuid.New()
	resourceID := uuid.New()
	base := "/v2/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String()

	var product string

	srv := newAuthzProductCapture(t, &product)
	defer srv.Close()

	auth := &middleware.AuthClient{Address: srv.URL, Enabled: true}

	f := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})

	// Mirror production: the ledger registers ErrorEnvelope on the app root.
	f.Use(ledgerMiddleware.ErrorEnvelope())

	mountFeesV2Routes(f, auth, nil)

	token := "Bearer " + guardBearerToken(t)

	ops := []struct {
		method string
		path   string
	}{
		{fiber.MethodPost, base + "/packages"},
		{fiber.MethodGet, base + "/packages"},
		{fiber.MethodGet, base + "/packages/" + resourceID.String()},
		{fiber.MethodPatch, base + "/packages/" + resourceID.String()},
		{fiber.MethodDelete, base + "/packages/" + resourceID.String()},
		{fiber.MethodPost, base + "/estimates"},
		{fiber.MethodPost, base + "/billing-packages"},
		{fiber.MethodGet, base + "/billing-packages"},
		{fiber.MethodGet, base + "/billing-packages/" + resourceID.String()},
		{fiber.MethodPatch, base + "/billing-packages/" + resourceID.String()},
		{fiber.MethodDelete, base + "/billing-packages/" + resourceID.String()},
		{fiber.MethodPost, base + "/billing/calculate"},
	}

	require.Len(t, ops, len(feesV2FullRoutes), "the sweep must cover every mounted fee route")

	for _, op := range ops {
		t.Run(op.method+" "+op.path, func(t *testing.T) {
			product = ""

			req := httptest.NewRequest(op.method, op.path, nil)
			req.Header.Set("Authorization", token)

			resp, err := f.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			require.Equalf(t, fiber.StatusForbidden, resp.StatusCode,
				"%s %s must reach auth and be denied", op.method, op.path)
			assert.Equalf(t, midazName, product,
				"%s %s must authorize under the midaz appName, got %q", op.method, op.path, product)
		})
	}
}
