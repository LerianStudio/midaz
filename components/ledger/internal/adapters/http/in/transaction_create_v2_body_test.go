// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func TestNormalizeCreateTransactionV2Body_PublicTranslateParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pending bool
	}{
		{name: "direct"},
		{name: "hold", pending: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			input := validV2Input()

			normalized, err := normalizeCreateTransactionV2Body(input, testCase.pending)
			require.NoError(t, err)

			transaction, scope, err := input.Translate(testCase.pending)
			require.NoError(t, err)

			assert.Equal(t, transaction, normalized.transaction)
			assert.Equal(t, scope, normalized.scope)
		})
	}
}

func TestNormalizeCreateTransactionV2Body_PreservesSingularRulePrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		mutate         func(*CreateTransactionV2Request)
		wantCode       error
		wantStatus     int
		wantDetailPart string
	}{
		{
			name: "debit side precedes credit side and amount",
			mutate: func(input *CreateTransactionV2Request) {
				input.Debits = nil
				input.Credits = nil
				input.Amount = "not-a-number"
			},
			wantCode:       constant.ErrMissingFieldsInRequest,
			wantStatus:     http.StatusBadRequest,
			wantDetailPart: "debits",
		},
		{
			name: "transaction amount precedes debit leg",
			mutate: func(input *CreateTransactionV2Request) {
				input.Amount = "not-a-number"
				input.Debits[0].Alias = "invalid alias"
			},
			wantCode:   constant.ErrInvalidTransactionNonPositiveValue,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "debit leg precedes credit leg",
			mutate: func(input *CreateTransactionV2Request) {
				input.Debits[0].Alias = "invalid alias"
				input.Credits[0].Alias = ""
			},
			wantCode:   constant.ErrAccountAliasInvalid,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "credit leg precedes scope completeness",
			mutate: func(input *CreateTransactionV2Request) {
				input.Debits[0].OrganizationID = ""
				input.Credits[0].Alias = ""
			},
			wantCode:       constant.ErrMissingFieldsInRequest,
			wantStatus:     http.StatusBadRequest,
			wantDetailPart: "credits[0].alias",
		},
		{
			name: "debit scope precedes credit scope",
			mutate: func(input *CreateTransactionV2Request) {
				input.Debits[0].OrganizationID = ""
				input.Credits[0].LedgerID = ""
			},
			wantCode:       constant.ErrMissingFieldsInRequest,
			wantStatus:     http.StatusBadRequest,
			wantDetailPart: "debits[0].organizationId",
		},
		{
			name: "scope mismatch remains the last body-only rule",
			mutate: func(input *CreateTransactionV2Request) {
				input.Credits[0].LedgerID = otherLedgerID
			},
			wantCode:   constant.ErrTransactionScopeMismatch,
			wantStatus: http.StatusUnprocessableEntity,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			input := validV2Input()
			testCase.mutate(&input)

			normalized, err := normalizeCreateTransactionV2Body(input, false)
			require.Error(t, err)
			assert.True(t, normalized.transaction.IsEmpty())
			assert.Equal(t, TransactionV2Scope{}, normalized.scope)

			problem := requireV2BodyProblem(t, err)
			assert.Equal(t, testCase.wantCode.Error(), problem.Code)
			assert.Equal(t, testCase.wantStatus, problem.Status)
			if testCase.wantDetailPart != "" {
				assert.Contains(t, problem.ErrorModel.Detail, testCase.wantDetailPart)
			}
		})
	}
}

func TestDecodeAndBuildV2Transaction_PreservesSingularValidationLayers(t *testing.T) {
	t.Parallel()

	validLegs := `"debits":[{"alias":"@srcA",` + scopeJSON + `,"amount":"100"}],` +
		`"credits":[{"alias":"@dstA",` + scopeJSON + `,"amount":"100"}]`

	tests := []struct {
		name       string
		body       string
		wantCode   error
		wantStatus int
	}{
		{
			name:       "unknown fields precede missing required tags",
			body:       `{"unexpected":true}`,
			wantCode:   constant.ErrUnexpectedFieldsInTheRequest,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "required tags precede pure amount normalization",
			body:       `{"amount":"not-a-number",` + validLegs + `}`,
			wantCode:   constant.ErrMissingFieldsInRequest,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "pure body rules retain their business wire error",
			body:       `{"asset":"BRL","amount":"not-a-number",` + validLegs + `}`,
			wantCode:   constant.ErrInvalidTransactionNonPositiveValue,
			wantStatus: http.StatusUnprocessableEntity,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			transaction, scope, exceptionID, err := decodeAndBuildV2Transaction([]byte(testCase.body), false, "")
			require.Error(t, err)
			assert.True(t, transaction.IsEmpty())
			assert.Equal(t, TransactionV2Scope{}, scope)
			assert.Nil(t, exceptionID)

			problem := requireV2BodyProblem(t, err)
			assert.Equal(t, testCase.wantCode.Error(), problem.Code)
			assert.Equal(t, testCase.wantStatus, problem.Status)
		})
	}
}

func requireV2BodyProblem(t *testing.T, err error) *pkgHTTP.Detail {
	t.Helper()

	problem, ok := pkgHTTP.HumaProblem(err).(*pkgHTTP.Detail)
	require.True(t, ok)

	return problem
}
