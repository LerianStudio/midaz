// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestNormalizeScopes_SchemeAlias_TableCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		transactionType *TransactionType
		scheme          *string
		want            *TransactionType
		wantErr         error
	}{
		{name: "only scheme fills transactionType", scheme: testutil.Ptr(" boleto "), want: testutil.Ptr(TransactionType("BOLETO"))},
		{name: "only transactionType fills scheme", transactionType: testutil.Ptr(TransactionType("pix")), want: testutil.Ptr(TransactionTypePix)},
		{name: "both equal after normalization", transactionType: testutil.Ptr(TransactionType("PIX")), scheme: testutil.Ptr("pix"), want: testutil.Ptr(TransactionTypePix)},
		{name: "neither set", want: nil},
		{name: "both different", transactionType: testutil.Ptr(TransactionTypePix), scheme: testutil.Ptr("BOLETO"), wantErr: constant.ErrValidationSchemeAliasConflict},
		{name: "invalid scheme", scheme: testutil.Ptr("bad value!"), wantErr: constant.ErrLimitInvalidScope},
		{name: "empty scheme", scheme: testutil.Ptr(""), wantErr: constant.ErrLimitInvalidScope},
		{name: "51 character transactionType", transactionType: testutil.Ptr(TransactionType(strings.Repeat("A", 51))), wantErr: constant.ErrLimitInvalidScope},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scopes := []Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1)), TransactionType: tt.transactionType, Scheme: tt.scheme}}

			err := NormalizeScopes(scopes)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			if tt.want == nil {
				assert.Nil(t, scopes[0].TransactionType)
				assert.Nil(t, scopes[0].Scheme)

				return
			}

			require.NotNil(t, scopes[0].TransactionType)
			require.NotNil(t, scopes[0].Scheme)
			assert.Equal(t, *tt.want, *scopes[0].TransactionType)
			assert.Equal(t, string(*tt.want), *scopes[0].Scheme)
		})
	}
}

func TestNormalizeScopes_DoesNotWriteThroughCallerPointers(t *testing.T) {
	t.Parallel()

	raw := " boleto "
	scopes := []Scope{{Scheme: &raw}}

	require.NoError(t, NormalizeScopes(scopes))
	assert.Equal(t, " boleto ", raw)
	assert.Equal(t, "BOLETO", *scopes[0].Scheme)
}

func TestScope_SchemeOnly_IsNotEmpty(t *testing.T) {
	t.Parallel()

	s := Scope{Scheme: testutil.Ptr("BOLETO")}
	assert.False(t, s.IsEmpty())
}

func TestScope_ToMap_IncludesScheme(t *testing.T) {
	t.Parallel()

	s := Scope{TransactionType: testutil.Ptr(TransactionType("BOLETO")), Scheme: testutil.Ptr("BOLETO")}
	got := s.ToMap()

	assert.Equal(t, "BOLETO", got["transactionType"])
	assert.Equal(t, "BOLETO", got["scheme"])
}

func TestCloneAndNormalizeScope_CopiesScheme(t *testing.T) {
	t.Parallel()

	original := "BOLETO"
	src := Scope{Scheme: &original}

	cloned := cloneAndNormalizeScope(src)

	require.NotNil(t, cloned.Scheme)
	assert.NotSame(t, src.Scheme, cloned.Scheme)

	original = "PIX"

	assert.Equal(t, "BOLETO", *cloned.Scheme)
}

func TestScope_Matches_FreeFormScheme_TableCases(t *testing.T) {
	t.Parallel()

	boleto := TransactionType("BOLETO")
	pix := TransactionTypePix
	pattern := Scope{TransactionType: &boleto, Scheme: testutil.Ptr("BOLETO")}

	tests := []struct {
		name  string
		other Scope
		want  bool
	}{
		{name: "same scheme matches", other: Scope{TransactionType: &boleto}, want: true},
		{name: "different scheme does not match", other: Scope{TransactionType: &pix}, want: false},
		{name: "absent scheme does not match", other: Scope{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, pattern.Matches(&tt.other))
		})
	}
}

func TestNewLimit_ScopeSchemeAlias_TableCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		scope   Scope
		want    TransactionType
		wantErr error
	}{
		{name: "scheme only is stored in both fields", scope: Scope{Scheme: testutil.Ptr(" boleto ")}, want: "BOLETO"},
		{name: "lowercase transactionType is canonicalized", scope: Scope{TransactionType: testutil.Ptr(TransactionType("pix"))}, want: TransactionTypePix},
		{name: "conflict", scope: Scope{TransactionType: testutil.Ptr(TransactionTypePix), Scheme: testutil.Ptr("BOLETO")}, wantErr: constant.ErrValidationSchemeAliasConflict},
		{name: "invalid scheme", scope: Scope{Scheme: testutil.Ptr("bad value!")}, wantErr: constant.ErrLimitInvalidScope},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			limit, err := NewLimit("Limit", LimitTypeDaily, decimal.RequireFromString("1000"), "USD", []Scope{tt.scope}, nil, testutil.FixedTime())
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, limit.Scopes[0].TransactionType)
			require.NotNil(t, limit.Scopes[0].Scheme)
			assert.Equal(t, tt.want, *limit.Scopes[0].TransactionType)
			assert.Equal(t, string(tt.want), *limit.Scopes[0].Scheme)
		})
	}
}

func TestLimit_Update_ScopeSchemeAlias_TableCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		scope   Scope
		want    TransactionType
		wantErr error
	}{
		{name: "scheme only is stored in both fields", scope: Scope{Scheme: testutil.Ptr("boleto")}, want: "BOLETO"},
		{name: "conflict", scope: Scope{TransactionType: testutil.Ptr(TransactionTypePix), Scheme: testutil.Ptr("BOLETO")}, wantErr: constant.ErrValidationSchemeAliasConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			limit, err := NewLimit("Limit", LimitTypeDaily, decimal.RequireFromString("1000"), "USD",
				[]Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1))}}, nil, testutil.FixedTime())
			require.NoError(t, err)

			scopes := []Scope{tt.scope}

			err = limit.Update(nil, nil, nil, &scopes, nil, nil, nil, nil, testutil.FixedTime())
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.NotNil(t, limit.Scopes[0].AccountID, "scopes must stay untouched on failure")

				return
			}

			require.NoError(t, err)
			require.NotNil(t, limit.Scopes[0].TransactionType)
			assert.Equal(t, tt.want, *limit.Scopes[0].TransactionType)
			assert.Equal(t, string(tt.want), *limit.Scopes[0].Scheme)
		})
	}
}

func TestNewRule_ScopeSchemeAlias_TableCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		scope   Scope
		want    TransactionType
		wantErr error
	}{
		{name: "scheme only is stored in both fields", scope: Scope{Scheme: testutil.Ptr(" boleto ")}, want: "BOLETO"},
		{name: "conflict", scope: Scope{TransactionType: testutil.Ptr(TransactionTypePix), Scheme: testutil.Ptr("BOLETO")}, wantErr: constant.ErrValidationSchemeAliasConflict},
		{name: "invalid scheme", scope: Scope{Scheme: testutil.Ptr("bad value!")}, wantErr: constant.ErrRuleInvalidScope},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rule, err := NewRule("Rule", "amount > 0", DecisionDeny, []Scope{tt.scope}, nil, testutil.FixedTime())
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, rule.Scopes[0].TransactionType)
			assert.Equal(t, tt.want, *rule.Scopes[0].TransactionType)
			assert.Equal(t, string(tt.want), *rule.Scopes[0].Scheme)
		})
	}
}

func TestRule_Update_ScopeSchemeAlias_TableCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		scope   Scope
		want    TransactionType
		wantErr error
	}{
		{name: "scheme only is stored in both fields", scope: Scope{Scheme: testutil.Ptr("boleto")}, want: "BOLETO"},
		{name: "conflict", scope: Scope{TransactionType: testutil.Ptr(TransactionTypePix), Scheme: testutil.Ptr("BOLETO")}, wantErr: constant.ErrValidationSchemeAliasConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rule, err := NewRule("Rule", "amount > 0", DecisionDeny, nil, nil, testutil.FixedTime())
			require.NoError(t, err)

			scopes := []Scope{tt.scope}

			err = rule.Update(nil, nil, nil, &scopes, testutil.FixedTime())
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, rule.Scopes, "scopes must stay untouched on failure")

				return
			}

			require.NoError(t, err)
			require.NotNil(t, rule.Scopes[0].TransactionType)
			assert.Equal(t, tt.want, *rule.Scopes[0].TransactionType)
			assert.Equal(t, string(tt.want), *rule.Scopes[0].Scheme)
		})
	}
}
