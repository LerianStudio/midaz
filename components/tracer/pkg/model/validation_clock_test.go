// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TestValidationRequest_Validate_WithClock verifies that Validate() correctly uses injected clock time
// for timestamp validation. This enables deterministic testing with MOCK_TIME.
func TestValidationRequest_Validate_WithClock(t *testing.T) {
	// Fixed clock for deterministic testing
	fixedNow := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	t.Run("valid timestamp within 24h window", func(t *testing.T) {
		// Transaction 1 hour ago (relative to fixedNow)
		txTime := fixedNow.Add(-1 * time.Hour)

		req := &model.ValidationRequest{
			RequestID:            uuid.New(),
			TransactionType:      model.TransactionTypePix,
			Amount:               decimal.NewFromFloat(100.00),
			Currency:             "BRL",
			TransactionTimestamp: txTime,
			Account: model.AccountContext{
				ID: uuid.New(),
			},
		}

		err := req.Validate(fixedNow)
		assert.NoError(t, err)
	})

	t.Run("timestamp too old (>24h)", func(t *testing.T) {
		// Transaction 25 hours ago (relative to fixedNow)
		txTime := fixedNow.Add(-25 * time.Hour)

		req := &model.ValidationRequest{
			RequestID:            uuid.New(),
			TransactionType:      model.TransactionTypePix,
			Amount:               decimal.NewFromFloat(100.00),
			Currency:             "BRL",
			TransactionTimestamp: txTime,
			Account: model.AccountContext{
				ID: uuid.New(),
			},
		}

		err := req.Validate(fixedNow)
		require.Error(t, err)
		assert.Equal(t, constant.ErrValidationTimestampPast, err)
	})

	t.Run("timestamp in future (beyond clock skew)", func(t *testing.T) {
		// Transaction 2 minutes in future (beyond 1min tolerance)
		txTime := fixedNow.Add(2 * time.Minute)

		req := &model.ValidationRequest{
			RequestID:            uuid.New(),
			TransactionType:      model.TransactionTypePix,
			Amount:               decimal.NewFromFloat(100.00),
			Currency:             "BRL",
			TransactionTimestamp: txTime,
			Account: model.AccountContext{
				ID: uuid.New(),
			},
		}

		err := req.Validate(fixedNow)
		require.Error(t, err)
		assert.Equal(t, constant.ErrValidationTimestampFuture, err)
	})

	t.Run("timestamp exactly at boundary (24h old)", func(t *testing.T) {
		// Transaction exactly 24 hours ago
		txTime := fixedNow.Add(-24 * time.Hour)

		req := &model.ValidationRequest{
			RequestID:            uuid.New(),
			TransactionType:      model.TransactionTypePix,
			Amount:               decimal.NewFromFloat(100.00),
			Currency:             "BRL",
			TransactionTimestamp: txTime,
			Account: model.AccountContext{
				ID: uuid.New(),
			},
		}

		// Should be INVALID (After is exclusive)
		err := req.Validate(fixedNow)
		require.Error(t, err)
		assert.Equal(t, constant.ErrValidationTimestampPast, err)
	})

	t.Run("timestamp just within boundary (23h 59min old)", func(t *testing.T) {
		// Transaction 23h 59min ago
		txTime := fixedNow.Add(-23*time.Hour - 59*time.Minute)

		req := &model.ValidationRequest{
			RequestID:            uuid.New(),
			TransactionType:      model.TransactionTypePix,
			Amount:               decimal.NewFromFloat(100.00),
			Currency:             "BRL",
			TransactionTimestamp: txTime,
			Account: model.AccountContext{
				ID: uuid.New(),
			},
		}

		// Should be VALID
		err := req.Validate(fixedNow)
		assert.NoError(t, err)
	})

	t.Run("MOCK_TIME scenario - validates 12h old timestamp with 10:00 mock", func(t *testing.T) {
		// Simulate MOCK_TIME = 2026-03-11T10:00:00Z
		mockNow := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)

		// Transaction from 22:00 previous day (12h before mock time)
		txTime := time.Date(2026, 3, 10, 22, 0, 0, 0, time.UTC)

		req := &model.ValidationRequest{
			RequestID:            uuid.New(),
			TransactionType:      model.TransactionTypePix,
			Amount:               decimal.NewFromFloat(100.00),
			Currency:             "BRL",
			TransactionTimestamp: txTime,
			Account: model.AccountContext{
				ID: uuid.New(),
			},
		}

		// Should be VALID (12h < 24h)
		err := req.Validate(mockNow)
		assert.NoError(t, err)
	})
}

// TestValidationRequest_NormalizeAndValidate_WithClock verifies that NormalizeAndValidate()
// correctly passes clock time to validation.
func TestValidationRequest_NormalizeAndValidate_WithClock(t *testing.T) {
	fixedNow := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	t.Run("valid request passes with clock injection", func(t *testing.T) {
		txTime := fixedNow.Add(-1 * time.Hour)

		req := &model.ValidationRequest{
			RequestID:            uuid.New(),
			TransactionType:      model.TransactionTypePix,
			Amount:               decimal.NewFromFloat(100.00),
			Currency:             "BRL",
			TransactionTimestamp: txTime,
			Account: model.AccountContext{
				ID: uuid.New(),
			},
		}

		err := req.NormalizeAndValidate(fixedNow)
		assert.NoError(t, err)
	})

	t.Run("old timestamp fails with clock injection", func(t *testing.T) {
		txTime := fixedNow.Add(-25 * time.Hour)

		req := &model.ValidationRequest{
			RequestID:            uuid.New(),
			TransactionType:      model.TransactionTypePix,
			Amount:               decimal.NewFromFloat(100.00),
			Currency:             "BRL",
			TransactionTimestamp: txTime,
			Account: model.AccountContext{
				ID: uuid.New(),
			},
		}

		err := req.NormalizeAndValidate(fixedNow)
		require.Error(t, err)
		assert.Equal(t, constant.ErrValidationTimestampPast, err)
	})
}
