// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const schemeShapeMessage = "must be 1 to 50 characters of A-Z, 0-9, _ or - after trimming and upper-casing"

func TestValidateBusinessError_TracerSchemeContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sentinel    error
		wantCode    string
		wantTitle   string
		wantMessage string
	}{
		{
			name:        "check limits transaction type shape",
			sentinel:    constant.ErrCheckLimitsInvalidTransactionType,
			wantCode:    "0405",
			wantTitle:   "Check Limits Invalid Transaction Type",
			wantMessage: schemeShapeMessage,
		},
		{
			name:        "validation transaction type shape",
			sentinel:    constant.ErrValidationInvalidTransactionType,
			wantCode:    "0414",
			wantTitle:   "Validation Invalid Transaction Type",
			wantMessage: schemeShapeMessage,
		},
		{
			name:        "scheme alias conflict",
			sentinel:    constant.ErrValidationSchemeAliasConflict,
			wantCode:    "0539",
			wantTitle:   "Scheme Alias Conflict",
			wantMessage: "scheme and transactionType must carry the same value",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := pkg.ValidateBusinessError(tc.sentinel, "Validation")

			mapped, ok := err.(pkg.ValidationError)
			require.True(t, ok, "%s must be an HTTP 400 ValidationError, got %T", tc.wantCode, err)
			assert.Equal(t, tc.wantCode, mapped.Code)
			assert.Equal(t, tc.wantTitle, mapped.Title)
			assert.Equal(t, tc.wantMessage, mapped.Message)
		})
	}
}
