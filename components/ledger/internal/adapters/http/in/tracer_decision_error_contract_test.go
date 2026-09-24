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

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	platformhttp "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func TestTracerDecisionErrorContract(t *testing.T) {
	for _, test := range []struct {
		sentinel            error
		code, title, detail string
	}{
		{constant.ErrTransactionReviewRequired, "0526", "Transaction Review Required", "Tracer requires review of this transaction. No accounting operation or pending hold was created."},
		{constant.ErrTransactionReservationDenied, "0177", "Transaction Reservation Denied Error", "The transaction could not be completed because Tracer denied it under the configured rules or usage limits."},
	} {
		t.Run(test.code, func(t *testing.T) {
			app := fiber.New()
			app.Get("/probe", func(c fiber.Ctx) error {
				return platformhttp.WithError(c, pkg.ValidateBusinessError(test.sentinel, constant.EntityTransaction))
			})
			response, err := app.Test(httptest.NewRequest(http.MethodGet, "/probe", nil))
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusUnprocessableEntity, response.StatusCode)
			var body map[string]any
			require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
			require.Equal(t, test.code, body["code"])
			require.Equal(t, test.title, body["title"])
			require.Equal(t, test.detail, body["detail"])
		})
	}
}
