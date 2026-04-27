// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateAssetCode covers the asset-code validator's three observable shapes:
// pass-through for a clean uppercase-only code, invalid-format for any non-letter
// character, and uppercase-requirement for a lowercase letter. The mapping back to
// the typed pkg business error is the contract callers downstream depend on.
func TestValidateAssetCode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		code         string
		wantErr      bool
		wantSentinel error
	}{
		{
			name:    "valid uppercase USD passes",
			code:    "USD",
			wantErr: false,
		},
		{
			name:    "valid uppercase BRL passes",
			code:    "BRL",
			wantErr: false,
		},
		{
			name:    "empty code passes (no characters to fail iteration)",
			code:    "",
			wantErr: false,
		},
		{
			name:         "lowercase usd fails uppercase requirement",
			code:         "usd",
			wantErr:      true,
			wantSentinel: constant.ErrCodeUppercaseRequirement,
		},
		{
			name:         "mixed case Usd fails uppercase requirement on first lowercase char",
			code:         "Usd",
			wantErr:      true,
			wantSentinel: constant.ErrCodeUppercaseRequirement,
		},
		{
			name:         "digit in code fails invalid format",
			code:         "U1D",
			wantErr:      true,
			wantSentinel: constant.ErrInvalidCodeFormat,
		},
		{
			name:         "leading digit fails invalid format",
			code:         "1USD",
			wantErr:      true,
			wantSentinel: constant.ErrInvalidCodeFormat,
		},
		{
			name:         "special character fails invalid format",
			code:         "U$D",
			wantErr:      true,
			wantSentinel: constant.ErrInvalidCodeFormat,
		},
	}

	uc := &UseCase{}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := uc.validateAssetCode(context.Background(), tc.code)

			if !tc.wantErr {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)

			// Mapped to a typed business error — the Code field carries the sentinel.
			var validation pkg.ValidationError
			require.ErrorAs(t, err, &validation, "expected ValidationError, got %T", err)
			assert.Equal(t, tc.wantSentinel.Error(), validation.Code,
				"sentinel mapping must surface as the typed error's Code")
		})
	}
}
