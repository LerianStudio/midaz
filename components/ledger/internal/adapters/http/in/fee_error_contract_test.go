// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// feePrefixedCodeRegex matches the retired FEE-00xx wire codes that the deleted
// feeshared error fork emitted. After the fork was folded into the canonical
// numeric registry, no fee error response may carry a FEE- code. Every contract
// assertion below verifies the emitted code does NOT match this pattern, so a
// regression that reintroduces the fork (or its sentinels) breaks the lock.
var feePrefixedCodeRegex = regexp.MustCompile(`^FEE-\d{4}$`)

// TestFeeErrorContract_CanonicalCodes locks the post-migration wire contract for
// the fee/billing surfaces: every error that previously carried a FEE-00xx code
// now carries its canonical midaz numeric code, routed to the binding HTTP status
// class (E3/D2) by pkg/net/http.WithError. Each row pins code -> status -> title
// exactly; there is no acceptable-set fallback. The codes were taken from the
// Family 1 migration table (docs/plans/2026-06-07-error-code-migration.md) and
// confirmed against the canonical errorMap in pkg/errors.go at authoring time.
//
// Representative coverage across the migrated categories:
//   - duplicate package         FEE-0018 -> 0184 (Conflict 409)
//   - package range overlap      FEE-0035 -> 0199 (Unprocessable 422, 400->422)
//   - billing route overlap      FEE-0058 -> 0219 (Unprocessable 422, 400->422)
//   - min>max amount             FEE-0015 -> 0182 (Unprocessable 422, 400->422)
//   - calculate-fee failure      FEE-0022 -> 0186 (Internal 500)
//   - billing calculation failed FEE-0060 -> 0221 (Internal 500)
//   - calculation field type     FEE-0005 -> 0179 (ValidationError 400, renamed)
//   - header parameter           FEE-0019 -> 0185 (ValidationError 400, renamed)
//   - segmentID malformed        FEE-0039 -> 0202 (ValidationError 400)
//   - invalid billing period     FEE-0063 -> 0224 (ValidationError 400)
//   - generic not-found          FEE-0012 -> 0007 (NotFound 404, reused)
//   - billing package not found  FEE-0052 -> 0213 (NotFound 404)
//   - billing target not found   FEE-0059 -> 0220 (NotFound 404)
//   - no active billing packages FEE-0061 -> 0222 (NotFound 404)
//   - invalid path parameter     FEE-0016 -> 0065 (ValidationError 400, reused)
func TestFeeErrorContract_CanonicalCodes(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
		expectedTitle  string
	}{
		{
			name:           "duplicate package emits canonical 0184 (409) not FEE-0018",
			err:            pkg.ValidateBusinessError(constant.ErrDuplicatePackage, constant.EntityPackage),
			expectedStatus: fiber.StatusConflict,
			expectedCode:   "0184",
			expectedTitle:  "Package already exists",
		},
		{
			name:           "package range overlap emits canonical 0199 (409) not FEE-0035",
			err:            pkg.ValidateBusinessError(constant.ErrPackageRange, constant.EntityPackage),
			expectedStatus: fiber.StatusConflict,
			expectedCode:   "0199",
			expectedTitle:  "Package amount range overlap",
		},
		{
			name:           "billing route overlap emits canonical 0219 (409) not FEE-0058",
			err:            pkg.ValidateBusinessError(constant.ErrBillingRouteOverlap, constant.EntityBillingPackage),
			expectedStatus: fiber.StatusConflict,
			expectedCode:   "0219",
			expectedTitle:  "Billing route overlap",
		},
		{
			name:           "min>max amount emits canonical 0182 (422) not FEE-0015",
			err:            pkg.ValidateBusinessError(constant.ErrMinAmountGreaterThanMaxAmount, constant.EntityPackage),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0182",
			expectedTitle:  "minimumAmount greater than maximumAmount",
		},
		{
			name:           "calculate-fee failure emits canonical 0186 (500) not FEE-0022",
			err:            pkg.ValidateBusinessError(constant.ErrCalculateFee, constant.EntityFeeCalculation),
			expectedStatus: fiber.StatusInternalServerError,
			expectedCode:   "0186",
			expectedTitle:  "Failed to calculate fee",
		},
		{
			name:           "billing calculation failed emits canonical 0221 (500) not FEE-0060",
			err:            pkg.ValidateBusinessError(constant.ErrBillingCalculationFailed, constant.EntityBillingPackage, "boom"),
			expectedStatus: fiber.StatusInternalServerError,
			expectedCode:   "0221",
			expectedTitle:  "Billing calculation failed",
		},
		{
			name:           "calculation field type emits canonical 0179 (400) not FEE-0005",
			err:            pkg.ValidateBusinessError(constant.ErrFeeCalculationFieldType, constant.EntityFeeCalculation),
			expectedStatus: fiber.StatusBadRequest,
			expectedCode:   "0179",
			expectedTitle:  "Calculation field type invalid",
		},
		{
			name:           "header parameter emits canonical 0185 (400) not FEE-0019",
			err:            pkg.ValidateBusinessError(constant.ErrFeeInvalidHeaderParameter, constant.EntityPackage, "organizationId"),
			expectedStatus: fiber.StatusBadRequest,
			expectedCode:   "0185",
			expectedTitle:  "Invalid header parameter",
		},
		{
			name:           "segmentID malformed emits canonical 0202 (400) not FEE-0039",
			err:            pkg.ValidateBusinessError(constant.ErrInvalidSegmentID, constant.EntityPackage),
			expectedStatus: fiber.StatusBadRequest,
			expectedCode:   "0202",
			expectedTitle:  "Invalid segmentID",
		},
		{
			name:           "invalid billing period emits canonical 0224 (400) not FEE-0063",
			err:            pkg.ValidateBusinessError(constant.ErrInvalidBillingPeriod, constant.EntityBillingPackage),
			expectedStatus: fiber.StatusBadRequest,
			expectedCode:   "0224",
			expectedTitle:  "Invalid billing period",
		},
		{
			name:           "generic entity not found emits canonical 0007 (404) not FEE-0012",
			err:            pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityPackage),
			expectedStatus: fiber.StatusNotFound,
			expectedCode:   "0007",
			expectedTitle:  "Entity Not Found",
		},
		{
			name:           "billing package not found emits canonical 0213 (404) not FEE-0052",
			err:            pkg.ValidateBusinessError(constant.ErrBillingPackageNotFound, constant.EntityBillingPackage, "abc"),
			expectedStatus: fiber.StatusNotFound,
			expectedCode:   "0213",
			expectedTitle:  "Billing package not found",
		},
		{
			name:           "billing target account not found emits canonical 0220 (404) not FEE-0059",
			err:            pkg.ValidateBusinessError(constant.ErrTargetAccountNotFound, constant.EntityBillingPackage, "alias"),
			expectedStatus: fiber.StatusNotFound,
			expectedCode:   "0220",
			expectedTitle:  "Target account not found",
		},
		{
			name:           "no active billing packages emits canonical 0222 (404) not FEE-0061",
			err:            pkg.ValidateBusinessError(constant.ErrNoActiveBillingPackages, constant.EntityBillingPackage),
			expectedStatus: fiber.StatusNotFound,
			expectedCode:   "0222",
			expectedTitle:  "No active billing packages",
		},
		{
			name:           "invalid path parameter emits canonical 0065 (400) not FEE-0016",
			err:            pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityPackage, "id"),
			expectedStatus: fiber.StatusBadRequest,
			expectedCode:   "0065",
			expectedTitle:  "Invalid Path Parameter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capturedErr := tt.err

			app := fiber.New()
			app.Get("/probe", func(c *fiber.Ctx) error {
				return http.WithError(c, capturedErr)
			})

			req := httptest.NewRequest("GET", "/probe", nil)

			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode,
				"canonical code must route to its binding HTTP status class")

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var errResp map[string]any
			require.NoError(t, json.Unmarshal(body, &errResp))

			code, ok := errResp["code"].(string)
			require.True(t, ok, "error response must carry a string code field, got: %s", string(body))

			assert.Equal(t, tt.expectedCode, code,
				"path must emit the exact canonical midaz code (no FEE-00xx fork rewrite)")
			assert.NotRegexp(t, feePrefixedCodeRegex, code,
				"response code must NOT be a retired FEE-00xx fork code")
			assert.Equal(t, tt.expectedTitle, errResp["title"],
				"error title is part of the wire contract and must not drift")
		})
	}
}
