// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func schemeTestRequest(transactionType TransactionType, schemeValue string) *ValidationRequest {
	return &ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(2),
		TransactionType:      transactionType,
		Scheme:               schemeValue,
		Amount:               decimal.RequireFromString("100"),
		Asset:                "USD",
		TransactionTimestamp: testutil.FixedTime(),
		Account:              AccountContext{ID: testutil.MustDeterministicUUID(1)},
	}
}

func TestValidationRequest_NormalizeAndValidate_SchemeAlias_TableCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		transactionType TransactionType
		scheme          string
		forReserve      bool
		want            TransactionType
		wantErr         error
	}{
		{name: "only scheme", scheme: "BOLETO", want: "BOLETO"},
		{name: "only transactionType", transactionType: "BOLETO", want: "BOLETO"},
		{name: "both equal", transactionType: "PIX", scheme: "PIX", want: "PIX"},
		{name: "both equal after normalization", transactionType: "pix", scheme: " PIX ", want: "PIX"},
		{name: "padded lowercase is normalized in both fields", scheme: " boleto ", want: "BOLETO"},
		{name: "both different", transactionType: "PIX", scheme: "BOLETO", wantErr: constant.ErrValidationSchemeAliasConflict},
		{name: "both different on reserve", transactionType: "PIX", scheme: "BOLETO", forReserve: true, wantErr: constant.ErrValidationSchemeAliasConflict},
		{name: "both empty on validate", wantErr: constant.ErrValidationInvalidTransactionType},
		{name: "both empty on reserve", forReserve: true, want: ""},
		{name: "whitespace only on validate", scheme: "   ", wantErr: constant.ErrValidationInvalidTransactionType},
		{name: "invalid scheme", scheme: "bad value!", wantErr: constant.ErrValidationInvalidTransactionType},
		{name: "invalid scheme on reserve", scheme: "bad value!", forReserve: true, wantErr: constant.ErrValidationInvalidTransactionType},
		{name: "51 characters", transactionType: TransactionType(strings.Repeat("A", 51)), wantErr: constant.ErrValidationInvalidTransactionType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := schemeTestRequest(tt.transactionType, tt.scheme)

			var err error
			if tt.forReserve {
				err = req.NormalizeAndValidateForReserve(testutil.FixedTime())
			} else {
				err = req.NormalizeAndValidate(testutil.FixedTime())
			}

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, tt.transactionType, req.TransactionType, "receiver must stay untouched on failure")
				assert.Equal(t, tt.scheme, req.Scheme, "receiver must stay untouched on failure")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, req.TransactionType)
			assert.Equal(t, string(tt.want), req.Scheme)
		})
	}
}

func TestValidationRequest_Validate_SchemeAlias_TableCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		transactionType TransactionType
		scheme          string
		forReserve      bool
		wantErr         error
	}{
		{name: "only scheme satisfies the requirement", scheme: "BOLETO"},
		{name: "only transactionType satisfies the requirement", transactionType: "BOLETO"},
		{name: "both empty is missing", wantErr: constant.ErrValidationInvalidTransactionType},
		{name: "both empty is allowed on reserve", forReserve: true},
		{name: "conflict", transactionType: "PIX", scheme: "BOLETO", wantErr: constant.ErrValidationSchemeAliasConflict},
		{name: "non-canonical scheme", scheme: "boleto", wantErr: constant.ErrValidationInvalidTransactionType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := schemeTestRequest(tt.transactionType, tt.scheme)

			var err error
			if tt.forReserve {
				err = req.ValidateForReserve(testutil.FixedTime())
			} else {
				err = req.Validate(testutil.FixedTime())
			}

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestNewValidationRequest_NormalizesScheme(t *testing.T) {
	t.Parallel()

	req, err := NewValidationRequest(testutil.FixedTime(), testutil.MustDeterministicUUID(2), TransactionType(" boleto "), nil,
		decimal.RequireFromString("100"), "USD", testutil.FixedTime(), AccountContext{ID: testutil.MustDeterministicUUID(1)},
		nil, nil, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, TransactionType("BOLETO"), req.TransactionType)
	assert.Equal(t, "BOLETO", req.Scheme)
}

func TestValidationRequest_JSON_SchemeIsOptional(t *testing.T) {
	t.Parallel()

	var req ValidationRequest

	require.NoError(t, json.Unmarshal([]byte(`{"scheme":"BOLETO"}`), &req))
	assert.Equal(t, "BOLETO", req.Scheme)
	assert.Equal(t, TransactionType(""), req.TransactionType)
}

func TestTransactionValidation_JSON_SchemeMirrorsTransactionType(t *testing.T) {
	t.Parallel()

	tv, err := NewTransactionValidation(testutil.MustDeterministicUUID(3), DecisionAllow, testutil.FixedTime())
	require.NoError(t, err)

	tv.TransactionType = "BOLETO"

	raw, err := json.Marshal(tv)
	require.NoError(t, err)

	var wire map[string]any

	require.NoError(t, json.Unmarshal(raw, &wire))
	assert.Equal(t, "BOLETO", wire["scheme"])
	assert.Equal(t, "BOLETO", wire["transactionType"])
	assert.Equal(t, "ALLOW", wire["decision"], "embedded evaluation result must still be flattened")
}
