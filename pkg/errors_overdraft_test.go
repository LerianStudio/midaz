// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg_test

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateBusinessError_OverdraftErrors verifies that ValidateBusinessError
// maps the three overdraft sentinels (0167, 0168, 0169) to
// UnprocessableOperationError (HTTP 422) with the correct codes, mirroring the
// existing ErrInsufficientFunds mapping at pkg/errors.go:446.
func TestValidateBusinessError_OverdraftErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		sentinel   error
		wantCode   string
		entityType string
	}{
		{
			name:       "ErrOverdraftLimitExceeded maps to UnprocessableOperationError (0167)",
			sentinel:   constant.ErrOverdraftLimitExceeded,
			wantCode:   "0167",
			entityType: "Balance",
		},
		{
			name:       "ErrDirectOperationOnInternalBalance maps to UnprocessableOperationError (0168)",
			sentinel:   constant.ErrDirectOperationOnInternalBalance,
			wantCode:   "0168",
			entityType: "Balance",
		},
		{
			name:       "ErrDeletionOfInternalBalance maps to UnprocessableOperationError (0169)",
			sentinel:   constant.ErrDeletionOfInternalBalance,
			wantCode:   "0169",
			entityType: "Balance",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := pkg.ValidateBusinessError(tt.sentinel, tt.entityType)
			require.Error(t, got, "ValidateBusinessError must return a mapped error for %s", tt.sentinel.Error())

			mapped, ok := got.(pkg.UnprocessableOperationError)
			require.True(t, ok,
				"%s must map to UnprocessableOperationError (HTTP 422), got %T", tt.sentinel.Error(), got)

			assert.Equal(t, tt.wantCode, mapped.Code,
				"mapped error code must match sentinel error string")
			assert.Equal(t, tt.entityType, mapped.EntityType,
				"entityType must be propagated through ValidateBusinessError")
			assert.NotEmpty(t, mapped.Title,
				"mapped error must have a non-empty Title")
			assert.NotEmpty(t, mapped.Message,
				"mapped error must have a non-empty Message")
		})
	}
}
