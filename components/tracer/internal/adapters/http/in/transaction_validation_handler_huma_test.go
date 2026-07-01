// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	tmctx "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// tvTenantSpyService is a TransactionValidationService stub that records the
// tenant ID it sees on its incoming ctx. It is the ctx-threading probe: the
// tenant middleware writes the tenant into c.UserContext(), the humafiber v2
// adapter builds the Huma handler ctx from c.UserContext(), and the handler
// passes that ctx to the service — a non-empty capturedTenant proves the whole
// chain end to end without a bridge. Distinct name from the rule test's
// tenantSpyService (same package) since the interface is different.
type tvTenantSpyService struct {
	capturedTenant string
	getResult      *model.TransactionValidation
	getErr         error
	listResult     *query.ListTransactionValidationsResult
	listErr        error
	// listFilters captures the filter the core built from the query, so tests can
	// assert imperative binding/defaults produced the same filter the Fiber path
	// would.
	listFilters *model.TransactionValidationFilters
}

func (s *tvTenantSpyService) GetTransactionValidation(ctx context.Context, _ uuid.UUID) (*model.TransactionValidation, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	return s.getResult, s.getErr
}

func (s *tvTenantSpyService) ListTransactionValidations(ctx context.Context, filters *model.TransactionValidationFilters) (*query.ListTransactionValidationsResult, error) {
	s.capturedTenant = tmctx.GetTenantIDContext(ctx)
	s.listFilters = filters
	return s.listResult, s.listErr
}

// buildHumaTransactionValidationApp mirrors buildHumaRuleApp for the two
// transaction-validation ops. See buildHumaRuleApp's header for the full
// production-wiring rationale.
//
// MUST-NOT-PARALLELIZE (like every 2b copy): tests built through this helper
// CANNOT call t.Parallel(). libProblem.Install() swaps the process-global
// huma.NewError hook, and Huma validation uses process-global sync.Pools;
// concurrent builds/requests cross-contaminate. -race does NOT catch it (the bug
// is logical, not a data race). These tests are sub-second — keep them
// sequential.
func buildHumaTransactionValidationApp(t *testing.T, svc TransactionValidationService, tenantID string) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	// problem.Install must run before any huma.Register (runtime + spec-gen).
	libProblem.Install()

	api := f.Group("/v1")
	api.Use(func(c *fiber.Ctx) error {
		if tenantID != "" {
			c.SetUserContext(tmctx.ContextWithTenantID(c.UserContext(), tenantID))
		}
		return c.Next()
	})

	hAPI := openapi.New(f, api, openapi.Config{Title: "tracer-test", Version: "test", Servers: []string{"/v1"}})

	h := NewTransactionValidationHandler(svc)
	RegisterTransactionValidationRoutes(hAPI, h)

	return f
}

func TestHuma_GetTransactionValidation_Success(t *testing.T) {
	// NOT parallel: buildHumaTransactionValidationApp mutates process-global huma
	// state (libProblem.Install + Huma validation pools).
	id := testutil.MustDeterministicUUID(7)
	tv := &model.TransactionValidation{
		ID:              id,
		Amount:          decimal.RequireFromString("100.00"),
		Currency:        "USD",
		TransactionType: model.TransactionTypeCard,
		Account:         model.AccountContext{ID: testutil.MustDeterministicUUID(8)},
		EvaluationResult: model.EvaluationResult{
			Decision:       model.DecisionAllow,
			Reason:         "All checks passed",
			MatchedRuleIDs: []uuid.UUID{},
		},
		LimitUsageDetails: []model.LimitUsageDetail{},
		ProcessingTimeMs:  42,
		CreatedAt:         testutil.FixedTime(),
	}
	svc := &tvTenantSpyService{getResult: tv}

	app := buildHumaTransactionValidationApp(t, svc, "tenant-beta")

	req := httptest.NewRequest(http.MethodGet, "/v1/validations/"+id.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "GetTransactionValidation must return 200 through Huma")

	// No Huma JSON-Schema hyperlink fields leak: openapi.New zeroes the
	// SchemaLinkTransformer, so the body is the model.TransactionValidation verbatim.
	assert.NotContains(t, string(respBody), "$schema", "SchemaLinkTransformer must be zeroed — no $schema in body")
	assert.NotContains(t, string(respBody), "$ref")
	assert.NotContains(t, string(respBody), "$id")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	// validationId is the model.TransactionValidation json tag for ID.
	assert.Equal(t, id.String(), got["validationId"])
	assert.Equal(t, "ALLOW", got["decision"])
	assert.Equal(t, "All checks passed", got["reason"])

	assert.Equal(t, "tenant-beta", svc.capturedTenant,
		"tenant from c.UserContext() must reach the service via the Huma handler ctx")
}

// TestHuma_GetTransactionValidation_BadUUID pins the malformed-path-param
// contract: a non-UUID {id} must reach the imperative uuid.Parse and produce the
// canonical 400 / code 0065 (ErrInvalidPathParameter) / entityType
// TransactionValidation — NOT a native Huma 422. Path params carry NO
// format:"uuid" tag.
func TestHuma_GetTransactionValidation_BadUUID(t *testing.T) {
	svc := &tvTenantSpyService{}
	app := buildHumaTransactionValidationApp(t, svc, "tenant-epsilon")

	req := httptest.NewRequest(http.MethodGet, "/v1/validations/not-a-uuid", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"malformed path UUID must be the canonical 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"),
		"error must carry the RFC 9457 media type")

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0065", got["code"], "code must be ErrInvalidPathParameter, identical to the Fiber path")
	assert.Equal(t, float64(http.StatusBadRequest), got["status"], "RFC 9457 status field")
	assert.Equal(t, libProblem.BaseURI+"/0065", got["type"], "RFC 9457 type is BaseURI/code")
	assert.Equal(t, "TransactionValidation", got["entityType"], "canonical entityType from ErrInvalidPathParameter")

	assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad path param")
}

// TestHuma_GetTransactionValidation_ErrorBodyMatchesFiberEnvelope pins
// field-identity of the error body across the two transports for a not-found.
func TestHuma_GetTransactionValidation_ErrorBodyMatchesFiberEnvelope(t *testing.T) {
	svc := &tvTenantSpyService{getErr: constant.ErrTransactionValidationNotFound}
	app := buildHumaTransactionValidationApp(t, svc, "tenant-delta")

	id := testutil.MustDeterministicUUID(3)
	req := httptest.NewRequest(http.MethodGet, "/v1/validations/"+id.String(), nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	humaBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(humaBody, &got), "body must be JSON: %s", string(humaBody))
	assert.Equal(t, constant.ErrTransactionValidationNotFound.Error(), got["code"],
		"code must be the canonical not-found code, identical to the Fiber path")
	assert.Equal(t, "TransactionValidation", got["entityType"])

	// The service IS reached here (it is what returns the not-found), so the tenant
	// threaded through the Huma handler ctx — proving ctx-threading on the error path too.
	assert.Equal(t, "tenant-delta", svc.capturedTenant,
		"tenant from c.UserContext() must reach the service via the Huma handler ctx")
}

func TestHuma_ListTransactionValidations_Success(t *testing.T) {
	now := testutil.FixedTime()
	tv := &model.TransactionValidation{
		ID:              testutil.MustDeterministicUUID(20),
		Amount:          decimal.RequireFromString("100.00"),
		Currency:        "USD",
		TransactionType: model.TransactionTypeCard,
		Account:         model.AccountContext{ID: testutil.MustDeterministicUUID(21)},
		EvaluationResult: model.EvaluationResult{
			Decision:       model.DecisionAllow,
			Reason:         "ok",
			MatchedRuleIDs: []uuid.UUID{},
		},
		LimitUsageDetails: []model.LimitUsageDetail{},
		ProcessingTimeMs:  12.5,
		CreatedAt:         now,
	}
	svc := &tvTenantSpyService{listResult: &query.ListTransactionValidationsResult{
		TransactionValidations: []*model.TransactionValidation{tv},
		NextCursor:             "next123",
		HasMore:                true,
	}}
	app := buildHumaTransactionValidationApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodGet, "/v1/validations?limit=25&decision=ALLOW&sort_by=created_at&sort_order=asc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "ListTransactionValidations must return 200 through Huma")
	assert.NotContains(t, string(respBody), "$schema")

	var got ListTransactionValidationsResponse
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be ListTransactionValidationsResponse: %s", string(respBody))
	require.Len(t, got.TransactionValidations, 1)
	assert.Equal(t, tv.ID, got.TransactionValidations[0].ID)
	assert.Equal(t, "next123", got.NextCursor)
	assert.True(t, got.HasMore)

	assert.Equal(t, "tenant-alpha", svc.capturedTenant)
	// Imperative binding produced the same typed filter the Fiber QueryParser path would.
	require.NotNil(t, svc.listFilters)
	assert.Equal(t, 25, svc.listFilters.Limit)
	require.NotNil(t, svc.listFilters.Decision)
	assert.Equal(t, model.DecisionAllow, *svc.listFilters.Decision)
	assert.Equal(t, "created_at", svc.listFilters.SortBy)
	assert.Equal(t, "ASC", svc.listFilters.SortOrder, "sort_order normalized to uppercase by SetDefaults")
}

func TestHuma_ListTransactionValidations_Defaults(t *testing.T) {
	svc := &tvTenantSpyService{listResult: &query.ListTransactionValidationsResult{TransactionValidations: nil}}
	app := buildHumaTransactionValidationApp(t, svc, "tenant-alpha")

	// No query params -> SetDefaults() must fill limit + sort.
	req := httptest.NewRequest(http.MethodGet, "/v1/validations", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
	assert.NotNil(t, got["transactionValidations"], "list key present")

	require.NotNil(t, svc.listFilters)
	assert.Equal(t, model.DefaultTransactionValidationFilterLimit, svc.listFilters.Limit, "default limit applied by SetDefaults")
	assert.Equal(t, "created_at", svc.listFilters.SortBy, "default sort field applied by SetDefaults")
	assert.Equal(t, "DESC", svc.listFilters.SortOrder, "default sort order applied by SetDefaults")
}

// TestHuma_ListTransactionValidations_InvalidLimit pins the query-param contract:
// an out-of-range limit must reach the imperative Validate() and produce the
// canonical 400 / code 0080 (ErrPaginationLimitExceeded) — NOT a native Huma 422
// from a min/max struct tag. Query fields carry NO validation tags.
func TestHuma_ListTransactionValidations_InvalidLimit(t *testing.T) {
	svc := &tvTenantSpyService{}
	app := buildHumaTransactionValidationApp(t, svc, "tenant-gamma")

	req := httptest.NewRequest(http.MethodGet, "/v1/validations?limit=1001", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "out-of-range limit must be the canonical 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0080", got["code"], "code must be ErrPaginationLimitExceeded, identical to the Fiber path")
	assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad query param")
}

// TestHuma_ListTransactionValidations_NonNumericLimit pins the harder half of the
// query contract: limit=abc cannot coerce to an int. The Huma binder returns a
// bind error the core canonicalizes to 0082 (ErrInvalidQueryParameter) — the same
// code the Fiber QueryParser-failure path produces. No native Huma 422.
func TestHuma_ListTransactionValidations_NonNumericLimit(t *testing.T) {
	svc := &tvTenantSpyService{}
	app := buildHumaTransactionValidationApp(t, svc, "tenant-gamma")

	req := httptest.NewRequest(http.MethodGet, "/v1/validations?limit=abc", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "non-numeric limit must be the canonical 400 — no native Huma 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	assert.Equal(t, "0082", got["code"], "code must be ErrInvalidQueryParameter, identical to the Fiber QueryParser-failure path")
	assert.Empty(t, svc.capturedTenant, "service must not be reached on an unparseable query param")
}

// TestHuma_ListTransactionValidations_InvalidParams pins the enum/date/uuid-shaped
// query params: a bad value must reach the imperative Validate() and produce the
// canonical 400 — NOT a native Huma 422 from an enum/format struct tag.
func TestHuma_ListTransactionValidations_InvalidParams(t *testing.T) {
	for _, tc := range []struct {
		name, query, code string
	}{
		// ErrInvalidTransactionValidationFilters (0431)
		{"invalid decision", "decision=INVALID", "0431"},
		{"invalid transaction_type", "transaction_type=INVALID", "0431"},
		{"invalid account_id", "account_id=not-a-uuid", "0431"},
		// ErrInvalidSortColumn (0332)
		{"invalid sort_by", "sort_by=priority", "0332"},
		// ErrInvalidSortOrder (0081)
		{"invalid sort_order", "sort_order=RANDOM", "0081"},
		// ErrInvalidDateFormat (0077)
		{"invalid start_date", "start_date=not-a-date", "0077"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tvTenantSpyService{}
			app := buildHumaTransactionValidationApp(t, svc, "tenant-gamma")

			req := httptest.NewRequest(http.MethodGet, "/v1/validations?"+tc.query, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "invalid query must be the canonical 400 — no native Huma 422")
			assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))
			assert.Equal(t, tc.code, got["code"], "canonical code identical to the Fiber path")
			assert.Empty(t, svc.capturedTenant, "service must not be reached on a bad query param")
		})
	}
}

// TestHuma_ListTransactionValidations_RepeatedKeyParity pins the repeated-query-key
// contract. Fiber's c.QueryParser (gorilla/schema over a scalar target) is
// LAST-wins: the Huma binder MUST resolve the SAME last value via the inline
// last() helper; reading the first value (url.Values.Get) would flip the
// status/code. Each row is anchored to what the LAST value hits in the shared
// imperative Validate().
func TestHuma_ListTransactionValidations_RepeatedKeyParity(t *testing.T) {
	// Repeated keys whose LAST value fails Validate: canonical 400, service NOT reached.
	rejectCases := []struct {
		name  string
		query string
		code  string
	}{
		// Last-wins "garbage" -> Decision invalid -> 0431. First-wins "ALLOW" would 200 (FLIP).
		{"decision last invalid", "?decision=ALLOW&decision=garbage", "0431"},
		// Last-wins "abc" -> int decode error -> 0082. First-wins "25" would 200 (FLIP).
		{"limit last non-numeric", "?limit=25&limit=abc", "0082"},
		// Last-wins "priority" -> not in sort whitelist -> 0332. First-wins "created_at" would 200 (FLIP).
		{"sort_by last invalid", "?sort_by=created_at&sort_by=priority", "0332"},
	}
	for _, tc := range rejectCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tvTenantSpyService{listResult: &query.ListTransactionValidationsResult{TransactionValidations: nil}}
			app := buildHumaTransactionValidationApp(t, svc, "tenant-alpha")

			req := httptest.NewRequest(http.MethodGet, "/v1/validations"+tc.query, nil)
			resp, err := app.Test(req, -1)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"repeated key %q must resolve to Fiber's LAST value and 400 — never the first-value 200", tc.query)
			assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

			var got map[string]any
			require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
			assert.Equal(t, tc.code, got["code"], "code must match Fiber's last-wins value for %q", tc.query)
			assert.Empty(t, svc.capturedTenant, "service must not be reached on a rejected query param")
		})
	}

	// Repeated key whose LAST value passes Validate: 200, filter built from the LAST value.
	t.Run("limit last in-range", func(t *testing.T) {
		svc := &tvTenantSpyService{listResult: &query.ListTransactionValidationsResult{TransactionValidations: nil}}
		app := buildHumaTransactionValidationApp(t, svc, "tenant-alpha")

		// Fiber last-wins "25" -> 200. First-wins "1001" would 400/0080 (reverse FLIP).
		req := httptest.NewRequest(http.MethodGet, "/v1/validations?limit=1001&limit=25", nil)
		resp, err := app.Test(req, -1)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		_, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode, "repeated limit must resolve to Fiber's LAST value and 200")
		require.NotNil(t, svc.listFilters)
		assert.Equal(t, 25, svc.listFilters.Limit, "limit must be the LAST value")
	})
}

// TestHuma_ListTransactionValidations_EmptyLimitParity is the money-path
// regression: Fiber's c.QueryParser binds a present-but-empty limit to a non-nil
// &0, which Validate() rejects (0331). Huma drops value=="" before the handler, so
// the binder MUST read the raw query (via the Resolver) to reproduce Fiber's
// present-vs-absent distinction. Without it, ?limit= silently returns 200.
func TestHuma_ListTransactionValidations_EmptyLimitParity(t *testing.T) {
	svc := &tvTenantSpyService{}
	app := buildHumaTransactionValidationApp(t, svc, "tenant-alpha")

	req := httptest.NewRequest(http.MethodGet, "/v1/validations?limit=", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"present-but-empty limit must be the canonical 400, matching Fiber — never a silent 200 or native 422")
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body must be JSON: %s", string(respBody))
	// ?limit= -> Fiber Limit=&0 (gorilla decodes "" as 0) -> *limit<1 -> 0331.
	assert.Equal(t, "0331", got["code"], "code must be ErrPaginationLimitInvalid, identical to the Fiber path")
	assert.Empty(t, svc.capturedTenant, "service must not be reached on a rejected query param")
}
