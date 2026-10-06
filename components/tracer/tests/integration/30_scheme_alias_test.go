// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// schemeAliasLimitCap is the DAILY cap of the BOLETO-scoped limit the reserve
// scenario spends against.
const schemeAliasLimitCap = "1000"

// schemeAliasConflictCode is the error code of a request whose scheme and its
// deprecated alias transactionType carry different values.
const schemeAliasConflictCode = "0539"

// schemeAliasLimit is the slice of a limit response the scenarios read back.
type schemeAliasLimit struct {
	ID     string                   `json:"limitId"`
	Scopes []testutil.ScopeResponse `json:"scopes"`
}

// schemeAliasLimitList is the slice of GET /v1/limits the scenarios read.
type schemeAliasLimitList struct {
	Limits []schemeAliasLimit `json:"limits"`
}

// schemeAliasValidation is the slice of GET /v1/validations/{id} the scenarios
// read back.
type schemeAliasValidation struct {
	ID              string `json:"validationId"`
	TransactionType string `json:"transactionType"`
	Scheme          string `json:"scheme"`
}

// schemeAliasValidationPayload builds a /v1/validations body for accountID with
// the given scheme fields; an empty value omits that field.
func schemeAliasValidationPayload(requestID, accountID uuid.UUID, scheme, transactionType string) map[string]any {
	payload := map[string]any{
		"requestId":            requestID.String(),
		"amount":               "100.00",
		"asset":                "BRL",
		"transactionTimestamp": testutil.FixedTime().Add(-1 * time.Minute).Format(time.RFC3339),
		"account": map[string]any{
			"accountId": accountID.String(),
			"type":      "checking",
			"status":    "active",
		},
	}

	if scheme != "" {
		payload["scheme"] = scheme
	}

	if transactionType != "" {
		payload["transactionType"] = transactionType
	}

	return payload
}

// mustValidateScheme posts a validation the tracer must accept and returns its
// decoded response: 201 when it is recorded, 200 when its request id replays a
// validation an earlier run recorded.
func mustValidateScheme(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()

	result, status := testutil.ExecuteValidationRequest(t, payload)
	require.Contains(t, []int{http.StatusCreated, http.StatusOK}, status, "validation must be accepted: %v", result)

	return result
}

// createSchemeScopedLimit creates a DRAFT DAILY BRL limit scoped to accountID
// and the given scheme, deleted when the test ends.
func createSchemeScopedLimit(t *testing.T, name string, accountID uuid.UUID, scheme string) string {
	t.Helper()

	scopedAccount := accountID.String()
	limitID := testutil.CreateLimitWithScope(t, name, schemeAliasLimitCap,
		[]testutil.ScopeInput{{AccountID: &scopedAccount, Scheme: &scheme}})

	t.Cleanup(func() { testutil.CleanupLimit(t, limitID) })

	return limitID
}

// mustGetSchemeAliasLimit reads a limit back over the API.
func mustGetSchemeAliasLimit(t *testing.T, limitID string) schemeAliasLimit {
	t.Helper()

	resp, body := testutil.GetLimit(t, limitID)
	require.Equal(t, http.StatusOK, resp.StatusCode, "GET limit: %s", string(body))

	var limit schemeAliasLimit
	require.NoError(t, json.Unmarshal(body, &limit))

	return limit
}

// listSchemeAliasLimitIDs lists limits with query and returns their ids.
func listSchemeAliasLimitIDs(t *testing.T, query url.Values) []string {
	t.Helper()

	resp, body := testutil.ListLimits(t, query.Encode())
	require.Equal(t, http.StatusOK, resp.StatusCode, "GET limits: %s", string(body))

	var list schemeAliasLimitList
	require.NoError(t, json.Unmarshal(body, &list))

	ids := make([]string, 0, len(list.Limits))
	for _, limit := range list.Limits {
		ids = append(ids, limit.ID)
	}

	return ids
}

// assertSchemeAliasConflict asserts a 400 carrying the scheme alias conflict
// code.
func assertSchemeAliasConflict(t *testing.T, status int, body []byte) {
	t.Helper()

	require.Equal(t, http.StatusBadRequest, status, "body: %s", string(body))
	assert.Equal(t, schemeAliasConflictCode, testutil.ParseErrorResponse(t, body).Code)
}

// assertBothSchemeFields asserts a scope carries want under scheme and under
// its deprecated alias transactionType.
func assertBothSchemeFields(t *testing.T, scope testutil.ScopeResponse, want string) {
	t.Helper()

	require.NotNil(t, scope.Scheme, "scope must carry scheme")
	require.NotNil(t, scope.TransactionType, "scope must carry transactionType")
	assert.Equal(t, want, *scope.Scheme)
	assert.Equal(t, want, *scope.TransactionType)
}

// TestIntegration_SchemeAlias_LimitScopeStoresNormalizedScheme proves a limit
// scope written with only a lower-case scheme is stored normalized and read
// back with the same value under scheme and its alias transactionType.
func TestIntegration_SchemeAlias_LimitScopeStoresNormalizedScheme(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(97001)
	limitID := createSchemeScopedLimit(t, "Scheme alias BOLETO scope", accountID, "boleto")

	limit := mustGetSchemeAliasLimit(t, limitID)

	require.Len(t, limit.Scopes, 1)
	assertBothSchemeFields(t, limit.Scopes[0], "BOLETO")
	require.NotNil(t, limit.Scopes[0].AccountID)
	assert.Equal(t, accountID.String(), *limit.Scopes[0].AccountID)
}

// TestIntegration_SchemeAlias_ReserveCountsAgainstSchemeScopedLimit proves a
// reserve whose transaction_type is a lower-case scheme outside the former enum
// resolves the limit scoped to that scheme: both admitted reserves hold one
// reservation, the reserve past the cap is refused as limit_exceeded, and a
// reserve on another scheme never counts against it.
func TestIntegration_SchemeAlias_ReserveCountsAgainstSchemeScopedLimit(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)

	accountID := testutil.MustDeterministicUUID(97002)
	limitID := createSchemeScopedLimit(t, "Scheme alias BOLETO reserve", accountID, "BOLETO")
	testutil.ActivateLimit(t, limitID)

	assertReserveAllowed(t, mustReserveTyped(t, 97101, accountID, "600.00", "boleto"), 1)
	assertReserveAllowed(t, mustReserveTyped(t, 97102, accountID, "400.00", "boleto"), 1)
	assertReserveLimitExceeded(t, mustReserveTyped(t, 97103, accountID, "1.00", "boleto"))
	assertReserveAllowed(t, mustReserveTyped(t, 97104, accountID, "1.00", "PIX"), 0)

	assert.True(t, limitUsage(t, db, limitID).Equal(decimal.RequireFromString(schemeAliasLimitCap)),
		"the BOLETO-scoped limit must hold exactly the BOLETO spend")
}

// TestIntegration_SchemeAlias_ValidationWithSchemeOnly proves /v1/validations
// accepts a request carrying only scheme, and the stored validation reads back
// with the same value under scheme and its alias transactionType.
func TestIntegration_SchemeAlias_ValidationWithSchemeOnly(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(97003)
	requestID := testutil.MustDeterministicUUID(97301)

	result := mustValidateScheme(t, schemeAliasValidationPayload(requestID, accountID, "BOLETO", ""))

	validationID, ok := result["validationId"].(string)
	require.True(t, ok, "response must carry validationId: %v", result)

	resp, body := testutil.GetValidation(t, validationID)
	require.Equal(t, http.StatusOK, resp.StatusCode, "GET validation: %s", string(body))

	var stored schemeAliasValidation
	require.NoError(t, json.Unmarshal(body, &stored))

	assert.Equal(t, "BOLETO", stored.Scheme)
	assert.Equal(t, "BOLETO", stored.TransactionType)
}

// TestIntegration_SchemeAlias_ValidationConflictRejected proves /v1/validations
// refuses a request whose scheme and transactionType differ with 400 0539.
func TestIntegration_SchemeAlias_ValidationConflictRejected(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(97004)
	requestID := testutil.MustDeterministicUUID(97302)

	payload, err := json.Marshal(schemeAliasValidationPayload(requestID, accountID, "PIX", "CARD"))
	require.NoError(t, err)

	resp, body := testutil.CreateValidationRaw(t, payload)

	assertSchemeAliasConflict(t, resp.StatusCode, body)
}

// TestIntegration_SchemeAlias_ListFilters proves the limit and validation lists
// filter by a lower-case scheme, accept the deprecated transaction_type alias
// for the same filter, and refuse the two carrying different values with 400
// 0539.
func TestIntegration_SchemeAlias_ListFilters(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(97005)

	t.Run("limits", func(t *testing.T) {
		boletoLimitID := createSchemeScopedLimit(t, "Scheme alias list BOLETO", accountID, "BOLETO")
		createSchemeScopedLimit(t, "Scheme alias list PIX", accountID, "PIX")

		bySchemeIDs := listSchemeAliasLimitIDs(t, url.Values{"account_id": {accountID.String()}, "scheme": {"boleto"}})
		assert.Equal(t, []string{boletoLimitID}, bySchemeIDs)

		byAliasIDs := listSchemeAliasLimitIDs(t, url.Values{"account_id": {accountID.String()}, "transaction_type": {"boleto"}})
		assert.Equal(t, []string{boletoLimitID}, byAliasIDs)

		resp, body := testutil.ListLimits(t, url.Values{"scheme": {"PIX"}, "transaction_type": {"CARD"}}.Encode())
		assertSchemeAliasConflict(t, resp.StatusCode, body)
	})

	t.Run("validations", func(t *testing.T) {
		boleto := mustValidateScheme(t, schemeAliasValidationPayload(testutil.MustDeterministicUUID(97303), accountID, "BOLETO", ""))
		mustValidateScheme(t, schemeAliasValidationPayload(testutil.MustDeterministicUUID(97304), accountID, "PIX", ""))

		for _, query := range []url.Values{
			{"account_id": {accountID.String()}, "scheme": {"boleto"}},
			{"account_id": {accountID.String()}, "transaction_type": {"boleto"}},
		} {
			resp, body := testutil.ListValidations(t, query.Encode())
			require.Equal(t, http.StatusOK, resp.StatusCode, "GET validations: %s", string(body))

			var list testutil.ListValidationsResponse
			require.NoError(t, json.Unmarshal(body, &list))

			require.Len(t, list.TransactionValidations, 1, "query %s", query.Encode())
			assert.Equal(t, boleto["validationId"], list.TransactionValidations[0].ID)
			assert.Equal(t, "BOLETO", list.TransactionValidations[0].Scheme)
			assert.Equal(t, "BOLETO", list.TransactionValidations[0].TransactionType)
		}

		resp, body := testutil.ListValidations(t, url.Values{"scheme": {"PIX"}, "transaction_type": {"CARD"}}.Encode())
		assertSchemeAliasConflict(t, resp.StatusCode, body)
	})
}

// TestIntegration_SchemeAlias_RuleExpressionReadsScheme proves a DENY rule
// reading the scheme refuses a validation sent with the lower-case scheme, both
// when the expression names scheme and when it names its alias transactionType,
// and leaves a validation on another scheme allowed.
//
// The request ids are derived from the rule id: validations are idempotent by
// request id, so a fixed id would replay the decision a previous run recorded
// against a rule that no longer exists.
func TestIntegration_SchemeAlias_RuleExpressionReadsScheme(t *testing.T) {
	for _, tc := range []struct {
		name       string
		accountID  uuid.UUID
		expression string
	}{
		{name: "scheme variable", accountID: testutil.MustDeterministicUUID(97006), expression: `scheme == "BOLETO"`},
		{name: "transactionType alias", accountID: testutil.MustDeterministicUUID(97007), expression: `transactionType == "BOLETO"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ruleID := createScopedReserveRule(t, "Scheme alias rule "+tc.name, tc.expression, "DENY", tc.accountID)
			requestNamespace := uuid.MustParse(ruleID)

			denied := mustValidateScheme(t, schemeAliasValidationPayload(
				uuid.NewSHA1(requestNamespace, []byte("boleto")), tc.accountID, "boleto", "",
			))
			assert.Equal(t, "DENY", denied["decision"])
			testutil.AssertRuleMatched(t, denied, ruleID)

			allowed := mustValidateScheme(t, schemeAliasValidationPayload(
				uuid.NewSHA1(requestNamespace, []byte("pix")), tc.accountID, "PIX", "",
			))
			assert.Equal(t, "ALLOW", allowed["decision"])
			testutil.AssertRuleEvaluatedButNotMatched(t, allowed, ruleID)
		})
	}
}
