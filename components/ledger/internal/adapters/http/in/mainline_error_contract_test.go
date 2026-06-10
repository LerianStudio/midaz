// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMainlineErrorContract_ReclassifiedCodes locks the wire contract for the 26
// mainline error codes whose HTTP status class was re-typed in the four-family
// consolidation (Task 3.6, docs/plans/2026-06-07-error-code-migration.md
// "Mainline 400 reclassification"). Each row pins code -> HTTP status -> title,
// driven end-to-end through pkg/net/http.WithError so the typed struct class
// (ValidationError-400, UnprocessableOperationError-422, EntityConflictError-409)
// is exercised exactly as production routes it. A future silent re-type of any of
// these arms in pkg/errors.go's errorMap breaks this lock.
//
// Coverage:
//   - 23 codes 400 -> 422 (ValidationError -> UnprocessableOperationError)
//   - 1 code 400 -> 409  (ValidationError -> EntityConflictError): 0156
//   - 2 reverse fixes -> 400 (ValidationError): 0017 (was 409), 0096 (was 500)
func TestMainlineErrorContract_ReclassifiedCodes(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
		expectedTitle  string
	}{
		// --- 23 move-422 (ValidationError -> UnprocessableOperationError) ---
		{
			name:           "0008 action not permitted is 422",
			err:            pkg.ValidateBusinessError(constant.ErrActionNotPermitted, constant.EntityLedger),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0008",
			expectedTitle:  "Action Not Permitted",
		},
		{
			name:           "0010 account type immutable is 422",
			err:            pkg.ValidateBusinessError(constant.ErrAccountTypeImmutable, constant.EntityAccount),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0010",
			expectedTitle:  "Account Type Immutable",
		},
		{
			name:           "0011 inactive account type is 422",
			err:            pkg.ValidateBusinessError(constant.ErrInactiveAccountType, constant.EntityAccount),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0011",
			expectedTitle:  "Inactive Account Type Error",
		},
		{
			name:           "0012 account balance deletion is 422",
			err:            pkg.ValidateBusinessError(constant.ErrAccountBalanceDeletion, constant.EntityAccount),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0012",
			expectedTitle:  "Account Balance Deletion Error",
		},
		{
			name:           "0013 resource already deleted is 422",
			err:            pkg.ValidateBusinessError(constant.ErrResourceAlreadyDeleted, constant.EntityAccount),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0013",
			expectedTitle:  "Resource Already Deleted",
		},
		{
			name:           "0014 segment ID inactive is 422",
			err:            pkg.ValidateBusinessError(constant.ErrSegmentIDInactive, constant.EntitySegment),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0014",
			expectedTitle:  "Segment ID Inactive",
		},
		{
			name:           "0024 account status transaction restriction is 422",
			err:            pkg.ValidateBusinessError(constant.ErrAccountStatusTransactionRestriction, constant.EntityTransaction),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0024",
			expectedTitle:  "Account Status Transaction Restriction",
		},
		{
			name:           "0025 insufficient account balance is 422",
			err:            pkg.ValidateBusinessError(constant.ErrInsufficientAccountBalance, constant.EntityBalance, "acc"),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0025",
			expectedTitle:  "Insufficient Account Balance Error",
		},
		{
			name:           "0026 transaction method restriction is 422",
			err:            pkg.ValidateBusinessError(constant.ErrTransactionMethodRestriction, constant.EntityTransaction, "x", "x"),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0026",
			expectedTitle:  "Transaction Method Restriction",
		},
		{
			name:           "0030 mismatched asset code is 422",
			err:            pkg.ValidateBusinessError(constant.ErrMismatchedAssetCode, constant.EntityAccount),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0030",
			expectedTitle:  "Mismatched Asset Code",
		},
		{
			name:           "0073 transaction value mismatch is 422",
			err:            pkg.ValidateBusinessError(constant.ErrTransactionValueMismatch, constant.EntityTransaction),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0073",
			expectedTitle:  "Transaction Value Mismatch",
		},
		{
			name:           "0074 forbidden external account manipulation is 422",
			err:            pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, constant.EntityAccount),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0074",
			expectedTitle:  "External Account Modification Prohibited",
		},
		{
			name:           "0086 lock version account balance is 422",
			err:            pkg.ValidateBusinessError(constant.ErrLockVersionAccountBalance, constant.EntityBalance),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0086",
			expectedTitle:  "Race condition detected",
		},
		{
			name:           "0087 transaction already has parent revert is 409",
			err:            pkg.ValidateBusinessError(constant.ErrTransactionIDHasAlreadyParentTransaction, constant.EntityTransaction),
			expectedStatus: fiber.StatusConflict,
			expectedCode:   "0087",
			expectedTitle:  "Transaction Revert already exist",
		},
		{
			name:           "0088 transaction is already a revert is 409",
			err:            pkg.ValidateBusinessError(constant.ErrTransactionIDIsAlreadyARevert, constant.EntityTransaction),
			expectedStatus: fiber.StatusConflict,
			expectedCode:   "0088",
			expectedTitle:  "Transaction is already a reversal",
		},
		{
			name:           "0089 transaction can't be reverted is 422",
			err:            pkg.ValidateBusinessError(constant.ErrTransactionCantRevert, constant.EntityTransaction),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0089",
			expectedTitle:  "Transaction can't be reverted",
		},
		{
			name:           "0090 transaction ambiguous account is 422",
			err:            pkg.ValidateBusinessError(constant.ErrTransactionAmbiguous, constant.EntityTransaction),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0090",
			expectedTitle:  "Transaction ambiguous account",
		},
		{
			name:           "0091 parent ID same as ID is 422",
			err:            pkg.ValidateBusinessError(constant.ErrParentIDSameID, constant.EntityAccount),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0091",
			expectedTitle:  "ID cannot be used as the parent ID",
		},
		{
			name:           "0093 balances can't be deleted is 409",
			err:            pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, constant.EntityBalance),
			expectedStatus: fiber.StatusConflict,
			expectedCode:   "0093",
			expectedTitle:  "Balance cannot be deleted",
		},
		{
			name:           "0124 additional balance not allowed is 422",
			err:            pkg.ValidateBusinessError(constant.ErrAdditionalBalanceNotAllowed, constant.EntityBalance),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0124",
			expectedTitle:  "Additional Balance Creation Not Allowed",
		},
		{
			name:           "0135 metadata index limit exceeded is 422",
			err:            pkg.ValidateBusinessError(constant.ErrMetadataIndexLimitExceeded, constant.EntityAccount),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0135",
			expectedTitle:  "Metadata Index Limit Exceeded",
		},
		{
			name:           "0137 metadata index deletion forbidden is 422",
			err:            pkg.ValidateBusinessError(constant.ErrMetadataIndexDeletionForbidden, constant.EntityAccount),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0137",
			expectedTitle:  "Metadata Index Deletion Forbidden",
		},
		{
			name:           "0170 reserved balance key is 422",
			err:            pkg.ValidateBusinessError(constant.ErrReservedBalanceKey, constant.EntityBalance, "key"),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0170",
			expectedTitle:  "Reserved Balance Key Error",
		},
		// --- 1 move-409 (ValidationError -> EntityConflictError) ---
		{
			name:           "0156 duplicate action route is 409",
			err:            pkg.ValidateBusinessError(constant.ErrDuplicateActionRoute, constant.EntityOperationRoute, "r", "a"),
			expectedStatus: fiber.StatusConflict,
			expectedCode:   "0156",
			expectedTitle:  "Duplicate Action Route",
		},
		// --- 2 reverse fixes -> 400 (ValidationError) ---
		{
			name:           "0017 invalid script format is 400 (reverse fix from 409)",
			err:            pkg.ValidateBusinessError(constant.ErrInvalidScriptFormat, constant.EntityTransaction),
			expectedStatus: fiber.StatusBadRequest,
			expectedCode:   "0017",
			expectedTitle:  "Invalid Script Format Error",
		},
		{
			name:           "0096 account alias invalid is 400 (reverse fix from 500)",
			err:            pkg.ValidateBusinessError(constant.ErrAccountAliasInvalid, constant.EntityAccount),
			expectedStatus: fiber.StatusBadRequest,
			expectedCode:   "0096",
			expectedTitle:  "Invalid Account Alias",
		},
	}

	require.Len(t, tests, 26, "the reclassification table is 26 codes (23 move-422 + 1 move-409 + 2 reverse fixes)")

	runErrorContractCases(t, tests)
}

// TestMainlineErrorContract_DependencyFaultCodes locks the three error-platform
// follow-up reclassifications (E5, E9, docs/plans/2026-06-07-v4-error-status-migration-notes.md
// "Error-platform follow-up reclassifications"): codes whose typed struct
// disagreed with the server-fault vs client-error class their own message describes.
//   - 0228 500 -> 503 (InternalServerError -> ServiceUnavailableError)
//   - 0231 400 -> 500 (ValidationError -> FailedPreconditionError, routed to 500)
//   - 0178 422 -> 503 (UnprocessableOperationError -> ServiceUnavailableError)
//
// A silent re-type of any of these arms in pkg/errors.go's errorMap breaks this lock.
func TestMainlineErrorContract_DependencyFaultCodes(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
		expectedTitle  string
	}{
		{
			name:           "0228 midaz query failed is 503 (dependency unavailable)",
			err:            pkg.ValidateBusinessError(constant.ErrMidazQueryFailed, constant.EntityTransaction),
			expectedStatus: fiber.StatusServiceUnavailable,
			expectedCode:   "0228",
			expectedTitle:  "Service dependency unavailable",
		},
		{
			name:           "0231 missing segment context is 500 (server config fault)",
			err:            pkg.ValidateBusinessError(constant.ErrMissingSegmentContext, ""),
			expectedStatus: fiber.StatusInternalServerError,
			expectedCode:   "0231",
			expectedTitle:  "Segment context unavailable",
		},
		{
			name:           "0178 transaction reservation unavailable is 503 (retryable outage)",
			err:            pkg.ValidateBusinessError(constant.ErrTransactionReservationUnavailable, constant.EntityTransaction),
			expectedStatus: fiber.StatusServiceUnavailable,
			expectedCode:   "0178",
			expectedTitle:  "Transaction Reservation Unavailable Error",
		},
	}

	require.Len(t, tests, 3, "the dependency-fault reclassification set is 3 codes (E5 0228, E5 0231, E9 0178)")

	runErrorContractCases(t, tests)
}

func runErrorContractCases(t *testing.T, tests []struct {
	name           string
	err            error
	expectedStatus int
	expectedCode   string
	expectedTitle  string
}) {
	t.Helper()

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
				"reclassified code must route to its binding HTTP status class")

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var errResp map[string]any
			require.NoError(t, json.Unmarshal(body, &errResp))

			code, ok := errResp["code"].(string)
			require.True(t, ok, "error response must carry a string code field, got: %s", string(body))

			assert.Equal(t, tt.expectedCode, code,
				"path must emit the exact canonical midaz code")
			assert.Equal(t, tt.expectedTitle, errResp["title"],
				"error title is part of the wire contract and must not drift")
		})
	}
}
