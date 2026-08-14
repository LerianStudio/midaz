// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// TestMainlineErrorContract_ReclassifiedCodes locks the wire contract for the mainline error
// codes whose HTTP status class was re-typed in the four-family consolidation. Each row pins
// code -> HTTP status -> title, driven end-to-end through pkg/net/http.WithError so the typed
// struct class (ValidationError-400, UnprocessableOperationError-422, EntityConflictError-409)
// is exercised exactly as production routes it. A future silent re-type of any of these arms in
// pkg/errors.go's errorMap breaks this lock.
//
// Coverage:
//   - 23 codes 400 -> 422 (ValidationError -> UnprocessableOperationError)
//   - 1 reverse fix -> 400 (ValidationError): 0096 (was 500)
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
		// --- 1 reverse fix -> 400 (ValidationError) ---
		{
			name:           "0096 account alias invalid is 400 (reverse fix from 500)",
			err:            pkg.ValidateBusinessError(constant.ErrAccountAliasInvalid, constant.EntityAccount),
			expectedStatus: fiber.StatusBadRequest,
			expectedCode:   "0096",
			expectedTitle:  "Invalid Account Alias",
		},
	}

	require.Len(t, tests, 24, "the reclassification table is 24 codes (23 move-422 + 1 reverse fix)")

	runErrorContractCases(t, tests)
}

// TestMainlineErrorContract_DependencyFaultCodes locks the three error-platform follow-up
// reclassifications: codes whose typed struct disagreed with the server-fault vs client-error
// class their own message describes.
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
			// >=500 scrub: MapError sets Title to http.StatusText(503). code+status frozen.
			expectedTitle: "Service Unavailable",
		},
		{
			name:           "0231 missing segment context is 500 (server config fault)",
			err:            pkg.ValidateBusinessError(constant.ErrMissingSegmentContext, ""),
			expectedStatus: fiber.StatusInternalServerError,
			expectedCode:   "0231",
			// >=500 scrub: MapError sets Title to http.StatusText(500). code+status frozen.
			expectedTitle: "Internal Server Error",
		},
		{
			name:           "0178 transaction reservation unavailable is 503 (retryable outage)",
			err:            pkg.ValidateBusinessError(constant.ErrTransactionReservationUnavailable, constant.EntityTransaction),
			expectedStatus: fiber.StatusServiceUnavailable,
			expectedCode:   "0178",
			// >=500 scrub: MapError sets Title to http.StatusText(503). code+status frozen.
			expectedTitle: "Service Unavailable",
		},
	}

	require.Len(t, tests, 3, "the dependency-fault reclassification set is 3 codes (E5 0228, E5 0231, E9 0178)")

	runErrorContractCases(t, tests)
}

// TestMainlineErrorContract_TransactionLifecycleCodes locks the wire contract for the three
// codes the v2 transaction lifecycle surface (commit / cancel / revert) rejects with, none of
// which were covered by the reclassification or dependency-fault tables. They span three
// different typed classes, so a re-type of any of them silently changes the status a client
// branches on:
//   - 0099 EntityConflictError -> 409 (non-PENDING commit/cancel; non-APPROVED revert)
//   - 0065 ValidationError -> 400 (malformed path UUID, rejected on the Fiber chain)
//   - 0150 UnprocessableOperationError -> 422 (revert of a non-bidirectional route)
//
// The integration lifecycle suite asserts these codes over live HTTP; this table is the E14
// registry lock behind them, so a drift in pkg/errors.go fails here without needing containers.
func TestMainlineErrorContract_TransactionLifecycleCodes(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
		expectedTitle  string
	}{
		{
			name:           "0099 commit transaction not pending is 409",
			err:            pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, constant.EntityTransaction),
			expectedStatus: fiber.StatusConflict,
			expectedCode:   "0099",
			expectedTitle:  "Invalid Transaction Status",
		},
		{
			// The message is a fmt.Sprintf over the offending parameter names, so the arg is
			// part of the call shape, not of the locked contract (only code/status/title are).
			name:           "0065 invalid path parameter is 400",
			err:            pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityTransaction, "transaction_id"),
			expectedStatus: fiber.StatusBadRequest,
			expectedCode:   "0065",
			expectedTitle:  "Invalid Path Parameter",
		},
		{
			name:           "0150 route not bidirectional is 422",
			err:            pkg.ValidateBusinessError(constant.ErrRouteNotBidirectional, constant.EntityTransaction),
			expectedStatus: fiber.StatusUnprocessableEntity,
			expectedCode:   "0150",
			expectedTitle:  "Route Not Bidirectional",
		},
	}

	require.Len(t, tests, 3, "the transaction-lifecycle lock set is 3 codes (0099 conflict, 0065 validation, 0150 unprocessable)")

	runErrorContractCases(t, tests)
}

// v2SideSpellingError runs the v2 request through the production Translate and returns
// the error it rejected with. Building the error through the real entry point rather
// than by hand-calling ValidateBusinessError keeps the lock honest: it pins what a
// client actually receives, not what a test-local call to the error factory renders.
func v2SideSpellingError(t *testing.T, in mtransaction.CreateTransactionV2Input) error {
	t.Helper()

	_, _, err := in.Translate(false)
	require.Error(t, err, "the input must be rejected by Translate")

	return err
}

// TestMainlineErrorContract_V2SideSpellingCodes locks the wire contract for the code the v2
// required-side rule answers with: 0009 ValidationError -> 400, for a side left empty (nil or
// an explicit `[]`).
func TestMainlineErrorContract_V2SideSpellingCodes(t *testing.T) {
	legs := []mtransaction.V2LegInput{v2ValueLeg("@a", "100")}

	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
		expectedTitle  string
	}{
		{
			name: "0009 an empty debits side is 400",
			err: v2SideSpellingError(t, mtransaction.CreateTransactionV2Input{
				Asset: "USD", Amount: "100", Credits: legs,
			}),
			expectedStatus: fiber.StatusBadRequest,
			expectedCode:   "0009",
			expectedTitle:  "Missing Fields in Request",
		},
		{
			name: "0009 an empty credits side is 400",
			err: v2SideSpellingError(t, mtransaction.CreateTransactionV2Input{
				Asset: "USD", Amount: "100", Debits: legs,
			}),
			expectedStatus: fiber.StatusBadRequest,
			expectedCode:   "0009",
			expectedTitle:  "Missing Fields in Request",
		},
	}

	require.Len(t, tests, 2, "the required-side lock set is both sides")

	runErrorContractCases(t, tests)
}

// v1SingleTransactionTypeError produces the v1 rendering of 0072 by running the canonical
// transaction shape through the SAME exported struct validator the v1 create ops run. The
// offending leg fills two value expressions at once, which is the `singletransactiontype`
// violation the v1 surface answers with 0072.
func v1SingleTransactionTypeError(t *testing.T) error {
	t.Helper()

	tx := &mtransaction.Transaction{
		Send: mtransaction.Send{
			Asset: "USD",
			Value: decimal.NewFromInt(100),
			Source: mtransaction.Source{From: []mtransaction.FromTo{{
				AccountAlias: "@a",
				Amount:       &mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(100)},
				Share:        &mtransaction.Share{Percentage: 100},
			}}},
			Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
				AccountAlias: "@b",
				Amount:       &mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(100)},
			}}},
		},
	}

	err := http.ValidateStruct(tx)
	require.Error(t, err, "a leg carrying two value expressions must violate singletransactiontype")

	return err
}

// v2LegValueExpressionError produces the v2 rendering of 0072: a leg on the named side
// filling NEITHER value expression, rejected by Translate.
func v2LegValueExpressionError(t *testing.T, in mtransaction.CreateTransactionV2Input) error {
	t.Helper()

	_, _, err := in.Translate(false)
	require.Error(t, err, "a leg with no value expression must be rejected by Translate")

	return err
}

// v2ValueLeg is a leg carrying a valid explicit amount, used to pad an array so the
// offending leg can be placed at a chosen index.
func v2ValueLeg(alias, amount string) mtransaction.V2LegInput {
	return mtransaction.V2LegInput{Alias: alias, Amount: amount, OrganizationID: v2ScopeOrgID, LedgerID: v2ScopeLedgerID}
}

// v2NoValueLeg is a fully scoped leg that fills NEITHER value expression, so the rejection it
// draws is attributable to the value-expression rule and to nothing else the leg leaves out.
func v2NoValueLeg(alias string) mtransaction.V2LegInput {
	return mtransaction.V2LegInput{Alias: alias, OrganizationID: v2ScopeOrgID, LedgerID: v2ScopeLedgerID}
}

// TestMainlineErrorContract_InvalidTransactionTypeMessagePerSurface locks the MESSAGE of
// 0072, not merely its (code, status) tuple. The two surfaces publish different sets of
// value expressions — v1 legs accept `amount`, `share` or `remaining`; a v2 leg accepts
// only `amount` or `share` — so one shared message cannot be correct for both. The status,
// the code and the title stay shared; only the enumerated expressions and the field name
// differ, and this is the only lock that reads the rendered string on either surface.
//
// The v1 row pins the released message BYTE FOR BYTE: v1 clients parse or display it, so
// narrowing the v2 wording must not touch it. The v2 rows additionally pin the LEG INDEX in
// the field name, and place the offending leg at index 1 as well as 0 so a hardcoded index
// cannot satisfy them.
func TestMainlineErrorContract_InvalidTransactionTypeMessagePerSurface(t *testing.T) {
	const (
		v1Detail        = "Only one transaction type ('amount', 'share', or 'remaining') must be specified in the 'send.source.from' field for each entry. Please review your input and try again."
		v2Expressions   = "('amount' or 'share')"
		v1OnlyRemaining = "remaining"
	)

	tests := []struct {
		name string
		err  error
		// wantDetail, when non-empty, pins the rendered message byte for byte.
		wantDetail string
		// wantDetailContains / wantDetailOmits pin the surface-specific parts of a
		// message whose field-name argument varies per side.
		wantDetailContains []string
		wantDetailOmits    []string
	}{
		{
			name:       "v1 detailed body keeps its released three-expression message",
			err:        v1SingleTransactionTypeError(t),
			wantDetail: v1Detail,
		},
		{
			name: "v2 debits leg names the two v2 expressions and the offending index",
			err: v2LegValueExpressionError(t, mtransaction.CreateTransactionV2Input{
				Asset: "USD", Amount: "100",
				Debits:  []mtransaction.V2LegInput{v2NoValueLeg("@a")},
				Credits: []mtransaction.V2LegInput{v2ValueLeg("@b", "100")},
			}),
			wantDetailContains: []string{v2Expressions, "'debits[0]'"},
			wantDetailOmits:    []string{v1OnlyRemaining},
		},
		{
			name: "v2 credits leg names the two v2 expressions and the offending index",
			err: v2LegValueExpressionError(t, mtransaction.CreateTransactionV2Input{
				Asset: "USD", Amount: "100",
				Debits:  []mtransaction.V2LegInput{v2ValueLeg("@a", "100")},
				Credits: []mtransaction.V2LegInput{v2NoValueLeg("@b")},
			}),
			wantDetailContains: []string{v2Expressions, "'credits[0]'"},
			wantDetailOmits:    []string{v1OnlyRemaining},
		},
		{
			// The offending leg is the SECOND one, so a field name that hardcoded index 0
			// would fail here. This is what makes the index a real part of the contract.
			name: "v2 debits leg at index one names its own index",
			err: v2LegValueExpressionError(t, mtransaction.CreateTransactionV2Input{
				Asset: "USD", Amount: "100",
				Debits:  []mtransaction.V2LegInput{v2ValueLeg("@a", "60"), v2NoValueLeg("@b")},
				Credits: []mtransaction.V2LegInput{v2ValueLeg("@c", "100")},
			}),
			wantDetailContains: []string{v2Expressions, "'debits[1]'"},
			wantDetailOmits:    []string{v1OnlyRemaining, "debits[0]"},
		},
		{
			name: "v2 credits leg at index one names its own index",
			err: v2LegValueExpressionError(t, mtransaction.CreateTransactionV2Input{
				Asset: "USD", Amount: "100",
				Debits:  []mtransaction.V2LegInput{v2ValueLeg("@a", "100")},
				Credits: []mtransaction.V2LegInput{v2ValueLeg("@b", "60"), v2NoValueLeg("@c")},
			}),
			wantDetailContains: []string{v2Expressions, "'credits[1]'"},
			wantDetailOmits:    []string{v1OnlyRemaining, "credits[0]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capturedErr := tt.err

			app := fiber.New()
			app.Get("/probe", func(c fiber.Ctx) error {
				return http.WithError(c, capturedErr)
			})

			resp, err := app.Test(httptest.NewRequest("GET", "/probe", nil))
			require.NoError(t, err)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var errResp map[string]any
			require.NoError(t, json.Unmarshal(body, &errResp))

			assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode, "0072 is a ValidationError on both surfaces")
			assert.Equal(t, "0072", errResp["code"], "both surfaces answer with the same canonical code")
			assert.Equal(t, "Invalid Transaction Type", errResp["title"], "the title is shared across surfaces")

			detail, ok := errResp["detail"].(string)
			require.Truef(t, ok, "the problem envelope must carry a string detail; got: %s", string(body))

			if tt.wantDetail != "" {
				assert.Equal(t, tt.wantDetail, detail, "the v1 message is released wire text and must stay byte-identical")
			}

			for _, want := range tt.wantDetailContains {
				assert.Contains(t, detail, want,
					"the v2 message must name the expressions a v2 leg publishes and the offending leg's index")
			}

			for _, unwanted := range tt.wantDetailOmits {
				assert.NotContains(t, detail, unwanted,
					"the v2 message must not offer an expression the v2 leg has no field for, nor point at a leg that is valid")
			}
		})
	}
}

func runErrorContractCases(t *testing.T, tests []struct {
	name           string
	err            error
	expectedStatus int
	expectedCode   string
	expectedTitle  string
},
) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capturedErr := tt.err

			app := fiber.New()
			app.Get("/probe", func(c fiber.Ctx) error {
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
