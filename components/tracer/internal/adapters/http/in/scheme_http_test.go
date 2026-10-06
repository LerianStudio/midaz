// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// overlongScheme is one character past the scheme maximum.
var overlongScheme = strings.Repeat("A", 51)

func TestResolveSchemeFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		alias     *string
		primary   *string
		wantValue string
		wantNil   bool
		wantErr   error
	}{
		{name: "both absent is no filter", wantNil: true},
		{name: "both empty is no filter", alias: testutil.StringPtr(""), primary: testutil.StringPtr("  "), wantNil: true},
		{name: "scheme is trimmed and upper-cased", primary: testutil.StringPtr(" boleto "), wantValue: "BOLETO"},
		{name: "deprecated alias is trimmed and upper-cased", alias: testutil.StringPtr("pix"), wantValue: "PIX"},
		{name: "equal after normalization is not a conflict", alias: testutil.StringPtr("PIX"), primary: testutil.StringPtr("pix"), wantValue: "PIX"},
		{name: "different values conflict", alias: testutil.StringPtr("PIX"), primary: testutil.StringPtr("CARD"), wantErr: constant.ErrValidationSchemeAliasConflict},
		{name: "invalid scheme", primary: testutil.StringPtr("bad value!"), wantErr: errInvalidSchemeFilter},
		{name: "invalid alias", alias: testutil.StringPtr("bad value!"), wantErr: errInvalidSchemeFilter},
		{name: "overlong scheme", primary: &overlongScheme, wantErr: errInvalidSchemeFilter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveSchemeFilter(tt.alias, tt.primary)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)

				return
			}

			require.NoError(t, err)

			if tt.wantNil {
				assert.Nil(t, got)
				return
			}

			require.NotNil(t, got)
			assert.Equal(t, model.TransactionType(tt.wantValue), *got)
		})
	}
}

func schemeScope(transactionType, scheme *string) model.Scope {
	var scope model.Scope

	if transactionType != nil {
		value := model.TransactionType(*transactionType)
		scope.TransactionType = &value
	}

	scope.Scheme = scheme

	return scope
}

func TestCreateRuleInput_Validate_NormalizesScopeScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		scope    model.Scope
		want     string
		wantCode string
	}{
		{name: "lowercase scheme", scope: schemeScope(nil, testutil.StringPtr("pix")), want: "PIX"},
		{name: "padded transaction type", scope: schemeScope(testutil.StringPtr(" pix "), nil), want: "PIX"},
		{name: "open scheme", scope: schemeScope(nil, testutil.StringPtr("BOLETO")), want: "BOLETO"},
		{name: "matching alias", scope: schemeScope(testutil.StringPtr("pix"), testutil.StringPtr("PIX")), want: "PIX"},
		{name: "conflicting alias", scope: schemeScope(testutil.StringPtr("PIX"), testutil.StringPtr("CARD")), wantCode: constant.ErrValidationSchemeAliasConflict.Error()},
		{name: "invalid scheme", scope: schemeScope(nil, testutil.StringPtr("bad value!")), wantCode: constant.ErrRuleInvalidScope.Error()},
		{name: "invalid transaction type", scope: schemeScope(testutil.StringPtr("bad value!"), nil), wantCode: constant.ErrRuleInvalidScope.Error()},
		{name: "overlong scheme", scope: schemeScope(nil, &overlongScheme), wantCode: constant.ErrRuleInvalidScope.Error()},
		{name: "empty scheme", scope: schemeScope(nil, testutil.StringPtr("")), wantCode: constant.ErrRuleInvalidScope.Error()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := CreateRuleInput{
				Name:       "Scheme Rule",
				Expression: "amount > 0",
				Action:     model.DecisionDeny,
				Scopes:     []model.Scope{tt.scope},
			}

			err := input.Validate()
			if tt.wantCode != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantCode)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, input.Scopes[0].TransactionType)
			require.NotNil(t, input.Scopes[0].Scheme)
			assert.Equal(t, model.TransactionType(tt.want), *input.Scopes[0].TransactionType)
			assert.Equal(t, tt.want, *input.Scopes[0].Scheme)
		})
	}
}

func TestUpdateRuleInput_Validate_NormalizesScopeScheme(t *testing.T) {
	t.Parallel()

	scopes := []model.Scope{schemeScope(nil, testutil.StringPtr("pix"))}
	input := UpdateRuleInput{Scopes: &scopes}

	require.NoError(t, input.Validate())
	require.NotNil(t, (*input.Scopes)[0].TransactionType)
	assert.Equal(t, model.TransactionTypePix, *(*input.Scopes)[0].TransactionType)
	require.NotNil(t, (*input.Scopes)[0].Scheme)
	assert.Equal(t, "PIX", *(*input.Scopes)[0].Scheme)

	conflicting := []model.Scope{schemeScope(testutil.StringPtr("PIX"), testutil.StringPtr("CARD"))}
	conflictInput := UpdateRuleInput{Scopes: &conflicting}

	err := conflictInput.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrValidationSchemeAliasConflict.Error())
}

func TestCreateLimitInput_Validate_NormalizesScopeScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		scope       model.Scope
		want        string
		wantCode    string
		wantMessage string
	}{
		{name: "lowercase transaction type", scope: schemeScope(testutil.StringPtr("pix"), nil), want: "PIX"},
		{name: "open scheme", scope: schemeScope(nil, testutil.StringPtr(" boleto ")), want: "BOLETO"},
		{name: "conflicting alias", scope: schemeScope(testutil.StringPtr("PIX"), testutil.StringPtr("CARD")), wantCode: constant.ErrValidationSchemeAliasConflict.Error()},
		{
			name:        "invalid transaction type",
			scope:       schemeScope(testutil.StringPtr("bad value!"), nil),
			wantCode:    constant.ErrMissingFieldsInRequest.Error(),
			wantMessage: "scope at index 0: transactionType " + schemeFormatHint,
		},
		{
			name:        "invalid scheme",
			scope:       schemeScope(nil, testutil.StringPtr("bad value!")),
			wantCode:    constant.ErrMissingFieldsInRequest.Error(),
			wantMessage: "scope at index 0: scheme " + schemeFormatHint,
		},
		{
			name:        "overlong scheme",
			scope:       schemeScope(nil, &overlongScheme),
			wantCode:    constant.ErrMissingFieldsInRequest.Error(),
			wantMessage: "scope at index 0: scheme " + schemeFormatHint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := CreateLimitInput{
				Name:      "Scheme Limit",
				LimitType: model.LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("100"),
				Asset:     "USD",
				Scopes:    []model.Scope{tt.scope},
			}

			err := input.Validate()
			if tt.wantCode != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantCode)

				if tt.wantMessage != "" {
					assert.Contains(t, err.Error(), tt.wantMessage)
				}

				return
			}

			require.NoError(t, err)
			require.NotNil(t, input.Scopes[0].TransactionType)
			assert.Equal(t, model.TransactionType(tt.want), *input.Scopes[0].TransactionType)
			require.NotNil(t, input.Scopes[0].Scheme)
			assert.Equal(t, tt.want, *input.Scopes[0].Scheme)
		})
	}
}

func TestUpdateLimitInput_Validate_NormalizesScopeScheme(t *testing.T) {
	t.Parallel()

	scopes := []model.Scope{schemeScope(nil, testutil.StringPtr("pix"))}
	input := UpdateLimitInput{Scopes: &scopes}

	require.NoError(t, input.Validate())
	require.NotNil(t, (*input.Scopes)[0].TransactionType)
	assert.Equal(t, model.TransactionTypePix, *(*input.Scopes)[0].TransactionType)
	require.NotNil(t, (*input.Scopes)[0].Scheme)
	assert.Equal(t, "PIX", *(*input.Scopes)[0].Scheme)
}

func TestToLimitScopeJSONFieldName_Scheme(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "scheme", toLimitScopeJSONFieldName("Scheme"))
}

// doHumaRequest sends one request through app and returns status and decoded body.
func doHumaRequest(t *testing.T, app *fiber.App, method, target string, body []byte) (int, map[string]any) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req := httptest.NewRequest(method, target, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(respBody, &got), "body JSON: %s", string(respBody))

	return resp.StatusCode, got
}

// echoScopesRuleService returns the created rule with the scopes the handler
// handed the service, standing in for the stored rule.
type echoScopesRuleService struct {
	*tenantSpyService
}

func (s *echoScopesRuleService) CreateRule(ctx context.Context, input *command.CreateRuleInput) (*model.Rule, error) {
	rule, err := s.tenantSpyService.CreateRule(ctx, input)
	if err != nil {
		return nil, err
	}

	echoed := *rule
	echoed.Scopes = input.Scopes

	return &echoed, nil
}

func TestHuma_CreateRule_SchemeScopeIsNormalized(t *testing.T) {
	// NOT parallel: buildHumaRuleApp mutates process-global huma state.
	svc := &tenantSpyService{createResult: &model.Rule{
		ID:         testutil.MustDeterministicUUID(1),
		Name:       "Pix Rule",
		Expression: `scheme == "PIX"`,
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusDraft,
		CreatedAt:  testutil.FixedTime(),
		UpdatedAt:  testutil.FixedTime(),
	}}
	app := buildHumaRuleApp(t, &echoScopesRuleService{tenantSpyService: svc}, "tenant-alpha")

	body, err := json.Marshal(map[string]any{
		"name":       "Pix Rule",
		"expression": `scheme == "PIX"`,
		"action":     "DENY",
		"scopes":     []map[string]any{{"scheme": "pix"}},
	})
	require.NoError(t, err)

	status, got := doHumaRequest(t, app, http.MethodPost, "/v1/rules", body)
	require.Equal(t, http.StatusCreated, status, "body: %v", got)

	require.NotNil(t, svc.createInput)
	require.Len(t, svc.createInput.Scopes, 1)
	require.NotNil(t, svc.createInput.Scopes[0].TransactionType)
	assert.Equal(t, model.TransactionTypePix, *svc.createInput.Scopes[0].TransactionType)
	require.NotNil(t, svc.createInput.Scopes[0].Scheme)
	assert.Equal(t, "PIX", *svc.createInput.Scopes[0].Scheme)

	scopes, ok := got["scopes"].([]any)
	require.True(t, ok, "scopes must be an array: %v", got["scopes"])
	require.Len(t, scopes, 1)

	scope, ok := scopes[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "PIX", scope["scheme"])
	assert.Equal(t, "PIX", scope["transactionType"])
}

func TestHuma_CreateRule_SchemeAliasConflict(t *testing.T) {
	svc := &tenantSpyService{createResult: &model.Rule{ID: testutil.MustDeterministicUUID(1)}}
	app := buildHumaRuleApp(t, svc, "tenant-alpha")

	body, err := json.Marshal(map[string]any{
		"name":       "Conflict Rule",
		"expression": "amount > 0",
		"action":     "DENY",
		"scopes":     []map[string]any{{"scheme": "CARD", "transactionType": "PIX"}},
	})
	require.NoError(t, err)

	status, got := doHumaRequest(t, app, http.MethodPost, "/v1/rules", body)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, constant.ErrValidationSchemeAliasConflict.Error(), got["code"])
	assert.Nil(t, svc.createInput, "service must not be reached")
}

func TestHuma_CreateLimit_SchemeScopeIsNormalized(t *testing.T) {
	svc := &tenantSpyLimitService{createResult: validLimit(testutil.MustDeterministicUUID(1))}
	app := buildHumaLimitApp(t, svc, "tenant-alpha")

	body, err := json.Marshal(map[string]any{
		"name":      "Pix Cap",
		"limitType": "DAILY",
		"maxAmount": "1000.00",
		"asset":     "USD",
		"scopes":    []map[string]any{{"transactionType": "pix"}},
	})
	require.NoError(t, err)

	status, got := doHumaRequest(t, app, http.MethodPost, "/v1/limits", body)
	require.Equal(t, http.StatusCreated, status, "body: %v", got)

	require.NotNil(t, svc.createInput)
	require.Len(t, svc.createInput.Scopes, 1)
	require.NotNil(t, svc.createInput.Scopes[0].TransactionType)
	assert.Equal(t, model.TransactionTypePix, *svc.createInput.Scopes[0].TransactionType)
	require.NotNil(t, svc.createInput.Scopes[0].Scheme)
	assert.Equal(t, "PIX", *svc.createInput.Scopes[0].Scheme)
}

func TestHuma_ListRules_SchemeFilter(t *testing.T) {
	for _, tc := range []struct {
		name, query, want, code string
	}{
		{name: "scheme is normalized", query: "scheme=boleto", want: "BOLETO"},
		{name: "deprecated alias is normalized", query: "transaction_type=pix", want: "PIX"},
		{name: "matching alias", query: "transaction_type=PIX&scheme=pix", want: "PIX"},
		{name: "conflicting alias", query: "transaction_type=PIX&scheme=CARD", code: "0539"},
		{name: "invalid scheme", query: "scheme=bad%20value!", code: "0082"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyService{listResult: &model.ListRulesResult{}}
			app := buildHumaRuleApp(t, svc, "tenant-alpha")

			status, got := doHumaRequest(t, app, http.MethodGet, "/v1/rules?"+tc.query, nil)
			if tc.code != "" {
				assert.Equal(t, http.StatusBadRequest, status)
				assert.Equal(t, tc.code, got["code"])
				assert.Nil(t, svc.listFilter, "service must not be reached")

				return
			}

			require.Equal(t, http.StatusOK, status, "body: %v", got)
			require.NotNil(t, svc.listFilter)
			require.NotNil(t, svc.listFilter.ScopeFilter)
			require.NotNil(t, svc.listFilter.ScopeFilter.TransactionType)
			assert.Equal(t, model.TransactionType(tc.want), *svc.listFilter.ScopeFilter.TransactionType)
		})
	}
}

func TestHuma_ListLimits_SchemeFilter(t *testing.T) {
	for _, tc := range []struct {
		name, query, want, code string
	}{
		{name: "scheme is normalized", query: "scheme=boleto", want: "BOLETO"},
		{name: "deprecated alias is normalized", query: "transaction_type=pix", want: "PIX"},
		{name: "conflicting alias", query: "transaction_type=PIX&scheme=CARD", code: "0539"},
		{name: "invalid scheme", query: "scheme=bad%20value!", code: "0082"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyLimitService{listResult: &model.ListLimitsResult{}}
			app := buildHumaLimitApp(t, svc, "tenant-alpha")

			status, got := doHumaRequest(t, app, http.MethodGet, "/v1/limits?"+tc.query, nil)
			if tc.code != "" {
				assert.Equal(t, http.StatusBadRequest, status)
				assert.Equal(t, tc.code, got["code"])
				assert.Nil(t, svc.listFilter, "service must not be reached")

				return
			}

			require.Equal(t, http.StatusOK, status, "body: %v", got)
			require.NotNil(t, svc.listFilter)
			require.NotNil(t, svc.listFilter.ScopeFilter)
			require.NotNil(t, svc.listFilter.ScopeFilter.TransactionType)
			assert.Equal(t, model.TransactionType(tc.want), *svc.listFilter.ScopeFilter.TransactionType)
		})
	}
}

func TestHuma_ListTransactionValidations_SchemeFilter(t *testing.T) {
	for _, tc := range []struct {
		name, query, want, code string
	}{
		{name: "scheme is normalized", query: "scheme=boleto", want: "BOLETO"},
		{name: "deprecated alias is normalized", query: "transaction_type=pix", want: "PIX"},
		{name: "conflicting alias", query: "transaction_type=PIX&scheme=CARD", code: "0539"},
		{name: "invalid scheme", query: "scheme=bad%20value!", code: "0431"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tvTenantSpyService{listResult: &query.ListTransactionValidationsResult{}}
			app := buildHumaTransactionValidationApp(t, svc, "tenant-alpha")

			status, got := doHumaRequest(t, app, http.MethodGet, "/v1/validations?"+tc.query, nil)
			if tc.code != "" {
				assert.Equal(t, http.StatusBadRequest, status)
				assert.Equal(t, tc.code, got["code"])
				assert.Nil(t, svc.listFilters, "service must not be reached")

				return
			}

			require.Equal(t, http.StatusOK, status, "body: %v", got)
			require.NotNil(t, svc.listFilters)
			require.NotNil(t, svc.listFilters.TransactionType)
			require.NotNil(t, svc.listFilters.TransactionType)
			assert.Equal(t, model.TransactionType(tc.want), *svc.listFilters.TransactionType)
		})
	}
}

func TestHuma_ListAuditEvents_SchemeFilter(t *testing.T) {
	for _, tc := range []struct {
		name, query, want, code string
	}{
		{name: "scheme is normalized", query: "scheme=boleto", want: "BOLETO"},
		{name: "deprecated alias is normalized", query: "transaction_type=pix", want: "PIX"},
		{name: "conflicting alias", query: "transaction_type=PIX&scheme=CARD", code: "0539"},
		{name: "invalid scheme", query: "scheme=bad%20value!", code: "0009"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyAuditEventService{listResult: &model.ListAuditEventsResult{}}
			app := buildHumaAuditEventApp(t, svc, "tenant-alpha")

			status, got := doHumaRequest(t, app, http.MethodGet, "/v1/audit-events?"+tc.query, nil)
			if tc.code != "" {
				assert.Equal(t, http.StatusBadRequest, status)
				assert.Equal(t, tc.code, got["code"])
				assert.Nil(t, svc.listFilter, "service must not be reached")

				return
			}

			require.Equal(t, http.StatusOK, status, "body: %v", got)
			require.NotNil(t, svc.listFilter)
			require.NotNil(t, svc.listFilter.TransactionType)
			require.NotNil(t, svc.listFilter.TransactionType)
			assert.Equal(t, model.TransactionType(tc.want), *svc.listFilter.TransactionType)
		})
	}
}

func TestHuma_Validate_SchemeOnlyBody(t *testing.T) {
	resp := &model.ValidationResponse{
		ValidationID: testutil.MustDeterministicUUID(10),
		RequestID:    testutil.MustDeterministicUUID(1),
		EvaluationResult: model.EvaluationResult{
			Decision:       model.DecisionAllow,
			MatchedRuleIDs: []uuid.UUID{},
			Reason:         "No matching rules found",
		},
		LimitUsageDetails: []model.LimitUsageDetail{},
	}
	svc := &validationSpyService{result: &services.ValidateResult{Response: resp}}
	app := buildHumaValidationApp(t, svc, "tenant-alpha")

	body, err := json.Marshal(map[string]any{
		"requestId":            testutil.MustDeterministicUUID(1).String(),
		"scheme":               "pix",
		"amount":               "100",
		"asset":                "USD",
		"transactionTimestamp": testutil.FixedTime(),
		"account":              map[string]any{"accountId": testutil.MustDeterministicUUID(2).String()},
	})
	require.NoError(t, err)

	status, got := doHumaRequest(t, app, http.MethodPost, "/v1/validations", body)
	require.Equal(t, http.StatusCreated, status, "body: %v", got)

	require.NotNil(t, svc.captured)
	assert.Equal(t, model.TransactionTypePix, svc.captured.TransactionType)
	assert.Equal(t, "PIX", svc.captured.Scheme)
}

func TestToValidationSummary_CarriesScheme(t *testing.T) {
	t.Parallel()

	summary := ToValidationSummary(&model.TransactionValidation{
		ID:              testutil.MustDeterministicUUID(1),
		TransactionType: model.TransactionType("BOLETO"),
		CreatedAt:       testutil.FixedTime(),
	})

	require.NotNil(t, summary)
	assert.Equal(t, model.TransactionType("BOLETO"), summary.TransactionType)
	assert.Equal(t, model.TransactionType("BOLETO"), summary.Scheme)
}
