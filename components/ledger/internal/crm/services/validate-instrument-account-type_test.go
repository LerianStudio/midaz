// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func TestNormalizeInstrumentAccountType(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	testCases := []struct {
		name        string
		input       *string
		expected    *string
		expectedErr error
	}{
		{name: "nil is treated as absent", input: nil, expected: nil},
		{name: "canonical value is kept", input: strPtr("DEPOSIT"), expected: strPtr("DEPOSIT")},
		{name: "lowercase with surrounding spaces is normalized", input: strPtr(" savings "), expected: strPtr("SAVINGS")},
		{name: "mixed case is normalized", input: strPtr("Other_Financial_Investments"), expected: strPtr("OTHER_FINANCIAL_INVESTMENTS")},
		{name: "empty string is treated as absent", input: strPtr(""), expected: nil},
		{name: "whitespace only is treated as absent", input: strPtr("   "), expected: nil},
		{name: "value outside the enum is rejected", input: strPtr("CHECKING"), expectedErr: cn.ErrInvalidInstrumentAccountType},
		{name: "numeric bacen code is rejected", input: strPtr("1"), expectedErr: cn.ErrInvalidInstrumentAccountType},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeInstrumentAccountType(tc.input)

			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.Nil(t, got)

				var validationErr pkg.ValidationError
				require.True(t, errors.As(err, &validationErr), "rejection must be a ValidationError, got %T", err)
				assert.Equal(t, tc.expectedErr.Error(), validationErr.Code)
				assert.Equal(t, cn.EntityInstrument, validationErr.EntityType)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestNormalizeInstrumentAccountType_AcceptsEveryCanonicalValue(t *testing.T) {
	for _, accountType := range mmodel.InstrumentAccountTypes() {
		t.Run(accountType, func(t *testing.T) {
			value := accountType

			got, err := normalizeInstrumentAccountType(&value)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, accountType, *got)
		})
	}
}

func TestNormalizeInstrumentAccountType_ReturnsFreshPointer(t *testing.T) {
	raw := " deposit "

	got, err := normalizeInstrumentAccountType(&raw)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.NotSame(t, &raw, got, "the normalized value must not alias the raw input pointer")
	assert.Equal(t, " deposit ", raw, "the raw input must not be mutated")
}
