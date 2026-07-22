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

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// trcCodeRegex matches the retired TRC-xxxx prefixed wire codes. After the
// Family-3 migration NO tracer error response may carry a TRC- code — every
// surface emits the canonical numeric registry code. A response matching this
// pattern proves the fork (or a regression of it) is back.
var trcCodeRegex = regexp.MustCompile(`^TRC-\d{4}$`)

// TestTracerErrorContract locks the post-migration wire contract for the tracer
// component: each representative canonical sentinel rendered through the
// canonical http.WithError envelope must surface its exact numeric code, the
// binding HTTP status from the migration table (Family 3), and the registry
// title — never a retired TRC- code. The cases span the breadth of the tracer
// taxonomy: generic input/path/query errors, rule lifecycle (invalid
// transition), limit CRUD (not-found / conflict / immutable), reservation
// not-found, timeout/cancelled (503), and the readyz probe codes.
//
// This is a code -> status -> title lock, modeled on ledger's
// crm_error_contract_test.go. It exercises WithError directly with a constructed
// canonical error so it is independent of any specific handler wiring.
func TestTracerErrorContract(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
		expectedTitle  string
	}{
		// --- generic input / path / query (400) ---
		{
			name:           "invalid path parameter -> 0065 / 400",
			err:            pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityRule, "id"),
			expectedStatus: 400,
			expectedCode:   "0065",
			expectedTitle:  "Invalid Path Parameter",
		},
		{
			name:           "invalid query parameter -> 0082 / 400",
			err:            pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityRule, "status"),
			expectedStatus: 400,
			expectedCode:   "0082",
			expectedTitle:  "Invalid Query Parameter",
		},
		{
			name:           "invalid cursor -> 0333 / 400",
			err:            pkg.ValidateBusinessError(constant.ErrInvalidCursor, constant.EntityRule),
			expectedStatus: 400,
			expectedCode:   "0333",
			expectedTitle:  "Invalid Cursor",
		},
		{
			name:           "invalid sort column -> 0332 / 400",
			err:            pkg.ValidateBusinessError(constant.ErrInvalidSortColumn, constant.EntityRule),
			expectedStatus: 400,
			expectedCode:   "0332",
			expectedTitle:  "Invalid Sort Column",
		},
		// --- rule lifecycle ---
		{
			name:           "rule not found -> 0347 / 404",
			err:            pkg.ValidateBusinessError(constant.ErrRuleNotFound, constant.EntityRule),
			expectedStatus: 404,
			expectedCode:   "0347",
			expectedTitle:  "Rule Not Found",
		},
		{
			name:           "rule name already exists -> 0348 / 409",
			err:            pkg.ValidateBusinessError(constant.ErrRuleNameAlreadyExists, constant.EntityRule),
			expectedStatus: 409,
			expectedCode:   "0348",
			expectedTitle:  "Rule Name Already Exists",
		},
		{
			name:           "rule invalid status transition -> 0349 / 422",
			err:            pkg.ValidateBusinessError(constant.ErrRuleInvalidStatus, constant.EntityRule),
			expectedStatus: 422,
			expectedCode:   "0349",
			expectedTitle:  "Rule Invalid Status",
		},
		{
			name:           "expression not modifiable -> 0351 / 422",
			err:            pkg.ValidateBusinessError(constant.ErrExpressionNotModifiable, constant.EntityRule),
			expectedStatus: 422,
			expectedCode:   "0351",
			expectedTitle:  "Expression Not Modifiable",
		},
		{
			name:           "expression cost exceeded -> 0342 / 422",
			err:            pkg.ValidateBusinessError(constant.ErrExpressionCostExceeded, constant.EntityRule),
			expectedStatus: 422,
			expectedCode:   "0342",
			expectedTitle:  "Expression Cost Exceeded",
		},
		// --- limit CRUD ---
		{
			name:           "limit not found -> 0362 / 404",
			err:            pkg.ValidateBusinessError(constant.ErrLimitNotFound, constant.EntityLimit),
			expectedStatus: 404,
			expectedCode:   "0362",
			expectedTitle:  "Limit Not Found",
		},
		{
			name:           "limit name already exists -> 0442 / 409",
			err:            pkg.ValidateBusinessError(constant.ErrLimitNameAlreadyExists, constant.EntityLimit),
			expectedStatus: 409,
			expectedCode:   "0442",
			expectedTitle:  "Limit Name Already Exists",
		},
		{
			name:           "limit immutable field -> 0380 / 422",
			err:            pkg.ValidateBusinessError(constant.ErrLimitImmutableField, constant.EntityLimit),
			expectedStatus: 422,
			expectedCode:   "0380",
			expectedTitle:  "Limit Immutable Field",
		},
		{
			name:           "limit already deleted -> 0370 / 422",
			err:            pkg.ValidateBusinessError(constant.ErrLimitAlreadyDeleted, constant.EntityLimit),
			expectedStatus: 422,
			expectedCode:   "0370",
			expectedTitle:  "Limit Already Deleted",
		},
		// --- reservation ---
		{
			name:           "reservation not found -> 0482 / 404",
			err:            pkg.ValidateBusinessError(constant.ErrReservationNotFound, constant.EntityReservation),
			expectedStatus: 404,
			expectedCode:   "0482",
			expectedTitle:  "Reservation Not Found",
		},
		{
			name:           "reservation already terminal -> 0483 / 422",
			err:            pkg.ValidateBusinessError(constant.ErrReservationAlreadyTerminal, constant.EntityReservation),
			expectedStatus: 422,
			expectedCode:   "0483",
			expectedTitle:  "Reservation Already Terminal",
		},
		// --- audit / transaction validation ---
		{
			name:           "audit event not found -> 0381 / 404",
			err:            pkg.ValidateBusinessError(constant.ErrAuditEventNotFound, constant.EntityAuditEvent),
			expectedStatus: 404,
			expectedCode:   "0381",
			expectedTitle:  "Audit Event Not Found",
		},
		{
			name:           "transaction validation not found -> 0432 / 404",
			err:            pkg.ValidateBusinessError(constant.ErrTransactionValidationNotFound, constant.EntityTransactionValidation),
			expectedStatus: 404,
			expectedCode:   "0432",
			expectedTitle:  "Transaction Validation Not Found",
		},
		// --- timeout (504) / cancelled (503) ---
		{
			name:           "validation timeout -> 0422 / 504",
			err:            pkg.ValidateBusinessError(constant.ErrValidationTimeout, constant.EntityValidationRequest),
			expectedStatus: 504,
			expectedCode:   "0422",
			expectedTitle:  "Gateway Timeout", // >=500: title scrubbed to status text (RFC 9457) == "Gateway Timeout"
		},
		{
			name:           "list validations timeout -> 0433 / 504",
			err:            pkg.ValidateBusinessError(constant.ErrListValidationsTimeout, constant.EntityTransactionValidation),
			expectedStatus: 504,
			expectedCode:   "0433",
			expectedTitle:  "Gateway Timeout", // >=500: title scrubbed to status text (RFC 9457) == "Gateway Timeout"
		},
		{
			name:           "context cancelled -> 0330 / 503",
			err:            pkg.ValidateBusinessError(constant.ErrContextCancelled, constant.EntityRule),
			expectedStatus: 503,
			expectedCode:   "0330",
			expectedTitle:  "Service Unavailable", // >=500: title scrubbed to status text (RFC 9457)
		},
		// --- readyz probe codes (503) ---
		{
			name:           "readyz dependencies unhealthy -> 0462 / 503",
			err:            pkg.ValidateBusinessError(constant.ErrReadyzDependenciesUnhealthy, constant.EntityRule),
			expectedStatus: 503,
			expectedCode:   "0462",
			expectedTitle:  "Service Unavailable", // >=500: title scrubbed to status text (RFC 9457)
		},
		{
			name:           "rule cache not ready -> 0437 / 503",
			err:            pkg.ValidateBusinessError(constant.ErrRuleCacheNotReady, constant.EntityRule),
			expectedStatus: 503,
			expectedCode:   "0437",
			expectedTitle:  "Service Unavailable", // >=500: title scrubbed to status text (RFC 9457)
		},
		// --- auth (401) ---
		{
			name:           "unauthorized missing sub -> 0474 / 401",
			err:            pkg.ValidateBusinessError(constant.ErrUnauthorizedMissingSub, constant.EntityRule),
			expectedStatus: 401,
			expectedCode:   "0474",
			expectedTitle:  "Unauthorized Missing Sub",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/probe", func(c *fiber.Ctx) error {
				return http.WithError(c, tt.err)
			})

			req := httptest.NewRequest("GET", "/probe", nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "status must match the migration-table binding class")

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var envelope map[string]any

			require.NoError(t, json.Unmarshal(body, &envelope), "response must be the RFC 9457 {code,status,title,detail} envelope: %s", string(body))

			code, ok := envelope["code"].(string)
			require.True(t, ok, "envelope must carry a string code, got: %s", string(body))

			assert.Equal(t, tt.expectedCode, code, "must emit the exact canonical numeric code")
			assert.NotRegexp(t, trcCodeRegex, code, "response code must NOT be a retired TRC- fork code")
			assert.Equal(t, tt.expectedTitle, envelope["title"], "must emit the registry title (>=500 scrubs to status text)")
			assert.NotEmpty(t, envelope["detail"], "envelope must carry a non-empty detail (<500 registry message; >=500 \"internal error\")")
		})
	}
}
