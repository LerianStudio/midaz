package utils_test

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestMidazError(t *testing.T) {
	t.Run("Error method with underlying error", func(t *testing.T) {
		underlyingErr := errors.New("underlying error")
		midazErr := &utils.MidazError{
			Code:    utils.CodeValidation,
			Message: "validation failed",
			Err:     underlyingErr,
		}

		expected := "validation_error: validation failed: underlying error"
		assert.Equal(t, expected, midazErr.Error())
	})

	t.Run("Error method without underlying error", func(t *testing.T) {
		midazErr := &utils.MidazError{
			Code:    utils.CodeNotFound,
			Message: "resource not found",
		}

		expected := "not_found: resource not found"
		assert.Equal(t, expected, midazErr.Error())
	})

	t.Run("Unwrap method", func(t *testing.T) {
		underlyingErr := errors.New("underlying error")
		midazErr := &utils.MidazError{
			Code:    utils.CodeValidation,
			Message: "validation failed",
			Err:     underlyingErr,
		}

		assert.Equal(t, underlyingErr, midazErr.Unwrap())
	})
}

func TestNewMidazError(t *testing.T) {
	t.Run("Creates error with correct properties", func(t *testing.T) {
		underlyingErr := errors.New("test error")
		midazErr := utils.NewMidazError(utils.CodeValidation, underlyingErr)

		assert.Equal(t, utils.CodeValidation, midazErr.Code)
		assert.Equal(t, "test error", midazErr.Message)
		assert.Equal(t, underlyingErr, midazErr.Err)
	})
}

func TestErrorCheckingFunctions(t *testing.T) {
	testCases := []struct {
		name           string
		err            error
		checkFunc      func(error) bool
		expectedResult bool
	}{
		{
			name:           "IsValidationError with ErrValidation",
			err:            utils.ErrValidation,
			checkFunc:      utils.IsValidationError,
			expectedResult: true,
		},
		{
			name:           "IsValidationError with string containing 'validation'",
			err:            errors.New("this is a validation error"),
			checkFunc:      utils.IsValidationError,
			expectedResult: true,
		},
		{
			name:           "IsValidationError with unrelated error",
			err:            errors.New("unrelated error"),
			checkFunc:      utils.IsValidationError,
			expectedResult: false,
		},
		{
			name:           "IsInsufficientBalanceError with ErrInsufficientBalance",
			err:            utils.ErrInsufficientBalance,
			checkFunc:      utils.IsInsufficientBalanceError,
			expectedResult: true,
		},
		{
			name:           "IsAccountEligibilityError with ErrAccountEligibility",
			err:            utils.ErrAccountEligibility,
			checkFunc:      utils.IsAccountEligibilityError,
			expectedResult: true,
		},
		{
			name:           "IsAssetMismatchError with ErrAssetMismatch",
			err:            utils.ErrAssetMismatch,
			checkFunc:      utils.IsAssetMismatchError,
			expectedResult: true,
		},
		{
			name:           "IsAuthenticationError with ErrAuthentication",
			err:            utils.ErrAuthentication,
			checkFunc:      utils.IsAuthenticationError,
			expectedResult: true,
		},
		{
			name:           "IsPermissionError with ErrPermission",
			err:            utils.ErrPermission,
			checkFunc:      utils.IsPermissionError,
			expectedResult: true,
		},
		{
			name:           "IsNotFoundError with ErrNotFound",
			err:            utils.ErrNotFound,
			checkFunc:      utils.IsNotFoundError,
			expectedResult: true,
		},
		{
			name:           "IsIdempotencyError with ErrIdempotency",
			err:            utils.ErrIdempotency,
			checkFunc:      utils.IsIdempotencyError,
			expectedResult: true,
		},
		{
			name:           "IsRateLimitError with ErrRateLimit",
			err:            utils.ErrRateLimit,
			checkFunc:      utils.IsRateLimitError,
			expectedResult: true,
		},
		{
			name:           "IsTimeoutError with ErrTimeout",
			err:            utils.ErrTimeout,
			checkFunc:      utils.IsTimeoutError,
			expectedResult: true,
		},
		{
			name:           "IsInternalError with ErrInternal",
			err:            utils.ErrInternal,
			checkFunc:      utils.IsInternalError,
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.checkFunc(tc.err)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestFormatTransactionError(t *testing.T) {
	testCases := []struct {
		name           string
		err            error
		operationType  string
		expectedResult string
	}{
		{
			name:           "nil error",
			err:            nil,
			operationType:  "Transfer",
			expectedResult: "",
		},
		{
			name:           "validation error",
			err:            utils.ErrValidation,
			operationType:  "Transfer",
			expectedResult: "Transfer failed: Invalid parameters - validation error",
		},
		{
			name:           "insufficient balance error",
			err:            utils.ErrInsufficientBalance,
			operationType:  "Withdrawal",
			expectedResult: "Withdrawal failed: Insufficient account balance - insufficient balance",
		},
		{
			name:           "account eligibility error",
			err:            utils.ErrAccountEligibility,
			operationType:  "Deposit",
			expectedResult: "Deposit failed: Account not eligible - account eligibility error",
		},
		{
			name:           "asset mismatch error",
			err:            utils.ErrAssetMismatch,
			operationType:  "Exchange",
			expectedResult: "Exchange failed: Asset type mismatch - asset mismatch",
		},
		{
			name:           "authentication error",
			err:            utils.ErrAuthentication,
			operationType:  "Transfer",
			expectedResult: "Transfer failed: Authentication error - authentication error",
		},
		{
			name:           "permission error",
			err:            utils.ErrPermission,
			operationType:  "Transfer",
			expectedResult: "Transfer failed: Permission denied - permission error",
		},
		{
			name:           "not found error",
			err:            utils.ErrNotFound,
			operationType:  "Transfer",
			expectedResult: "Transfer failed: Resource not found - not found",
		},
		{
			name:           "idempotency error",
			err:            utils.ErrIdempotency,
			operationType:  "Transfer",
			expectedResult: "Transfer failed: Idempotency issue - idempotency error",
		},
		{
			name:           "rate limit error",
			err:            utils.ErrRateLimit,
			operationType:  "Transfer",
			expectedResult: "Transfer failed: Rate limit exceeded - rate limit exceeded",
		},
		{
			name:           "timeout error",
			err:            utils.ErrTimeout,
			operationType:  "Transfer",
			expectedResult: "Transfer failed: Operation timed out - timeout",
		},
		{
			name:           "internal error",
			err:            utils.ErrInternal,
			operationType:  "Transfer",
			expectedResult: "Transfer failed: Internal server error - internal error",
		},
		{
			name:           "unknown error",
			err:            errors.New("unknown error"),
			operationType:  "Transfer",
			expectedResult: "Transfer failed: unknown error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := utils.FormatTransactionError(tc.err, tc.operationType)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestCategorizeTransactionError(t *testing.T) {
	testCases := []struct {
		name           string
		err            error
		expectedResult string
	}{
		{
			name:           "nil error",
			err:            nil,
			expectedResult: "none",
		},
		{
			name:           "validation error",
			err:            utils.ErrValidation,
			expectedResult: "validation",
		},
		{
			name:           "insufficient balance error",
			err:            utils.ErrInsufficientBalance,
			expectedResult: "insufficient_balance",
		},
		{
			name:           "account eligibility error",
			err:            utils.ErrAccountEligibility,
			expectedResult: "account_eligibility",
		},
		{
			name:           "asset mismatch error",
			err:            utils.ErrAssetMismatch,
			expectedResult: "asset_mismatch",
		},
		{
			name:           "authentication error",
			err:            utils.ErrAuthentication,
			expectedResult: "authentication",
		},
		{
			name:           "permission error",
			err:            utils.ErrPermission,
			expectedResult: "permission",
		},
		{
			name:           "not found error",
			err:            utils.ErrNotFound,
			expectedResult: "not_found",
		},
		{
			name:           "idempotency error",
			err:            utils.ErrIdempotency,
			expectedResult: "idempotency",
		},
		{
			name:           "rate limit error",
			err:            utils.ErrRateLimit,
			expectedResult: "rate_limit",
		},
		{
			name:           "timeout error",
			err:            utils.ErrTimeout,
			expectedResult: "timeout",
		},
		{
			name:           "internal error",
			err:            utils.ErrInternal,
			expectedResult: "internal",
		},
		{
			name:           "unknown error",
			err:            errors.New("unknown error"),
			expectedResult: "unknown",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := utils.CategorizeTransactionError(tc.err)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}
