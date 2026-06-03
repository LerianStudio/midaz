// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
)

func TestValidateMetadata(t *testing.T) {
	t.Parallel()

	createValidRequest := func() *ValidationRequest {
		return &ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(1),
			TransactionType:      TransactionTypeCard,
			Amount:               decimal.RequireFromString("10"),
			Currency:             "USD",
			TransactionTimestamp: testutil.FixedTime(),
			Account: AccountContext{
				ID: testutil.MustDeterministicUUID(2),
			},
			Metadata: nil,
		}
	}

	t.Run("Success - nil metadata", func(t *testing.T) {
		req := createValidRequest()
		req.Metadata = nil

		err := req.validateMetadata()
		assert.NoError(t, err)
	})

	t.Run("Success - empty metadata", func(t *testing.T) {
		req := createValidRequest()
		req.Metadata = map[string]any{}

		err := req.validateMetadata()
		assert.NoError(t, err)
	})

	t.Run("Success - valid metadata with single entry", func(t *testing.T) {
		req := createValidRequest()
		req.Metadata = map[string]any{
			"key1": "value1",
		}

		err := req.validateMetadata()
		assert.NoError(t, err)
	})

	t.Run("Success - metadata with exactly 50 entries (boundary)", func(t *testing.T) {
		req := createValidRequest()
		req.Metadata = make(map[string]any, 50)

		for i := 0; i < 50; i++ {
			key := fmt.Sprintf("key%d", i)
			req.Metadata[key] = i
		}

		err := req.validateMetadata()
		assert.NoError(t, err)
	})

	t.Run("Error - metadata exceeds 50 entries", func(t *testing.T) {
		req := createValidRequest()
		req.Metadata = make(map[string]any, 51)

		for i := 0; i < 51; i++ {
			key := fmt.Sprintf("key%d", i)
			req.Metadata[key] = i
		}

		err := req.validateMetadata()
		require.Error(t, err)
		assert.ErrorIs(t, err, constant.ErrMetadataEntriesExceeded)
	})

	t.Run("Success - metadata key with exactly 64 characters (boundary)", func(t *testing.T) {
		req := createValidRequest()
		keyAtMax := strings.Repeat("a", 64)
		req.Metadata = map[string]any{
			keyAtMax: "value",
		}

		err := req.validateMetadata()
		assert.NoError(t, err)
	})

	t.Run("Error - metadata key exceeds 64 characters", func(t *testing.T) {
		req := createValidRequest()
		longKey := strings.Repeat("a", 65)
		req.Metadata = map[string]any{
			longKey: "value",
		}

		err := req.validateMetadata()
		require.Error(t, err)
		assert.ErrorIs(t, err, constant.ErrMetadataKeyLengthExceeded)
	})

	t.Run("Success - metadata key with alphanumeric characters", func(t *testing.T) {
		req := createValidRequest()
		req.Metadata = map[string]any{
			"key123ABC": "value",
		}

		err := req.validateMetadata()
		assert.NoError(t, err)
	})

	t.Run("Success - metadata key with underscores", func(t *testing.T) {
		req := createValidRequest()
		req.Metadata = map[string]any{
			"key_name_with_underscores": "value",
		}

		err := req.validateMetadata()
		assert.NoError(t, err)
	})

	t.Run("Success - metadata key with mixed alphanumeric and underscores", func(t *testing.T) {
		req := createValidRequest()
		req.Metadata = map[string]any{
			"transaction_id_123": "value",
		}

		err := req.validateMetadata()
		assert.NoError(t, err)
	})

	t.Run("Error - metadata key with spaces", func(t *testing.T) {
		req := createValidRequest()
		req.Metadata = map[string]any{
			"key with spaces": "value",
		}

		err := req.validateMetadata()
		require.Error(t, err)
		assert.ErrorIs(t, err, constant.ErrMetadataKeyInvalidChars)
	})

	t.Run("Error - metadata key with hyphens", func(t *testing.T) {
		req := createValidRequest()
		req.Metadata = map[string]any{
			"key-with-hyphens": "value",
		}

		err := req.validateMetadata()
		require.Error(t, err)
		assert.ErrorIs(t, err, constant.ErrMetadataKeyInvalidChars)
	})

	t.Run("Error - metadata key with dots", func(t *testing.T) {
		req := createValidRequest()
		req.Metadata = map[string]any{
			"key.with.dots": "value",
		}

		err := req.validateMetadata()
		require.Error(t, err)
		assert.ErrorIs(t, err, constant.ErrMetadataKeyInvalidChars)
	})

	t.Run("Error - metadata key with special characters", func(t *testing.T) {
		specialChars := []string{"key@test", "key#test", "key$test", "key%test", "key&test"}

		for _, key := range specialChars {
			t.Run(key, func(t *testing.T) {
				req := createValidRequest()
				req.Metadata = map[string]any{
					key: "value",
				}

				err := req.validateMetadata()
				require.Error(t, err)
				assert.ErrorIs(t, err, constant.ErrMetadataKeyInvalidChars)
			})
		}
	})

	t.Run("Success - multiple valid metadata entries", func(t *testing.T) {
		req := createValidRequest()
		req.Metadata = map[string]any{
			"transaction_id":   "abc123",
			"merchant_name":    "Test Merchant",
			"user_agent":       "Mozilla/5.0",
			"session_id":       "00000000-0000-0000-0000-000000000042",
			"request_source":   "mobile_app",
			"api_version":      "1.0",
			"client_ip":        "192.168.1.1",
			"device_id":        "device123",
			"correlation_id":   "corr456",
			"timestamp_millis": 1234567890,
		}

		err := req.validateMetadata()
		assert.NoError(t, err)
	})

	t.Run("Error - one invalid key among many valid keys", func(t *testing.T) {
		req := createValidRequest()
		req.Metadata = map[string]any{
			"valid_key1":    "value1",
			"valid_key2":    "value2",
			"invalid-key":   "value3", // hyphen is not allowed
			"another_valid": "value4",
		}

		err := req.validateMetadata()
		require.Error(t, err)
		assert.ErrorIs(t, err, constant.ErrMetadataKeyInvalidChars)
	})
}

func TestValidateOptionalFields_AccountType(t *testing.T) {
	t.Parallel()

	createValidRequest := func() *ValidationRequest {
		return &ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(10),
			TransactionType:      TransactionTypeCard,
			Amount:               decimal.RequireFromString("10"),
			Currency:             "USD",
			TransactionTimestamp: testutil.FixedTime(),
			Account: AccountContext{
				ID: testutil.MustDeterministicUUID(11),
			},
		}
	}

	t.Run("Success - empty account type", func(t *testing.T) {
		req := createValidRequest()
		req.Account.Type = ""

		err := req.validateOptionalFields()
		assert.NoError(t, err)
	})

	t.Run("Success - valid account type: checking", func(t *testing.T) {
		req := createValidRequest()
		req.Account.Type = "checking"

		err := req.validateOptionalFields()
		assert.NoError(t, err)
	})

	t.Run("Success - valid account type: savings", func(t *testing.T) {
		req := createValidRequest()
		req.Account.Type = "savings"

		err := req.validateOptionalFields()
		assert.NoError(t, err)
	})

	t.Run("Success - valid account type: credit", func(t *testing.T) {
		req := createValidRequest()
		req.Account.Type = "credit"

		err := req.validateOptionalFields()
		assert.NoError(t, err)
	})

	t.Run("Error - invalid account type", func(t *testing.T) {
		req := createValidRequest()
		req.Account.Type = "investment"

		err := req.validateOptionalFields()
		require.Error(t, err)
		assert.ErrorIs(t, err, constant.ErrValidationInvalidAccountType)
	})

	t.Run("Error - uppercase account type (case sensitive)", func(t *testing.T) {
		req := createValidRequest()
		req.Account.Type = "CHECKING"

		err := req.validateOptionalFields()
		require.Error(t, err)
		assert.ErrorIs(t, err, constant.ErrValidationInvalidAccountType)
	})
}

func TestValidateOptionalFields_AccountStatus(t *testing.T) {
	t.Parallel()

	createValidRequest := func() *ValidationRequest {
		return &ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(10),
			TransactionType:      TransactionTypeCard,
			Amount:               decimal.RequireFromString("10"),
			Currency:             "USD",
			TransactionTimestamp: testutil.FixedTime(),
			Account: AccountContext{
				ID: testutil.MustDeterministicUUID(11),
			},
		}
	}

	t.Run("Success - empty account status", func(t *testing.T) {
		req := createValidRequest()
		req.Account.Status = ""

		err := req.validateOptionalFields()
		assert.NoError(t, err)
	})

	t.Run("Success - valid account status: active", func(t *testing.T) {
		req := createValidRequest()
		req.Account.Status = "active"

		err := req.validateOptionalFields()
		assert.NoError(t, err)
	})

	t.Run("Success - valid account status: suspended", func(t *testing.T) {
		req := createValidRequest()
		req.Account.Status = "suspended"

		err := req.validateOptionalFields()
		assert.NoError(t, err)
	})

	t.Run("Success - valid account status: closed", func(t *testing.T) {
		req := createValidRequest()
		req.Account.Status = "closed"

		err := req.validateOptionalFields()
		assert.NoError(t, err)
	})

	t.Run("Error - invalid account status", func(t *testing.T) {
		req := createValidRequest()
		req.Account.Status = "pending"

		err := req.validateOptionalFields()
		require.Error(t, err)
		assert.ErrorIs(t, err, constant.ErrValidationInvalidAccountStatus)
	})

	t.Run("Error - uppercase account status (case sensitive)", func(t *testing.T) {
		req := createValidRequest()
		req.Account.Status = "ACTIVE"

		err := req.validateOptionalFields()
		require.Error(t, err)
		assert.ErrorIs(t, err, constant.ErrValidationInvalidAccountStatus)
	})
}

func TestValidateOptionalFields_SubType(t *testing.T) {
	t.Parallel()

	createValidRequest := func() *ValidationRequest {
		return &ValidationRequest{
			RequestID:            testutil.MustDeterministicUUID(10),
			TransactionType:      TransactionTypeCard,
			Amount:               decimal.RequireFromString("10"),
			Currency:             "USD",
			TransactionTimestamp: testutil.FixedTime(),
			Account: AccountContext{
				ID: testutil.MustDeterministicUUID(11),
			},
		}
	}

	t.Run("Success - nil subType", func(t *testing.T) {
		req := createValidRequest()
		req.SubType = nil

		err := req.validateOptionalFields()
		assert.NoError(t, err)
	})

	t.Run("Success - valid subType", func(t *testing.T) {
		req := createValidRequest()
		subType := "CREDIT"
		req.SubType = &subType

		err := req.validateOptionalFields()
		assert.NoError(t, err)
	})

	t.Run("Success - subType at max length (boundary)", func(t *testing.T) {
		req := createValidRequest()
		subTypeAtMax := strings.Repeat("A", MaxSubTypeLength)
		req.SubType = &subTypeAtMax

		err := req.validateOptionalFields()
		assert.NoError(t, err)
	})

	t.Run("Error - subType exceeds max length", func(t *testing.T) {
		req := createValidRequest()
		longSubType := strings.Repeat("A", MaxSubTypeLength+1)
		req.SubType = &longSubType

		err := req.validateOptionalFields()
		require.Error(t, err)
		assert.ErrorIs(t, err, constant.ErrValidationSubTypeTooLong)
	})
}
