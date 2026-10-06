// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// newSchemeRequest builds a request carrying only the scheme, normalized the
// way the HTTP and reserve entry points normalize it.
func newSchemeRequest(t *testing.T, scheme string) *model.ValidationRequest {
	t.Helper()

	req := &model.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1),
		Scheme:               scheme,
		Amount:               decimal.RequireFromString("1500"),
		Asset:                "BRL",
		TransactionTimestamp: testTimestamp,
		Account:              model.AccountContext{ID: testAccountID},
	}
	require.NoError(t, req.NormalizeAndValidate(testTimestamp))

	return req
}

func TestEvaluate_SchemeVariable(t *testing.T) {
	adapter := newTestAdapter(t)

	tests := []struct {
		name       string
		expression string
		scheme     string
		expected   bool
	}{
		{name: "scheme matches a request sent in lower case", expression: `scheme == "PIX"`, scheme: "pix", expected: true},
		{name: "transactionType still matches", expression: `transactionType == "PIX"`, scheme: "pix", expected: true},
		{name: "scheme is always upper case", expression: `scheme == "pix"`, scheme: "pix", expected: false},
		{name: "scheme and transactionType carry the same value", expression: `scheme == transactionType`, scheme: " boleto ", expected: true},
		{name: "open scheme", expression: `scheme in ["BOLETO", "TED"]`, scheme: "boleto", expected: true},
		{name: "other scheme does not match", expression: `scheme == "PIX"`, scheme: "card", expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			program := compileForEval(t, adapter, tc.expression)

			result, err := adapter.Evaluate(context.Background(), program, newSchemeRequest(t, tc.scheme))

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestBuildActivation_BindsSchemeToTransactionType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		transactionType model.TransactionType
		scheme          string
		want            string
	}{
		{name: "transaction type only", transactionType: model.TransactionTypePix, want: "PIX"},
		{name: "scheme only", scheme: "BOLETO", want: "BOLETO"},
		{name: "both", transactionType: model.TransactionTypeCard, scheme: "CARD", want: "CARD"},
		{name: "neither", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := newTestRequest()
			req.TransactionType = tc.transactionType
			req.Scheme = tc.scheme

			activation, err := BuildActivation(req)

			require.NoError(t, err)
			assert.Equal(t, tc.want, activation["scheme"])
			assert.Equal(t, tc.want, activation["transactionType"])
		})
	}
}

func TestTransactionTypeExpressions_IncludeSchemeExample(t *testing.T) {
	t.Parallel()

	var found bool

	for _, example := range TransactionTypeExpressions {
		if example.Expression == `scheme == "PIX"` {
			found = true
		}
	}

	assert.True(t, found, `TransactionTypeExpressions must document scheme == "PIX"`)
}
