// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	libHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// baseInstant is the fixed reference point every validity-window case is derived
// from. A frozen instant keeps the table deterministic: the boundary case
// (expiresAt == effectiveAt) has to compare exactly equal, which a time.Now()
// derived table cannot guarantee.
var baseInstant = time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

// TestValidateAccountExceptionWindow exercises the cross-field validity-window
// rule of the always-valid model: an exception whose expiration does not sit
// strictly after its start is rejected with 0505. Every partial combination
// (only one bound, or neither) is accepted — RF-09 makes an absent expiresAt mean
// "indeterminate validity", and an absent effectiveAt mean "effective now".
func TestValidateAccountExceptionWindow(t *testing.T) {
	t.Parallel()

	before := baseInstant.Add(-time.Hour)
	after := baseInstant.Add(time.Hour)

	tests := []struct {
		name        string
		effectiveAt *time.Time
		expiresAt   *time.Time
		wantErr     bool
	}{
		{name: "both absent is accepted", effectiveAt: nil, expiresAt: nil, wantErr: false},
		{name: "only effectiveAt is accepted", effectiveAt: &baseInstant, expiresAt: nil, wantErr: false},
		{name: "only expiresAt is accepted", effectiveAt: nil, expiresAt: &baseInstant, wantErr: false},
		{name: "expiresAt strictly after effectiveAt is accepted", effectiveAt: &baseInstant, expiresAt: &after, wantErr: false},
		{name: "expiresAt equal to effectiveAt is rejected", effectiveAt: &baseInstant, expiresAt: &baseInstant, wantErr: true},
		{name: "expiresAt before effectiveAt is rejected", effectiveAt: &baseInstant, expiresAt: &before, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := mmodel.ValidateAccountExceptionWindow(tt.effectiveAt, tt.expiresAt)

			if !tt.wantErr {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)

			var vErr pkg.ValidationError

			require.Truef(t, errors.As(err, &vErr), "expected pkg.ValidationError, got %T", err)
			require.Equal(t, cn.ErrInvalidAccountExceptionValidityWindow.Error(), vErr.Code)
			require.Equal(t, cn.EntityAccountException, vErr.EntityType)
			require.NotEmpty(t, vErr.Title, "ValidationError.Title must not be blank")
			require.NotEmpty(t, vErr.Message, "ValidationError.Message must not be blank")
		})
	}
}

// TestValidateAccountExceptionWindow_DistinctTimeZonesSameInstant pins that the
// comparison is on the INSTANT, not on the wall clock: the same moment expressed
// in two zones must still be rejected as a non-positive window.
func TestValidateAccountExceptionWindow_DistinctTimeZonesSameInstant(t *testing.T) {
	t.Parallel()

	utc := baseInstant
	shifted := baseInstant.In(time.FixedZone("UTC-3", -3*60*60))

	err := mmodel.ValidateAccountExceptionWindow(&utc, &shifted)

	require.Error(t, err, "the same instant in another zone is still a zero-length window")

	var vErr pkg.ValidationError

	require.True(t, errors.As(err, &vErr))
	require.Equal(t, cn.ErrInvalidAccountExceptionValidityWindow.Error(), vErr.Code)
}

// TestCreateAccountExceptionInputValidation drives the REAL production validation
// path (http.ValidateStruct) over CreateAccountExceptionInput, covering every tag
// the TRD 4 requires: at least one operational type code, each code non-blank and
// bounded, an optional but bounded balance key, and a required bounded context.
func TestCreateAccountExceptionInputValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   mmodel.CreateAccountExceptionInput
		wantErr bool
	}{
		{
			name: "minimal valid input",
			input: mmodel.CreateAccountExceptionInput{
				OperationalTypeCodes: []string{"PIX_IN"},
				Context:              "Court order 12345",
			},
			wantErr: false,
		},
		{
			name: "valid input with balance key and window",
			input: mmodel.CreateAccountExceptionInput{
				OperationalTypeCodes: []string{"PIX_IN", "TED_OUT"},
				BalanceKey:           testutils.Ptr("asset-freeze"),
				Context:              "Court order 12345",
				EffectiveAt:          &baseInstant,
				ExpiresAt:            testutils.Ptr(baseInstant.Add(24 * time.Hour)),
			},
			wantErr: false,
		},
		{
			name: "nil operational type codes is rejected",
			input: mmodel.CreateAccountExceptionInput{
				Context: "Court order 12345",
			},
			wantErr: true,
		},
		{
			name: "empty operational type codes is rejected",
			input: mmodel.CreateAccountExceptionInput{
				OperationalTypeCodes: []string{},
				Context:              "Court order 12345",
			},
			wantErr: true,
		},
		{
			name: "operational type code with whitespace is rejected",
			input: mmodel.CreateAccountExceptionInput{
				OperationalTypeCodes: []string{"PIX IN"},
				Context:              "Court order 12345",
			},
			wantErr: true,
		},
		{
			name: "operational type code over 100 chars is rejected",
			input: mmodel.CreateAccountExceptionInput{
				OperationalTypeCodes: []string{strings.Repeat("A", 101)},
				Context:              "Court order 12345",
			},
			wantErr: true,
		},
		{
			name: "operational type code at exactly 100 chars is accepted",
			input: mmodel.CreateAccountExceptionInput{
				OperationalTypeCodes: []string{strings.Repeat("A", 100)},
				Context:              "Court order 12345",
			},
			wantErr: false,
		},
		{
			name: "missing context is rejected",
			input: mmodel.CreateAccountExceptionInput{
				OperationalTypeCodes: []string{"PIX_IN"},
			},
			wantErr: true,
		},
		{
			name: "context over 256 chars is rejected",
			input: mmodel.CreateAccountExceptionInput{
				OperationalTypeCodes: []string{"PIX_IN"},
				Context:              strings.Repeat("c", 257),
			},
			wantErr: true,
		},
		{
			name: "context at exactly 256 chars is accepted",
			input: mmodel.CreateAccountExceptionInput{
				OperationalTypeCodes: []string{"PIX_IN"},
				Context:              strings.Repeat("c", 256),
			},
			wantErr: false,
		},
		{
			name: "balance key with whitespace is rejected",
			input: mmodel.CreateAccountExceptionInput{
				OperationalTypeCodes: []string{"PIX_IN"},
				BalanceKey:           testutils.Ptr("asset freeze"),
				Context:              "Court order 12345",
			},
			wantErr: true,
		},
		{
			name: "balance key over 100 chars is rejected",
			input: mmodel.CreateAccountExceptionInput{
				OperationalTypeCodes: []string{"PIX_IN"},
				BalanceKey:           testutils.Ptr(strings.Repeat("k", 101)),
				Context:              "Court order 12345",
			},
			wantErr: true,
		},
		{
			name: "empty balance key is rejected on create (no clear semantics here)",
			input: mmodel.CreateAccountExceptionInput{
				OperationalTypeCodes: []string{"PIX_IN"},
				BalanceKey:           testutils.Ptr(""),
				Context:              "Court order 12345",
			},
			wantErr: true,
		},
		{
			name: "absent balance key is accepted (scope is every balance)",
			input: mmodel.CreateAccountExceptionInput{
				OperationalTypeCodes: []string{"PIX_IN"},
				BalanceKey:           nil,
				Context:              "Court order 12345",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := libHTTP.ValidateStruct(&tt.input)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

// TestUpdateAccountExceptionInputValidation asserts PATCH semantics: every field
// is optional, so a fully empty payload validates, while a field that IS present
// still has to satisfy the same bounds as on create.
func TestUpdateAccountExceptionInputValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   mmodel.UpdateAccountExceptionInput
		wantErr bool
	}{
		{
			name:    "empty payload is accepted (nothing changes)",
			input:   mmodel.UpdateAccountExceptionInput{},
			wantErr: false,
		},
		{
			name: "populated payload is accepted",
			input: mmodel.UpdateAccountExceptionInput{
				OperationalTypeCodes: []string{"PIX_IN"},
				BalanceKey:           testutils.Ptr("asset-freeze"),
				Context:              testutils.Ptr("Updated court order"),
				ExpiresAt:            testutils.Ptr(baseInstant.Add(time.Hour)),
			},
			wantErr: false,
		},
		{
			name: "empty balance key is accepted and clears the restriction",
			input: mmodel.UpdateAccountExceptionInput{
				BalanceKey: testutils.Ptr(""),
			},
			wantErr: false,
		},
		{
			name: "present but empty operational type codes is rejected",
			input: mmodel.UpdateAccountExceptionInput{
				OperationalTypeCodes: []string{},
			},
			wantErr: true,
		},
		{
			name: "operational type code with whitespace is rejected",
			input: mmodel.UpdateAccountExceptionInput{
				OperationalTypeCodes: []string{"PIX IN"},
			},
			wantErr: true,
		},
		{
			name: "balance key with whitespace is rejected",
			input: mmodel.UpdateAccountExceptionInput{
				BalanceKey: testutils.Ptr("asset freeze"),
			},
			wantErr: true,
		},
		{
			name: "balance key with a tab is rejected",
			input: mmodel.UpdateAccountExceptionInput{
				BalanceKey: testutils.Ptr("asset\tfreeze"),
			},
			wantErr: true,
		},
		{
			name: "whitespace-only balance key is rejected (not the clear sentinel)",
			input: mmodel.UpdateAccountExceptionInput{
				BalanceKey: testutils.Ptr("   "),
			},
			wantErr: true,
		},
		{
			name: "balance key over 100 chars is rejected",
			input: mmodel.UpdateAccountExceptionInput{
				BalanceKey: testutils.Ptr(strings.Repeat("k", 101)),
			},
			wantErr: true,
		},
		{
			name: "context over 256 chars is rejected",
			input: mmodel.UpdateAccountExceptionInput{
				Context: testutils.Ptr(strings.Repeat("c", 257)),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := libHTTP.ValidateStruct(&tt.input)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

// TestAccountExceptionJSONContract pins the wire contract of the response model:
// camelCase keys, an always-present operationalTypeCodes array, and the two
// nil-means-something fields omitted when unset.
func TestAccountExceptionJSONContract(t *testing.T) {
	t.Parallel()

	exception := mmodel.AccountException{
		ID:                   "01965ed9-7fa4-75b2-8872-fc9e8509ab0a",
		OrganizationID:       "01965ed9-7fa4-75b2-8872-fc9e8509ab0b",
		LedgerID:             "01965ed9-7fa4-75b2-8872-fc9e8509ab0c",
		AccountID:            "01965ed9-7fa4-75b2-8872-fc9e8509ab0d",
		OperationalTypeCodes: []string{"PIX_IN"},
		Context:              "Court order 12345",
		CreatedAt:            baseInstant,
		UpdatedAt:            baseInstant,
	}

	data, err := json.Marshal(exception)
	require.NoError(t, err)

	var raw map[string]any

	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Equal(t, "01965ed9-7fa4-75b2-8872-fc9e8509ab0a", raw["id"])
	assert.Equal(t, "01965ed9-7fa4-75b2-8872-fc9e8509ab0d", raw["accountId"])
	assert.Equal(t, "Court order 12345", raw["context"])

	codes, hasCodes := raw["operationalTypeCodes"]
	require.True(t, hasCodes, "operationalTypeCodes must always be present in the response")
	assert.Equal(t, []any{"PIX_IN"}, codes)

	_, hasBalanceKey := raw["balanceKey"]
	assert.False(t, hasBalanceKey, "nil BalanceKey must be omitted: absent means every balance (RF-07)")

	_, hasExpiresAt := raw["expiresAt"]
	assert.False(t, hasExpiresAt, "nil ExpiresAt must be omitted: absent means indeterminate validity (RF-09)")

	_, hasEffectiveAt := raw["effectiveAt"]
	assert.False(t, hasEffectiveAt, "nil EffectiveAt must be omitted")
}

// TestAccountExceptionJSONContract_PopulatedOptionalFields is the mirror of the
// omit case: a populated balanceKey/expiresAt MUST reach the wire, because an
// integrator reads the effective scope and the expiry off the query response.
func TestAccountExceptionJSONContract_PopulatedOptionalFields(t *testing.T) {
	t.Parallel()

	expiresAt := baseInstant.Add(24 * time.Hour)
	exception := mmodel.AccountException{
		OperationalTypeCodes: []string{"PIX_IN"},
		BalanceKey:           testutils.Ptr("asset-freeze"),
		Context:              "Court order 12345",
		EffectiveAt:          &baseInstant,
		ExpiresAt:            &expiresAt,
	}

	data, err := json.Marshal(exception)
	require.NoError(t, err)

	var raw map[string]any

	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Equal(t, "asset-freeze", raw["balanceKey"])
	assert.Equal(t, baseInstant.Format(time.RFC3339Nano), raw["effectiveAt"])
	assert.Equal(t, expiresAt.Format(time.RFC3339Nano), raw["expiresAt"])
}
