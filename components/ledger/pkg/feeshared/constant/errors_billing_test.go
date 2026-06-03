// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant_test

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBillingErrorCodes_ConstantsExist verifies that all Motor 2 error constants
// (FEE-0052 through FEE-0069) are defined with the correct error codes.
func TestBillingErrorCodes_ConstantsExist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		constant     error
		expectedCode string
	}{
		// CRUD errors
		{
			name:         "ErrBillingPackageNotFound has code FEE-0052",
			constant:     constant.ErrBillingPackageNotFound,
			expectedCode: "FEE-0052",
		},
		{
			name:         "ErrInvalidBillingPackageType has code FEE-0053",
			constant:     constant.ErrInvalidBillingPackageType,
			expectedCode: "FEE-0053",
		},
		{
			name:         "ErrMissingVolumeFields has code FEE-0054",
			constant:     constant.ErrMissingVolumeFields,
			expectedCode: "FEE-0054",
		},
		{
			name:         "ErrMissingMaintenanceFields has code FEE-0055",
			constant:     constant.ErrMissingMaintenanceFields,
			expectedCode: "FEE-0055",
		},
		{
			name:         "ErrInvalidPricingModel has code FEE-0056",
			constant:     constant.ErrInvalidPricingModel,
			expectedCode: "FEE-0056",
		},
		{
			name:         "ErrInvalidPricingTier has code FEE-0057",
			constant:     constant.ErrInvalidPricingTier,
			expectedCode: "FEE-0057",
		},
		{
			name:         "ErrBillingRouteOverlap has code FEE-0058",
			constant:     constant.ErrBillingRouteOverlap,
			expectedCode: "FEE-0058",
		},
		{
			name:         "ErrInvalidBillingPeriod has code FEE-0063",
			constant:     constant.ErrInvalidBillingPeriod,
			expectedCode: "FEE-0063",
		},
		{
			name:         "ErrInvalidFreeQuota has code FEE-0064",
			constant:     constant.ErrInvalidFreeQuota,
			expectedCode: "FEE-0064",
		},
		{
			name:         "ErrInvalidDiscountTier has code FEE-0065",
			constant:     constant.ErrInvalidDiscountTier,
			expectedCode: "FEE-0065",
		},
		{
			name:         "ErrInvalidCountMode has code FEE-0067",
			constant:     constant.ErrInvalidCountMode,
			expectedCode: "FEE-0067",
		},
		// Calculation errors
		{
			name:         "ErrTargetAccountNotFound has code FEE-0059",
			constant:     constant.ErrTargetAccountNotFound,
			expectedCode: "FEE-0059",
		},
		{
			name:         "ErrBillingCalculationFailed has code FEE-0060",
			constant:     constant.ErrBillingCalculationFailed,
			expectedCode: "FEE-0060",
		},
		{
			name:         "ErrNoActiveBillingPackages has code FEE-0061",
			constant:     constant.ErrNoActiveBillingPackages,
			expectedCode: "FEE-0061",
		},
		// Integration errors
		{
			name:         "ErrSegmentResolutionFailed has code FEE-0062",
			constant:     constant.ErrSegmentResolutionFailed,
			expectedCode: "FEE-0062",
		},
		{
			name:         "ErrMidazQueryFailed has code FEE-0068",
			constant:     constant.ErrMidazQueryFailed,
			expectedCode: "FEE-0068",
		},
		{
			name:         "ErrInvalidAccountTarget has code FEE-0069",
			constant:     constant.ErrInvalidAccountTarget,
			expectedCode: "FEE-0069",
		},
		{
			name:         "ErrInvalidFeeAmount has code FEE-0070",
			constant:     constant.ErrInvalidFeeAmount,
			expectedCode: "FEE-0070",
		},
		{
			name:         "ErrMissingSegmentContext has code FEE-0071",
			constant:     constant.ErrMissingSegmentContext,
			expectedCode: "FEE-0071",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.NotNil(t, tt.constant, "constant must not be nil")
			assert.Equal(t, tt.expectedCode, tt.constant.Error(),
				"error constant must have correct FEE-XXXX code")
		})
	}
}

// TestBillingErrorCodes_ValidateBusinessErrorMapping verifies that ValidateBusinessError()
// correctly maps each Motor 2 error constant to its expected typed error.
func TestBillingErrorCodes_ValidateBusinessErrorMapping(t *testing.T) {
	t.Parallel()

	entityType := "BillingPackage"

	tests := []struct {
		name            string
		inputErr        error
		entityType      string
		args            []any
		expectedErrType string
		checkFn         func(t *testing.T, result error)
	}{
		// CRUD errors - EntityNotFoundError
		{
			name:       "ErrBillingPackageNotFound maps to EntityNotFoundError",
			inputErr:   constant.ErrBillingPackageNotFound,
			entityType: entityType,
			args:       []any{"pkg-123"},
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				notFoundErr, ok := result.(pkg.EntityNotFoundError)
				require.True(t, ok, "expected EntityNotFoundError, got %T", result)
				assert.Equal(t, entityType, notFoundErr.EntityType)
				assert.Equal(t, constant.ErrBillingPackageNotFound.Error(), notFoundErr.Code)
			},
		},
		// CRUD errors - ValidationError
		{
			name:       "ErrInvalidBillingPackageType maps to ValidationError",
			inputErr:   constant.ErrInvalidBillingPackageType,
			entityType: entityType,
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				validationErr, ok := result.(pkg.ValidationError)
				require.True(t, ok, "expected ValidationError, got %T", result)
				assert.Equal(t, entityType, validationErr.EntityType)
				assert.Equal(t, constant.ErrInvalidBillingPackageType.Error(), validationErr.Code)
			},
		},
		{
			name:       "ErrMissingVolumeFields maps to ValidationError",
			inputErr:   constant.ErrMissingVolumeFields,
			entityType: entityType,
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				validationErr, ok := result.(pkg.ValidationError)
				require.True(t, ok, "expected ValidationError, got %T", result)
				assert.Equal(t, entityType, validationErr.EntityType)
				assert.Equal(t, constant.ErrMissingVolumeFields.Error(), validationErr.Code)
			},
		},
		{
			name:       "ErrMissingMaintenanceFields maps to ValidationError",
			inputErr:   constant.ErrMissingMaintenanceFields,
			entityType: entityType,
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				validationErr, ok := result.(pkg.ValidationError)
				require.True(t, ok, "expected ValidationError, got %T", result)
				assert.Equal(t, entityType, validationErr.EntityType)
				assert.Equal(t, constant.ErrMissingMaintenanceFields.Error(), validationErr.Code)
			},
		},
		{
			name:       "ErrInvalidPricingModel maps to ValidationError",
			inputErr:   constant.ErrInvalidPricingModel,
			entityType: entityType,
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				validationErr, ok := result.(pkg.ValidationError)
				require.True(t, ok, "expected ValidationError, got %T", result)
				assert.Equal(t, entityType, validationErr.EntityType)
				assert.Equal(t, constant.ErrInvalidPricingModel.Error(), validationErr.Code)
			},
		},
		{
			name:       "ErrInvalidPricingTier maps to ValidationError",
			inputErr:   constant.ErrInvalidPricingTier,
			entityType: entityType,
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				validationErr, ok := result.(pkg.ValidationError)
				require.True(t, ok, "expected ValidationError, got %T", result)
				assert.Equal(t, entityType, validationErr.EntityType)
				assert.Equal(t, constant.ErrInvalidPricingTier.Error(), validationErr.Code)
			},
		},
		{
			name:       "ErrBillingRouteOverlap maps to ValidationError",
			inputErr:   constant.ErrBillingRouteOverlap,
			entityType: entityType,
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				validationErr, ok := result.(pkg.ValidationError)
				require.True(t, ok, "expected ValidationError, got %T", result)
				assert.Equal(t, entityType, validationErr.EntityType)
				assert.Equal(t, constant.ErrBillingRouteOverlap.Error(), validationErr.Code)
			},
		},
		{
			name:       "ErrInvalidBillingPeriod maps to ValidationError",
			inputErr:   constant.ErrInvalidBillingPeriod,
			entityType: entityType,
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				validationErr, ok := result.(pkg.ValidationError)
				require.True(t, ok, "expected ValidationError, got %T", result)
				assert.Equal(t, entityType, validationErr.EntityType)
				assert.Equal(t, constant.ErrInvalidBillingPeriod.Error(), validationErr.Code)
			},
		},
		{
			name:       "ErrInvalidFreeQuota maps to ValidationError",
			inputErr:   constant.ErrInvalidFreeQuota,
			entityType: entityType,
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				validationErr, ok := result.(pkg.ValidationError)
				require.True(t, ok, "expected ValidationError, got %T", result)
				assert.Equal(t, entityType, validationErr.EntityType)
				assert.Equal(t, constant.ErrInvalidFreeQuota.Error(), validationErr.Code)
			},
		},
		{
			name:       "ErrInvalidDiscountTier maps to ValidationError",
			inputErr:   constant.ErrInvalidDiscountTier,
			entityType: entityType,
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				validationErr, ok := result.(pkg.ValidationError)
				require.True(t, ok, "expected ValidationError, got %T", result)
				assert.Equal(t, entityType, validationErr.EntityType)
				assert.Equal(t, constant.ErrInvalidDiscountTier.Error(), validationErr.Code)
			},
		},
		{
			name:       "ErrInvalidCountMode maps to ValidationError",
			inputErr:   constant.ErrInvalidCountMode,
			entityType: entityType,
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				validationErr, ok := result.(pkg.ValidationError)
				require.True(t, ok, "expected ValidationError, got %T", result)
				assert.Equal(t, entityType, validationErr.EntityType)
				assert.Equal(t, constant.ErrInvalidCountMode.Error(), validationErr.Code)
			},
		},
		// Calculation errors - EntityNotFoundError
		{
			name:       "ErrTargetAccountNotFound maps to EntityNotFoundError",
			inputErr:   constant.ErrTargetAccountNotFound,
			entityType: entityType,
			args:       []any{"account-alias"},
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				notFoundErr, ok := result.(pkg.EntityNotFoundError)
				require.True(t, ok, "expected EntityNotFoundError, got %T", result)
				assert.Equal(t, entityType, notFoundErr.EntityType)
				assert.Equal(t, constant.ErrTargetAccountNotFound.Error(), notFoundErr.Code)
			},
		},
		// Calculation errors - UnprocessableOperationError
		{
			name:       "ErrBillingCalculationFailed maps to UnprocessableOperationError",
			inputErr:   constant.ErrBillingCalculationFailed,
			entityType: entityType,
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				unprocessableErr, ok := result.(pkg.UnprocessableOperationError)
				require.True(t, ok, "expected UnprocessableOperationError, got %T", result)
				assert.Equal(t, entityType, unprocessableErr.EntityType)
				assert.Equal(t, constant.ErrBillingCalculationFailed.Error(), unprocessableErr.Code)
			},
		},
		{
			name:       "ErrNoActiveBillingPackages maps to EntityNotFoundError",
			inputErr:   constant.ErrNoActiveBillingPackages,
			entityType: entityType,
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				notFoundErr, ok := result.(pkg.EntityNotFoundError)
				require.True(t, ok, "expected EntityNotFoundError, got %T", result)
				assert.Equal(t, entityType, notFoundErr.EntityType)
				assert.Equal(t, constant.ErrNoActiveBillingPackages.Error(), notFoundErr.Code)
			},
		},
		// Integration errors - UnprocessableOperationError (422 — known dependency failures)
		{
			name:       "ErrSegmentResolutionFailed maps to UnprocessableOperationError",
			inputErr:   constant.ErrSegmentResolutionFailed,
			entityType: entityType,
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				unprocessableErr, ok := result.(pkg.UnprocessableOperationError)
				require.True(t, ok, "expected UnprocessableOperationError, got %T", result)
				assert.Equal(t, entityType, unprocessableErr.EntityType)
				assert.Equal(t, constant.ErrSegmentResolutionFailed.Error(), unprocessableErr.Code)
				assert.NotContains(t, unprocessableErr.Message, "connection", "public message must not leak transport details")
			},
		},
		{
			name:       "ErrMidazQueryFailed maps to UnprocessableOperationError",
			inputErr:   constant.ErrMidazQueryFailed,
			entityType: entityType,
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				unprocessableErr, ok := result.(pkg.UnprocessableOperationError)
				require.True(t, ok, "expected UnprocessableOperationError, got %T", result)
				assert.Equal(t, entityType, unprocessableErr.EntityType)
				assert.Equal(t, constant.ErrMidazQueryFailed.Error(), unprocessableErr.Code)
				assert.NotContains(t, unprocessableErr.Message, "connection", "public message must not leak transport details")
			},
		},
		// Integration errors - InternalServerError (500 — internal configuration issue)
		{
			name:       "ErrMissingSegmentContext maps to InternalServerError",
			inputErr:   constant.ErrMissingSegmentContext,
			entityType: entityType,
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				internalErr, ok := result.(pkg.InternalServerError)
				require.True(t, ok, "expected InternalServerError, got %T", result)
				assert.Equal(t, entityType, internalErr.EntityType)
				assert.Equal(t, constant.ErrMissingSegmentContext.Error(), internalErr.Code)
			},
		},
		// Integration errors - ValidationError
		{
			name:       "ErrInvalidAccountTarget maps to ValidationError",
			inputErr:   constant.ErrInvalidAccountTarget,
			entityType: entityType,
			checkFn: func(t *testing.T, result error) {
				t.Helper()

				validationErr, ok := result.(pkg.ValidationError)
				require.True(t, ok, "expected ValidationError, got %T", result)
				assert.Equal(t, entityType, validationErr.EntityType)
				assert.Equal(t, constant.ErrInvalidAccountTarget.Error(), validationErr.Code)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := pkg.ValidateBusinessError(tt.inputErr, tt.entityType, tt.args...)
			require.Error(t, result)
			tt.checkFn(t, result)
		})
	}
}
