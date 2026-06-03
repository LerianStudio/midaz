// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestErrorConstants_LimitExtended tests that the new limit-related error constants exist.
// These are required for WEEKLY, CUSTOM limit types and time window validation.
func TestErrorConstants_LimitExtended(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		code     string
		category string
	}{
		{
			name:     "ErrLimitTimeWindowMismatch exists with code TRC-0300",
			err:      ErrLimitTimeWindowMismatch,
			code:     "TRC-0300",
			category: "time window validation",
		},
		{
			name:     "ErrLimitTimeWindowZeroWidth exists with code TRC-0301",
			err:      ErrLimitTimeWindowZeroWidth,
			code:     "TRC-0301",
			category: "time window validation",
		},
		{
			name:     "ErrTimeOfDayInvalidFormat exists with code TRC-0302",
			err:      ErrTimeOfDayInvalidFormat,
			code:     "TRC-0302",
			category: "time of day parsing",
		},
		{
			name:     "ErrRuleNameAlreadyExistsInCtx exists with code TRC-0303",
			err:      ErrRuleNameAlreadyExistsInCtx,
			code:     "TRC-0303",
			category: "rule name uniqueness",
		},
		{
			name:     "ErrLimitNameAlreadyExists exists with code TRC-0304",
			err:      ErrLimitNameAlreadyExists,
			code:     "TRC-0304",
			category: "limit name uniqueness",
		},
		{
			name:     "ErrLimitCustomDatesNotAllowed exists with code TRC-0305",
			err:      ErrLimitCustomDatesNotAllowed,
			code:     "TRC-0305",
			category: "custom period validation",
		},
		{
			name:     "ErrLimitUnknownType exists with code TRC-0306",
			err:      ErrLimitUnknownType,
			code:     "TRC-0306",
			category: "limit type validation",
		},
		{
			name:     "ErrLimitCustomPeriodTooLong exists with code TRC-0307",
			err:      ErrLimitCustomPeriodTooLong,
			code:     "TRC-0307",
			category: "custom period validation",
		},
		{
			name:     "ErrLimitCustomPeriodExpired exists with code TRC-0308",
			err:      ErrLimitCustomPeriodExpired,
			code:     "TRC-0308",
			category: "custom period validation",
		},
		{
			name:     "ErrLimitInvalidCustomStartFormat exists with code TRC-0309",
			err:      ErrLimitInvalidCustomStartFormat,
			code:     "TRC-0309",
			category: "custom period validation",
		},
		{
			name:     "ErrLimitInvalidCustomEndFormat exists with code TRC-0310",
			err:      ErrLimitInvalidCustomEndFormat,
			code:     "TRC-0310",
			category: "custom period validation",
		},
		{
			name:     "ErrLimitCustomDatesRequired exists with code TRC-0311",
			err:      ErrLimitCustomDatesRequired,
			code:     "TRC-0311",
			category: "custom period validation",
		},
		{
			name:     "ErrLimitCustomDatesOrder exists with code TRC-0312",
			err:      ErrLimitCustomDatesOrder,
			code:     "TRC-0312",
			category: "custom period validation",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Verify error is not nil
			assert.NotNil(t, tc.err, "error constant should exist")

			// Verify error message contains the expected code
			assert.Contains(t, tc.err.Error(), tc.code,
				"error message should contain code %s for %s", tc.code, tc.category)

			// Verify it's a proper error type
			wrapped := fmt.Errorf("wrapped: %w", tc.err)
			assert.True(t, errors.Is(wrapped, tc.err),
				"error should be unwrappable with errors.Is")
		})
	}
}

// TestErrorConstants_UniquenessTRC0300Range tests that error codes in TRC-0300 range are unique.
func TestErrorConstants_UniquenessTRC0300Range(t *testing.T) {
	// All TRC-0300 range errors that should be unique
	errorConstants := []error{
		ErrLimitTimeWindowMismatch,       // TRC-0300
		ErrLimitTimeWindowZeroWidth,      // TRC-0301
		ErrTimeOfDayInvalidFormat,        // TRC-0302
		ErrRuleNameAlreadyExistsInCtx,    // TRC-0303
		ErrLimitNameAlreadyExists,        // TRC-0304
		ErrLimitCustomDatesNotAllowed,    // TRC-0305
		ErrLimitUnknownType,              // TRC-0306
		ErrLimitCustomPeriodTooLong,      // TRC-0307
		ErrLimitCustomPeriodExpired,      // TRC-0308
		ErrLimitInvalidCustomStartFormat, // TRC-0309
		ErrLimitInvalidCustomEndFormat,   // TRC-0310
		ErrLimitCustomDatesRequired,      // TRC-0311
		ErrLimitCustomDatesOrder,         // TRC-0312
	}

	// Check for duplicates using error messages
	seen := make(map[string]int)
	for i, err := range errorConstants {
		msg := err.Error()
		if prev, exists := seen[msg]; exists {
			t.Errorf("duplicate error code: index %d and %d both have message %q", prev, i, msg)
		}
		seen[msg] = i
	}
}

// TestErrorConstants_NonEmptyMessages tests that error constants have non-empty error text.
func TestErrorConstants_NonEmptyMessages(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "time window mismatch has non-empty message",
			err:  ErrLimitTimeWindowMismatch,
		},
		{
			name: "zero width window has non-empty message",
			err:  ErrLimitTimeWindowZeroWidth,
		},
		{
			name: "custom dates not allowed has non-empty message",
			err:  ErrLimitCustomDatesNotAllowed,
		},
		{
			name: "custom period too long has non-empty message",
			err:  ErrLimitCustomPeriodTooLong,
		},
		{
			name: "invalid time of day format has non-empty message",
			err:  ErrTimeOfDayInvalidFormat,
		},
		{
			name: "unknown limit type has non-empty message",
			err:  ErrLimitUnknownType,
		},
		{
			name: "custom period expired has non-empty message",
			err:  ErrLimitCustomPeriodExpired,
		},
		{
			name: "invalid custom start format has non-empty message",
			err:  ErrLimitInvalidCustomStartFormat,
		},
		{
			name: "invalid custom end format has non-empty message",
			err:  ErrLimitInvalidCustomEndFormat,
		},
		{
			name: "custom dates required has non-empty message",
			err:  ErrLimitCustomDatesRequired,
		},
		{
			name: "custom dates order has non-empty message",
			err:  ErrLimitCustomDatesOrder,
		},
		{
			name: "rule name already exists in context has non-empty message",
			err:  ErrRuleNameAlreadyExistsInCtx,
		},
		{
			name: "limit name already exists for rule has non-empty message",
			err:  ErrLimitNameAlreadyExists,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotEmpty(t, tc.err.Error(), "error should have a non-empty message")
		})
	}
}
