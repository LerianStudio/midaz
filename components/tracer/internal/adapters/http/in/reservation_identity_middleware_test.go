// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/seamidentity"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestReservationIdentityRejectsPlaintextAndForgedHeaders(t *testing.T) {
	t.Parallel()
	resolver, err := seamidentity.NewResolver([]seamidentity.Binding{{URI: "spiffe://example.test/producer", IntegrationID: "producer", AssetNamespace: "assets", Purposes: []seamidentity.Purpose{seamidentity.PurposeReserve}}}, 256)
	require.NoError(t, err)
	for _, tc := range []struct {
		name     string
		resolver *seamidentity.Resolver
		status   int
		code     error
	}{
		{"unverified peer", resolver, http.StatusForbidden, constant.ErrInsufficientPrivileges},
		{"missing configuration", nil, http.StatusServiceUnavailable, constant.ErrContextPolicyUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			called := false
			app.Post("/v1/reservations", NewReservationIdentityMiddleware(tc.resolver), func(c fiber.Ctx) error {
				called = true
				return c.SendStatus(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/reservations", nil)
			req.Header.Set("X-Integration-Id", "producer")
			req.Header.Set("X-Asset-Namespace", "assets")
			req.Header.Set("X-Forwarded-Client-Cert", "spiffe://example.test/producer")
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, tc.status, resp.StatusCode)
			var body struct {
				Code string `json:"code"`
			}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			require.Equal(t, tc.code.Error(), body.Code)
			require.False(t, called)
		})
	}
}
