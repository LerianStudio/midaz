// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/internal/testutil"
	"tracer/pkg/constant"
)

// newTestLimit creates a valid Limit for testing purposes.
// Returns a Limit with LimitTypeDaily, a sample scope, and a default description.
// The limit is created via NewLimit to ensure all invariants are satisfied.
// Fails the test immediately if NewLimit returns an error.
func newTestLimit(t *testing.T) *Limit {
	t.Helper()

	limit, err := NewLimit(
		"Test Limit",
		LimitTypeDaily,
		decimal.RequireFromString("1000"),
		"USD",
		[]Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(1))}},
		testutil.StringPtr("Test description"),
		testutil.FixedTime(),
	)
	require.NoError(t, err, "newTestLimit: NewLimit failed")

	return limit
}

// newTestLimitWithStatus creates a valid Limit with the specified status for testing.
// Useful for testing status transitions and status-dependent behavior.
// Fails the test immediately if the underlying NewLimit or SetStatus returns an error.
// Uses the public SetStatus API which properly handles DeletedAt invariant for DELETED status.
// Limits now start as DRAFT, so transitions follow: DRAFT → target status.
func newTestLimitWithStatus(t *testing.T, status LimitStatus) *Limit {
	t.Helper()

	limit := newTestLimit(t)

	// Use public SetStatus API to maintain invariants (e.g., DeletedAt for DELETED status).
	// SetStatus is idempotent for same-status transitions (DRAFT → DRAFT is no-op).
	// New limits start as DRAFT, so we need proper transition paths:
	// - DRAFT: no transition needed (initial state)
	// - ACTIVE: DRAFT → ACTIVE
	// - INACTIVE: DRAFT → ACTIVE → INACTIVE (DRAFT cannot go directly to INACTIVE)
	// - DELETED: DRAFT → DELETED (direct deletion of unwanted drafts is allowed)
	if status == LimitStatusDraft {
		return limit
	}

	baseTime := testutil.FixedTime()

	if status == LimitStatusDeleted {
		// DRAFT → DELETED is valid (direct deletion of unwanted drafts)
		err := limit.SetStatus(LimitStatusDeleted, baseTime.Add(1*time.Second))
		require.NoError(t, err, "newTestLimitWithStatus: SetStatus to DELETED failed")

		return limit
	}

	if status == LimitStatusInactive {
		// Must go through ACTIVE first: DRAFT → ACTIVE → INACTIVE
		err := limit.SetStatus(LimitStatusActive, baseTime.Add(1*time.Second))
		require.NoError(t, err, "newTestLimitWithStatus: SetStatus to ACTIVE failed")
	}

	err := limit.SetStatus(status, baseTime.Add(2*time.Second))
	require.NoError(t, err, "newTestLimitWithStatus: SetStatus failed")

	return limit
}

func TestLimitType_IsValid(t *testing.T) {
	tests := []struct {
		name      string
		limitType LimitType
		expected  bool
	}{
		{
			name:      "DAILY is valid",
			limitType: LimitTypeDaily,
			expected:  true,
		},
		{
			name:      "MONTHLY is valid",
			limitType: LimitTypeMonthly,
			expected:  true,
		},
		{
			name:      "PER_TRANSACTION is valid",
			limitType: LimitTypePerTransaction,
			expected:  true,
		},
		{
			name:      "rejects invalid type",
			limitType: LimitType("INVALID"),
			expected:  false,
		},
		{
			name:      "rejects empty string",
			limitType: LimitType(""),
			expected:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.limitType.IsValid()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestLimitStatus_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		status   LimitStatus
		expected bool
	}{
		{
			name:     "ACTIVE is valid",
			status:   LimitStatusActive,
			expected: true,
		},
		{
			name:     "INACTIVE is valid",
			status:   LimitStatusInactive,
			expected: true,
		},
		{
			name:     "DELETED is valid",
			status:   LimitStatusDeleted,
			expected: true,
		},
		{
			name:     "rejects invalid status",
			status:   LimitStatus("INVALID"),
			expected: false,
		},
		{
			name:     "rejects empty string",
			status:   LimitStatus(""),
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.status.IsValid()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCalculateResetAt(t *testing.T) {
	// Fixed reference time: 2025-01-15 10:30:00 UTC
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name      string
		limitType LimitType
		now       time.Time
		expected  *time.Time
	}{
		{
			name:      "DAILY resets at next midnight UTC",
			limitType: LimitTypeDaily,
			now:       now,
			expected:  testutil.Ptr(time.Date(2025, 1, 16, 0, 0, 0, 0, time.UTC)),
		},
		{
			name:      "MONTHLY resets at first of next month",
			limitType: LimitTypeMonthly,
			now:       now,
			expected:  testutil.Ptr(time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)),
		},
		{
			name:      "MONTHLY at end of year wraps to January",
			limitType: LimitTypeMonthly,
			now:       time.Date(2025, 12, 15, 10, 30, 0, 0, time.UTC),
			expected:  testutil.Ptr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		},
		{
			name:      "PER_TRANSACTION returns nil",
			limitType: LimitTypePerTransaction,
			now:       now,
			expected:  nil,
		},
		{
			name:      "invalid type returns nil",
			limitType: LimitType("INVALID"),
			now:       now,
			expected:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := CalculateResetAt(tc.limitType, tc.now)
			if tc.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, *tc.expected, *result)
			}
		})
	}
}

func TestNewLimit(t *testing.T) {
	validScope := Scope{
		AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(10)),
	}

	tests := []struct {
		name                string
		limitName           string
		expectedName        string // if empty, defaults to limitName (for normalization tests)
		limitType           LimitType
		maxAmount           decimal.Decimal
		currency            string
		expectedCurrency    string // if empty, defaults to currency (for normalization tests)
		scopes              []Scope
		description         *string
		expectedDescription *string // if nil and description is set, defaults to description (for normalization tests)
		expectError         bool
		errorIs             error
	}{
		{
			name:        "creates valid limit",
			limitName:   "Daily Card Limit",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"), // $1000.00
			currency:    "USD",
			scopes:      []Scope{validScope},
			description: testutil.StringPtr("Daily spending limit for card transactions"),
			expectError: false,
		},
		{
			name:        "creates limit without description",
			limitName:   "Monthly Limit",
			limitType:   LimitTypeMonthly,
			maxAmount:   decimal.RequireFromString("5000"),
			currency:    "BRL",
			scopes:      []Scope{validScope},
			description: nil,
			expectError: false,
		},
		{
			name:         "trims name whitespace",
			limitName:    "  Trimmed Name  ",
			expectedName: "Trimmed Name",
			limitType:    LimitTypeDaily,
			maxAmount:    decimal.RequireFromString("1000"),
			currency:     "USD",
			scopes:       []Scope{validScope},
			description:  nil,
			expectError:  false,
		},
		{
			name:                "trims description whitespace",
			limitName:           "Test Limit",
			limitType:           LimitTypeDaily,
			maxAmount:           decimal.RequireFromString("1000"),
			currency:            "USD",
			scopes:              []Scope{validScope},
			description:         testutil.StringPtr("  trimmed description  "),
			expectedDescription: testutil.StringPtr("trimmed description"),
			expectError:         false,
		},
		{
			name:        "rejects empty name",
			limitName:   "",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      []Scope{validScope},
			expectError: true,
			errorIs:     constant.ErrLimitNameRequired,
		},
		{
			name:        "rejects whitespace-only name",
			limitName:   "   ",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      []Scope{validScope},
			expectError: true,
			errorIs:     constant.ErrLimitNameRequired,
		},
		{
			name:        "rejects invalid limit type",
			limitName:   "Test Limit",
			limitType:   LimitType("INVALID"),
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      []Scope{validScope},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidType,
		},
		{
			name:        "rejects zero maxAmount",
			limitName:   "Test Limit",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("0"),
			currency:    "USD",
			scopes:      []Scope{validScope},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidMaxAmount,
		},
		{
			name:        "rejects negative maxAmount",
			limitName:   "Test Limit",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("-1"),
			currency:    "USD",
			scopes:      []Scope{validScope},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidMaxAmount,
		},
		{
			name:        "rejects empty currency",
			limitName:   "Test Limit",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "",
			scopes:      []Scope{validScope},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidCurrency,
		},
		{
			name:        "rejects invalid currency length",
			limitName:   "Test Limit",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "US",
			scopes:      []Scope{validScope},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidCurrency,
		},
		{
			name:             "normalizes lowercase currency to uppercase",
			limitName:        "Test Limit",
			limitType:        LimitTypeDaily,
			maxAmount:        decimal.RequireFromString("1000"),
			currency:         "usd",
			expectedCurrency: "USD",
			scopes:           []Scope{validScope},
			expectError:      false,
		},
		{
			name:             "trims and normalizes currency",
			limitName:        "Test Limit",
			limitType:        LimitTypeDaily,
			maxAmount:        decimal.RequireFromString("1000"),
			currency:         "  brl  ",
			expectedCurrency: "BRL",
			scopes:           []Scope{validScope},
			expectError:      false,
		},
		{
			name:        "rejects currency with numbers",
			limitName:   "Test Limit",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "US1",
			scopes:      []Scope{validScope},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidCurrency,
		},
		{
			name:        "rejects scope with invalid TransactionType",
			limitName:   "Test Limit",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      []Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(11)), TransactionType: testutil.Ptr(TransactionType("INVALID"))}},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidScope,
		},
		{
			name:        "accepts name at max length",
			limitName:   strings.Repeat("a", MaxNameLength),
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      []Scope{validScope},
			description: nil,
			expectError: false,
		},
		{
			name:        "rejects name exceeding max length",
			limitName:   strings.Repeat("a", MaxNameLength+1),
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      []Scope{validScope},
			expectError: true,
			errorIs:     constant.ErrLimitNameTooLong,
		},
		{
			name:        "rejects name with XSS characters",
			limitName:   "<script>alert('xss')</script>",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      []Scope{validScope},
			expectError: true,
			errorIs:     constant.ErrLimitNameInvalidChars,
		},
		{
			name:        "rejects name with tab",
			limitName:   "Name\twith\ttabs",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      []Scope{validScope},
			expectError: true,
			errorIs:     constant.ErrLimitNameInvalidChars,
		},
		{
			name:        "rejects name with newline",
			limitName:   "Name\nwith\nnewlines",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      []Scope{validScope},
			expectError: true,
			errorIs:     constant.ErrLimitNameInvalidChars,
		},
		{
			name:        "rejects name with carriage return",
			limitName:   "Name\rwith\rcarriage",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      []Scope{validScope},
			expectError: true,
			errorIs:     constant.ErrLimitNameInvalidChars,
		},
		{
			name:        "rejects empty scopes",
			limitName:   "Test Limit",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      []Scope{},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidScope,
		},
		{
			name:        "rejects nil scopes",
			limitName:   "Test Limit",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      nil,
			expectError: true,
			errorIs:     constant.ErrLimitInvalidScope,
		},
		{
			name:        "rejects empty scope in array",
			limitName:   "Test Limit",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      []Scope{{}},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidScope,
		},
		{
			name:        "rejects description with XSS",
			limitName:   "Test Limit",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      []Scope{validScope},
			description: testutil.StringPtr("<script>alert('xss')</script>"),
			expectError: true,
			errorIs:     constant.ErrLimitDescriptionInvalidChars,
		},
		{
			name:        "rejects description with HTML",
			limitName:   "Test Limit",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      []Scope{validScope},
			description: testutil.StringPtr("Text with <img src=x> tag"),
			expectError: true,
			errorIs:     constant.ErrLimitDescriptionInvalidChars,
		},
		{
			name:        "accepts description at max length",
			limitName:   "Test Limit",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      []Scope{validScope},
			description: testutil.StringPtr(strings.Repeat("a", MaxDescriptionLength)),
			expectError: false,
		},
		{
			name:        "rejects description exceeding max length",
			limitName:   "Test Limit",
			limitType:   LimitTypeDaily,
			maxAmount:   decimal.RequireFromString("1000"),
			currency:    "USD",
			scopes:      []Scope{validScope},
			description: testutil.StringPtr(strings.Repeat("a", MaxDescriptionLength+1)),
			expectError: true,
			errorIs:     constant.ErrLimitDescriptionTooLong,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			limit, err := NewLimit(tc.limitName, tc.limitType, tc.maxAmount, tc.currency, tc.scopes, tc.description, testutil.FixedTime())

			if tc.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.errorIs)
				assert.Nil(t, limit)
			} else {
				require.NoError(t, err)
				require.NotNil(t, limit)

				assert.NotEqual(t, uuid.Nil, limit.ID)

				expectedName := tc.limitName
				if tc.expectedName != "" {
					expectedName = tc.expectedName
				}

				assert.Equal(t, expectedName, limit.Name)

				expectedDescription := tc.description
				if tc.expectedDescription != nil {
					expectedDescription = tc.expectedDescription
				}

				assert.Equal(t, expectedDescription, limit.Description)
				assert.Equal(t, tc.limitType, limit.LimitType)
				assert.True(t, tc.maxAmount.Equal(limit.MaxAmount), "expected MaxAmount %s, got %s", tc.maxAmount, limit.MaxAmount)

				expectedCurrency := tc.currency
				if tc.expectedCurrency != "" {
					expectedCurrency = tc.expectedCurrency
				}

				assert.Equal(t, expectedCurrency, limit.Currency)
				// Scopes ordering is part of NewLimit's contract (see limit.go comment).
				// Using Equal (not ElementsMatch) to verify order preservation.
				assert.Equal(t, tc.scopes, limit.Scopes)
				assert.Equal(t, LimitStatusDraft, limit.Status)
				assert.False(t, limit.CreatedAt.IsZero())
				assert.False(t, limit.UpdatedAt.IsZero())
				assert.Nil(t, limit.DeletedAt)

				// Verify resetAt based on type
				if tc.limitType == LimitTypePerTransaction {
					assert.Nil(t, limit.ResetAt)
				} else {
					assert.NotNil(t, limit.ResetAt)
				}
			}
		})
	}

	t.Run("does not allow external mutation of scopes slice passed to NewLimit", func(t *testing.T) {
		scopes := []Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(12))}}
		limit, err := NewLimit("Test Limit", LimitTypeDaily, decimal.RequireFromString("1000"), "USD", scopes, nil, testutil.FixedTime())
		require.NoError(t, err)

		// mutate caller slice after creation
		scopes[0] = Scope{PortfolioID: testutil.UUIDPtr(testutil.MustDeterministicUUID(13))}

		// limit must remain unchanged
		require.Len(t, limit.Scopes, 1)
		assert.NotNil(t, limit.Scopes[0].AccountID)
	})
}

func TestLimit_Update(t *testing.T) {
	tests := []struct {
		name         string
		updateName   *string
		expectedName string // if non-empty, assert limit.Name equals this (for normalization tests)
		updateMax    *decimal.Decimal
		updateDesc   *string
		expectedDesc string // if non-empty, assert limit.Description equals this (for normalization tests)
		updateScope  *[]Scope
		expectError  bool
		errorIs      error
	}{
		{
			name:        "updates name",
			updateName:  testutil.StringPtr("Updated Name"),
			expectError: false,
		},
		{
			name:         "trims name whitespace",
			updateName:   testutil.StringPtr("  Valid Name  "),
			expectedName: "Valid Name",
			expectError:  false,
		},
		{
			name:        "updates maxAmount",
			updateMax:   testutil.Ptr(decimal.RequireFromString("2000")),
			expectError: false,
		},
		{
			name:        "updates description",
			updateDesc:  testutil.StringPtr("Updated description"),
			expectError: false,
		},
		{
			name:         "trims description whitespace",
			updateDesc:   testutil.StringPtr("  Trimmed Description  "),
			expectedDesc: "Trimmed Description",
			expectError:  false,
		},
		{
			name:        "updates scopes",
			updateScope: &[]Scope{{PortfolioID: testutil.UUIDPtr(testutil.MustDeterministicUUID(14))}},
			expectError: false,
		},
		{
			name:        "updates multiple fields",
			updateName:  testutil.StringPtr("Multi Update"),
			updateMax:   testutil.Ptr(decimal.RequireFromString("3000")),
			expectError: false,
		},
		{
			name:        "rejects empty name",
			updateName:  testutil.StringPtr(""),
			expectError: true,
			errorIs:     constant.ErrLimitNameRequired,
		},
		{
			name:        "rejects whitespace-only name",
			updateName:  testutil.StringPtr("   "),
			expectError: true,
			errorIs:     constant.ErrLimitNameRequired,
		},
		{
			name:        "rejects zero maxAmount",
			updateMax:   testutil.Ptr(decimal.RequireFromString("0")),
			expectError: true,
			errorIs:     constant.ErrLimitInvalidMaxAmount,
		},
		{
			name:        "rejects negative maxAmount",
			updateMax:   testutil.Ptr(decimal.RequireFromString("-1")),
			expectError: true,
			errorIs:     constant.ErrLimitInvalidMaxAmount,
		},
		{
			name:        "rejects empty scopes",
			updateScope: &[]Scope{},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidScope,
		},
		{
			name:        "rejects invalid scope",
			updateScope: &[]Scope{{}},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidScope,
		},
		{
			name:        "rejects scope with invalid TransactionType",
			updateScope: &[]Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(15)), TransactionType: testutil.Ptr(TransactionType("INVALID"))}},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidScope,
		},
		{
			name:        "rejects description with XSS",
			updateDesc:  testutil.StringPtr("<script>alert('xss')</script>"),
			expectError: true,
			errorIs:     constant.ErrLimitDescriptionInvalidChars,
		},
		{
			name:        "rejects description with HTML",
			updateDesc:  testutil.StringPtr("Valid text <b>bold</b> more text"),
			expectError: true,
			errorIs:     constant.ErrLimitDescriptionInvalidChars,
		},
		{
			name:        "accepts description with special chars",
			updateDesc:  testutil.StringPtr("Valid: $100 @ 5% rate & more!"),
			expectError: false,
		},
		{
			name:        "rejects description exceeding max length",
			updateDesc:  testutil.StringPtr(strings.Repeat("a", MaxDescriptionLength+1)),
			expectError: true,
			errorIs:     constant.ErrLimitDescriptionTooLong,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			limit := newTestLimit(t)
			// Set a deterministic past time to detect UpdatedAt changes without sleeping
			originalUpdatedAt := testutil.FixedTime().Add(-1 * time.Minute)
			limit.UpdatedAt = originalUpdatedAt

			// Capture original state to verify no partial mutation on error
			originalName := limit.Name
			originalMaxAmount := limit.MaxAmount
			originalDescription := limit.Description
			originalScopes := make([]Scope, len(limit.Scopes))
			copy(originalScopes, limit.Scopes)

			err := limit.Update(tc.updateName, tc.updateMax, tc.updateDesc, tc.updateScope, nil, nil, nil, nil, testutil.FixedTime())

			if tc.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.errorIs)

				// Verify no partial mutation occurred
				assert.Equal(t, originalName, limit.Name, "Name should not change on error")
				assert.True(t, originalMaxAmount.Equal(limit.MaxAmount), "MaxAmount should not change on error")
				assert.Equal(t, originalDescription, limit.Description, "Description should not change on error")
				assert.Equal(t, originalScopes, limit.Scopes, "Scopes should not change on error")
				assert.Equal(t, originalUpdatedAt, limit.UpdatedAt, "UpdatedAt should not change on error")
			} else {
				require.NoError(t, err)

				if tc.updateName != nil {
					expectedName := *tc.updateName
					if tc.expectedName != "" {
						expectedName = tc.expectedName
					}
					assert.Equal(t, expectedName, limit.Name)
				}
				if tc.updateMax != nil {
					assert.True(t, (*tc.updateMax).Equal(limit.MaxAmount), "expected MaxAmount %s, got %s", *tc.updateMax, limit.MaxAmount)
				}
				if tc.updateDesc != nil {
					require.NotNil(t, limit.Description, "Description should not be nil after update")
					expectedDesc := *tc.updateDesc
					if tc.expectedDesc != "" {
						expectedDesc = tc.expectedDesc
					}
					assert.Equal(t, expectedDesc, *limit.Description)
				}
				if tc.updateScope != nil {
					assert.Equal(t, *tc.updateScope, limit.Scopes)
				}

				assert.True(t, limit.UpdatedAt.After(originalUpdatedAt))
			}
		})
	}
}

func TestLimit_Update_NoChanges(t *testing.T) {
	// L1 fix: UpdatedAt should NOT change when no fields are modified
	tests := []struct {
		name string
	}{
		{
			name: "does not change UpdatedAt when no changes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			limit := newTestLimit(t)
			// Use a deterministic fixed time to verify it remains unchanged
			fixedTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			limit.UpdatedAt = fixedTime

			// Call Update with all nil parameters
			err := limit.Update(nil, nil, nil, nil, nil, nil, nil, nil, testutil.FixedTime())

			require.NoError(t, err)
			assert.Equal(t, fixedTime, limit.UpdatedAt, "UpdatedAt should not change when no fields are modified")
		})
	}
}

func TestLimit_SetStatus(t *testing.T) {
	tests := []struct {
		name           string
		initialStatus  LimitStatus
		newStatus      LimitStatus
		expectedErr    error
		checkDeleted   bool
		isIdempotentOp bool
	}{
		{
			name:          "ACTIVE to INACTIVE",
			initialStatus: LimitStatusActive,
			newStatus:     LimitStatusInactive,
			expectedErr:   nil,
		},
		{
			name:          "rejects ACTIVE to DELETED (must deactivate first)",
			initialStatus: LimitStatusActive,
			newStatus:     LimitStatusDeleted,
			expectedErr:   constant.ErrLimitInvalidStatusChange,
		},
		{
			name:          "INACTIVE to ACTIVE",
			initialStatus: LimitStatusInactive,
			newStatus:     LimitStatusActive,
			expectedErr:   nil,
		},
		{
			name:          "INACTIVE to DELETED sets DeletedAt",
			initialStatus: LimitStatusInactive,
			newStatus:     LimitStatusDeleted,
			expectedErr:   nil,
			checkDeleted:  true,
		},
		{
			name:           "ACTIVE to ACTIVE is idempotent no-op",
			initialStatus:  LimitStatusActive,
			newStatus:      LimitStatusActive,
			expectedErr:    nil,
			isIdempotentOp: true,
		},
		{
			name:          "rejects DELETED to ACTIVE",
			initialStatus: LimitStatusDeleted,
			newStatus:     LimitStatusActive,
			expectedErr:   constant.ErrLimitInvalidStatusChange,
		},
		{
			name:          "rejects DELETED to INACTIVE",
			initialStatus: LimitStatusDeleted,
			newStatus:     LimitStatusInactive,
			expectedErr:   constant.ErrLimitInvalidStatusChange,
		},
		{
			name:          "INACTIVE to DRAFT (recovery)",
			initialStatus: LimitStatusInactive,
			newStatus:     LimitStatusDraft,
			expectedErr:   nil,
		},
		{
			name:           "DRAFT to DRAFT is idempotent no-op",
			initialStatus:  LimitStatusDraft,
			newStatus:      LimitStatusDraft,
			expectedErr:    nil,
			isIdempotentOp: true,
		},
		{
			name:          "rejects ACTIVE to DRAFT (must deactivate first)",
			initialStatus: LimitStatusActive,
			newStatus:     LimitStatusDraft,
			expectedErr:   constant.ErrLimitInvalidStatusChange,
		},
		{
			name:          "rejects DELETED to DRAFT (terminal state)",
			initialStatus: LimitStatusDeleted,
			newStatus:     LimitStatusDraft,
			expectedErr:   constant.ErrLimitInvalidStatusChange,
		},
		{
			name:          "rejects invalid status",
			initialStatus: LimitStatusActive,
			newStatus:     LimitStatus("INVALID"),
			expectedErr:   constant.ErrLimitInvalidStatusChange,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			limit := newTestLimitWithStatus(t, tc.initialStatus)
			// Set a deterministic past time to detect UpdatedAt changes without sleeping
			originalUpdatedAt := testutil.FixedTime().Add(-1 * time.Minute)
			limit.UpdatedAt = originalUpdatedAt

			// Capture original values to verify no mutation on failure
			originalStatus := limit.Status
			originalDeletedAt := limit.DeletedAt

			err := limit.SetStatus(tc.newStatus, testutil.FixedTime())

			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.expectedErr)

				// Verify no mutation occurred on failure
				assert.Equal(t, originalStatus, limit.Status, "Status should not change on error")
				assert.Equal(t, originalUpdatedAt, limit.UpdatedAt, "UpdatedAt should not change on error")
				assert.Equal(t, originalDeletedAt, limit.DeletedAt, "DeletedAt should not change on error")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.newStatus, limit.Status)

				if tc.isIdempotentOp {
					// Idempotent operations should NOT update timestamp
					assert.Equal(t, originalUpdatedAt, limit.UpdatedAt)
				} else {
					assert.True(t, limit.UpdatedAt.After(originalUpdatedAt))
				}

				// Assert DeletedAt invariant based on target status
				if tc.checkDeleted {
					assert.NotNil(t, limit.DeletedAt, "DeletedAt must be set for DELETED status")
				} else {
					assert.Nil(t, limit.DeletedAt, "DeletedAt must be nil for non-DELETED status")
				}
			}
		})
	}
}

func TestLimit_IsActive(t *testing.T) {
	tests := []struct {
		name     string
		status   LimitStatus
		expected bool
	}{
		{
			name:     "ACTIVE returns true",
			status:   LimitStatusActive,
			expected: true,
		},
		{
			name:     "INACTIVE returns false",
			status:   LimitStatusInactive,
			expected: false,
		},
		{
			name:     "DELETED returns false",
			status:   LimitStatusDeleted,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			limit := &Limit{Status: tc.status}
			assert.Equal(t, tc.expected, limit.IsActive())
		})
	}
}

func TestLimit_Validate(t *testing.T) {
	validScope := Scope{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(20))}

	tests := []struct {
		name        string
		limit       *Limit
		expectError bool
		errorIs     error
	}{
		{
			name: "valid limit passes",
			limit: &Limit{
				ID:        testutil.MustDeterministicUUID(21),
				Name:      "Valid Limit",
				LimitType: LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []Scope{validScope},
				Status:    LimitStatusActive,
				CreatedAt: testutil.FixedTime(),
				UpdatedAt: testutil.FixedTime(),
			},
			expectError: false,
		},
		{
			name: "rejects empty name",
			limit: &Limit{
				ID:        testutil.MustDeterministicUUID(22),
				Name:      "",
				LimitType: LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []Scope{validScope},
				Status:    LimitStatusActive,
			},
			expectError: true,
			errorIs:     constant.ErrLimitNameRequired,
		},
		{
			name: "rejects name with invalid characters (loaded from DB)",
			limit: &Limit{
				ID:        testutil.MustDeterministicUUID(23),
				Name:      "<script>alert('xss')</script>",
				LimitType: LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []Scope{validScope},
				Status:    LimitStatusActive,
			},
			expectError: true,
			errorIs:     constant.ErrLimitNameInvalidChars,
		},
		{
			name: "rejects name exceeding max length (loaded from DB)",
			limit: &Limit{
				ID:        testutil.MustDeterministicUUID(24),
				Name:      strings.Repeat("a", MaxNameLength+1),
				LimitType: LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []Scope{validScope},
				Status:    LimitStatusActive,
			},
			expectError: true,
			errorIs:     constant.ErrLimitNameTooLong,
		},
		{
			name: "rejects invalid limit type",
			limit: &Limit{
				ID:        testutil.MustDeterministicUUID(25),
				Name:      "Test",
				LimitType: LimitType("INVALID"),
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []Scope{validScope},
				Status:    LimitStatusActive,
			},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidType,
		},
		{
			name: "rejects zero maxAmount",
			limit: &Limit{
				ID:        testutil.MustDeterministicUUID(26),
				Name:      "Test",
				LimitType: LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("0"),
				Currency:  "USD",
				Scopes:    []Scope{validScope},
				Status:    LimitStatusActive,
			},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidMaxAmount,
		},
		{
			name: "rejects invalid currency",
			limit: &Limit{
				ID:        testutil.MustDeterministicUUID(27),
				Name:      "Test",
				LimitType: LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "US",
				Scopes:    []Scope{validScope},
				Status:    LimitStatusActive,
			},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidCurrency,
		},
		{
			name: "rejects empty scopes",
			limit: &Limit{
				ID:        testutil.MustDeterministicUUID(28),
				Name:      "Test",
				LimitType: LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []Scope{},
				Status:    LimitStatusActive,
			},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidScope,
		},
		{
			name: "rejects invalid status",
			limit: &Limit{
				ID:        testutil.MustDeterministicUUID(29),
				Name:      "Test",
				LimitType: LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []Scope{validScope},
				Status:    LimitStatus("INVALID"),
			},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidStatusChange,
		},
		{
			name: "rejects DELETED without DeletedAt",
			limit: &Limit{
				ID:        testutil.MustDeterministicUUID(30),
				Name:      "Test",
				LimitType: LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []Scope{validScope},
				Status:    LimitStatusDeleted,
				DeletedAt: nil,
			},
			expectError: true,
			errorIs:     constant.ErrLimitDeletedAtInvariant,
		},
		{
			name: "rejects non-DELETED with DeletedAt",
			limit: &Limit{
				ID:        testutil.MustDeterministicUUID(31),
				Name:      "Test",
				LimitType: LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []Scope{validScope},
				Status:    LimitStatusActive,
				DeletedAt: testutil.Ptr(testutil.FixedTime()),
			},
			expectError: true,
			errorIs:     constant.ErrLimitDeletedAtInvariant,
		},
		{
			name: "DELETED with DeletedAt passes",
			limit: &Limit{
				ID:        testutil.MustDeterministicUUID(32),
				Name:      "Test",
				LimitType: LimitTypeDaily,
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "USD",
				Scopes:    []Scope{validScope},
				Status:    LimitStatusDeleted,
				DeletedAt: testutil.Ptr(testutil.FixedTime()),
				CreatedAt: testutil.FixedTime(),
				UpdatedAt: testutil.FixedTime(),
			},
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.limit.Validate()

			if tc.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.errorIs)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNewUsageCounter(t *testing.T) {
	limitID := testutil.MustDeterministicUUID(40)

	tests := []struct {
		name              string
		limitID           uuid.UUID
		scopeKey          string
		periodKey         string
		expectedErr       error
		expectedScopeKey  string // if set, assert ScopeKey equals this (for normalization tests)
		expectedPeriodKey string // if set, assert PeriodKey equals this (for normalization tests)
	}{
		{
			name:        "creates valid counter",
			limitID:     limitID,
			scopeKey:    "acct:abc-123",
			periodKey:   "2025-01-15",
			expectedErr: nil,
		},
		{
			name:             "trims scopeKey whitespace",
			limitID:          limitID,
			scopeKey:         "  acct:abc-123  ",
			periodKey:        "2025-01-15",
			expectedErr:      nil,
			expectedScopeKey: "acct:abc-123",
		},
		{
			name:              "trims periodKey whitespace",
			limitID:           limitID,
			scopeKey:          "acct:abc-123",
			periodKey:         "  2025-01-15  ",
			expectedErr:       nil,
			expectedPeriodKey: "2025-01-15",
		},
		{
			name:        "rejects nil limitID",
			limitID:     uuid.Nil,
			scopeKey:    "acct:abc-123",
			periodKey:   "2025-01-15",
			expectedErr: constant.ErrUsageCounterLimitIDRequired,
		},
		{
			name:        "rejects empty scopeKey",
			limitID:     limitID,
			scopeKey:    "",
			periodKey:   "2025-01-15",
			expectedErr: constant.ErrUsageCounterScopeKeyRequired,
		},
		{
			name:        "rejects whitespace-only scopeKey",
			limitID:     limitID,
			scopeKey:    "   \t  ",
			periodKey:   "2025-01-15",
			expectedErr: constant.ErrUsageCounterScopeKeyRequired,
		},
		{
			name:        "rejects empty periodKey",
			limitID:     limitID,
			scopeKey:    "acct:abc-123",
			periodKey:   "",
			expectedErr: constant.ErrUsageCounterPeriodKeyRequired,
		},
		{
			name:        "rejects whitespace-only periodKey",
			limitID:     limitID,
			scopeKey:    "acct:abc-123",
			periodKey:   "   \t  ",
			expectedErr: constant.ErrUsageCounterPeriodKeyRequired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			counter, err := NewUsageCounter(tc.limitID, tc.scopeKey, tc.periodKey, testutil.FixedTime())

			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.expectedErr)
				assert.Nil(t, counter)
			} else {
				require.NoError(t, err)
				require.NotNil(t, counter)

				assert.NotEqual(t, uuid.Nil, counter.ID)
				assert.Equal(t, tc.limitID, counter.LimitID)

				expectedScopeKey := tc.scopeKey
				if tc.expectedScopeKey != "" {
					expectedScopeKey = tc.expectedScopeKey
				}
				assert.Equal(t, expectedScopeKey, counter.ScopeKey)

				expectedPeriodKey := tc.periodKey
				if tc.expectedPeriodKey != "" {
					expectedPeriodKey = tc.expectedPeriodKey
				}
				assert.Equal(t, expectedPeriodKey, counter.PeriodKey)

				assert.True(t, decimal.RequireFromString("0").Equal(counter.CurrentUsage))
				assert.False(t, counter.LastUpdatedAt.IsZero())
			}
		})
	}
}

func TestUsageCounter_Increment(t *testing.T) {
	createCounter := func(t *testing.T) *UsageCounter {
		t.Helper()

		counter, err := NewUsageCounter(testutil.MustDeterministicUUID(41), "acct:123", "2025-01", testutil.FixedTime())
		require.NoError(t, err, "NewUsageCounter failed")

		return counter
	}

	tests := []struct {
		name              string
		amount            decimal.Decimal
		expectedUsage     decimal.Decimal
		expectedErr       error
		initialUsage      decimal.Decimal
		expectTimeChanged bool
	}{
		{
			name:              "increments by positive amount",
			amount:            decimal.RequireFromString("10"),
			expectedUsage:     decimal.RequireFromString("10"),
			expectedErr:       nil,
			expectTimeChanged: true,
		},
		{
			name:              "zero increment does not update timestamp",
			amount:            decimal.RequireFromString("0"),
			expectedUsage:     decimal.RequireFromString("0"),
			expectedErr:       nil,
			expectTimeChanged: false,
		},
		{
			name:              "accumulates multiple increments",
			amount:            decimal.RequireFromString("5"),
			initialUsage:      decimal.RequireFromString("10"),
			expectedUsage:     decimal.RequireFromString("15"),
			expectedErr:       nil,
			expectTimeChanged: true,
		},
		{
			name:        "rejects negative amount",
			amount:      decimal.RequireFromString("-1"),
			expectedErr: constant.ErrUsageCounterIncrementNonNegative,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			counter := createCounter(t)
			if tc.initialUsage.IsPositive() {
				counter.CurrentUsage = tc.initialUsage
			}
			// Set a deterministic past time to detect LastUpdatedAt changes without sleeping
			originalUpdatedAt := testutil.FixedTime().Add(-1 * time.Minute)
			counter.LastUpdatedAt = originalUpdatedAt

			// Capture original state to verify no mutation on error
			originalUsage := counter.CurrentUsage

			err := counter.Increment(tc.amount, testutil.FixedTime())

			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.expectedErr)

				// Verify no mutation occurred on error
				assert.True(t, originalUsage.Equal(counter.CurrentUsage), "CurrentUsage should not change on error")
				assert.Equal(t, originalUpdatedAt, counter.LastUpdatedAt, "LastUpdatedAt should not change on error")
			} else {
				require.NoError(t, err)
				assert.True(t, tc.expectedUsage.Equal(counter.CurrentUsage), "expected %s, got %s", tc.expectedUsage, counter.CurrentUsage)

				if tc.expectTimeChanged {
					assert.True(t, counter.LastUpdatedAt.After(originalUpdatedAt))
				} else {
					assert.Equal(t, originalUpdatedAt, counter.LastUpdatedAt)
				}
			}
		})
	}
}

func TestUsageCounter_Validate(t *testing.T) {
	tests := []struct {
		name        string
		counter     *UsageCounter
		expectedErr error
	}{
		{
			name: "valid counter passes",
			counter: &UsageCounter{
				ID:            testutil.MustDeterministicUUID(50),
				LimitID:       testutil.MustDeterministicUUID(51),
				ScopeKey:      "acct:123",
				PeriodKey:     "2025-01",
				CurrentUsage:  decimal.RequireFromString("10"),
				LastUpdatedAt: testutil.FixedTime(),
			},
			expectedErr: nil,
		},
		{
			name: "rejects nil limitID",
			counter: &UsageCounter{
				ID:            testutil.MustDeterministicUUID(52),
				LimitID:       uuid.Nil,
				ScopeKey:      "acct:123",
				PeriodKey:     "2025-01",
				CurrentUsage:  decimal.RequireFromString("0"),
				LastUpdatedAt: testutil.FixedTime(),
			},
			expectedErr: constant.ErrUsageCounterLimitIDRequired,
		},
		{
			name: "rejects empty scopeKey",
			counter: &UsageCounter{
				ID:            testutil.MustDeterministicUUID(53),
				LimitID:       testutil.MustDeterministicUUID(54),
				ScopeKey:      "",
				PeriodKey:     "2025-01",
				CurrentUsage:  decimal.RequireFromString("0"),
				LastUpdatedAt: testutil.FixedTime(),
			},
			expectedErr: constant.ErrUsageCounterScopeKeyRequired,
		},
		{
			name: "rejects empty periodKey",
			counter: &UsageCounter{
				ID:            testutil.MustDeterministicUUID(55),
				LimitID:       testutil.MustDeterministicUUID(56),
				ScopeKey:      "acct:123",
				PeriodKey:     "",
				CurrentUsage:  decimal.RequireFromString("0"),
				LastUpdatedAt: testutil.FixedTime(),
			},
			expectedErr: constant.ErrUsageCounterPeriodKeyRequired,
		},
		{
			name: "rejects negative currentUsage",
			counter: &UsageCounter{
				ID:            testutil.MustDeterministicUUID(57),
				LimitID:       testutil.MustDeterministicUUID(58),
				ScopeKey:      "acct:123",
				PeriodKey:     "2025-01",
				CurrentUsage:  decimal.RequireFromString("-1"),
				LastUpdatedAt: testutil.FixedTime(),
			},
			expectedErr: constant.ErrUsageCounterCurrentUsageNegative,
		},
		{
			name: "rejects whitespace-only scopeKey",
			counter: &UsageCounter{
				ID:            testutil.MustDeterministicUUID(59),
				LimitID:       testutil.MustDeterministicUUID(60),
				ScopeKey:      "   \t  ",
				PeriodKey:     "2025-01",
				CurrentUsage:  decimal.RequireFromString("0"),
				LastUpdatedAt: testutil.FixedTime(),
			},
			expectedErr: constant.ErrUsageCounterScopeKeyRequired,
		},
		{
			name: "rejects whitespace-only periodKey",
			counter: &UsageCounter{
				ID:            testutil.MustDeterministicUUID(61),
				LimitID:       testutil.MustDeterministicUUID(62),
				ScopeKey:      "acct:123",
				PeriodKey:     "   \t  ",
				CurrentUsage:  decimal.RequireFromString("0"),
				LastUpdatedAt: testutil.FixedTime(),
			},
			expectedErr: constant.ErrUsageCounterPeriodKeyRequired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.counter.Validate()

			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestListLimitsFilter_ApplyDefaults(t *testing.T) {
	tests := []struct {
		name           string
		filter         ListLimitsFilter
		expectedLimit  int
		expectedSortBy string
		expectedOrder  string
	}{
		{
			name:           "empty filter applies all defaults",
			filter:         ListLimitsFilter{},
			expectedLimit:  constant.DefaultPaginationLimit,
			expectedSortBy: DefaultLimitSortField,
			expectedOrder:  string(constant.Desc),
		},
		{
			name: "preserves valid values",
			filter: ListLimitsFilter{
				Limit:     50,
				SortBy:    "name",
				SortOrder: string(constant.Asc),
			},
			expectedLimit:  50,
			expectedSortBy: "name",
			expectedOrder:  string(constant.Asc),
		},
		{
			name: "caps limit at MaxPaginationLimit",
			filter: ListLimitsFilter{
				Limit: 200,
			},
			expectedLimit:  constant.MaxPaginationLimit,
			expectedSortBy: DefaultLimitSortField,
			expectedOrder:  string(constant.Desc),
		},
		{
			name: "negative limit gets default",
			filter: ListLimitsFilter{
				Limit: -10,
			},
			expectedLimit:  constant.DefaultPaginationLimit,
			expectedSortBy: DefaultLimitSortField,
			expectedOrder:  string(constant.Desc),
		},
		{
			name: "normalizes sort order to lowercase",
			filter: ListLimitsFilter{
				SortOrder: "ASC",
			},
			expectedLimit:  constant.DefaultPaginationLimit,
			expectedSortBy: DefaultLimitSortField,
			expectedOrder:  string(constant.Asc),
		},
		{
			name: "preserves non-empty sort by value",
			filter: ListLimitsFilter{
				SortBy: "updated_at",
			},
			expectedLimit:  constant.DefaultPaginationLimit,
			expectedSortBy: "updated_at",
			expectedOrder:  string(constant.Desc),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.filter.ApplyDefaults()

			assert.Equal(t, tc.expectedLimit, tc.filter.Limit)
			assert.Equal(t, tc.expectedSortBy, tc.filter.SortBy)
			assert.Equal(t, tc.expectedOrder, tc.filter.SortOrder)
		})
	}
}

func TestListLimitsFilter_Validate(t *testing.T) {
	tests := []struct {
		name        string
		filter      ListLimitsFilter
		expectError bool
		errorIs     error
	}{
		{
			name: "valid filter with all fields",
			filter: ListLimitsFilter{
				Status:    testutil.Ptr(LimitStatusActive),
				LimitType: testutil.Ptr(LimitTypeDaily),
				Limit:     50,
				SortBy:    "name",
				SortOrder: string(constant.Asc),
			},
			expectError: false,
		},
		{
			name: "rejects invalid case for sort field (uppercase rejected)",
			filter: ListLimitsFilter{
				Limit:  10,
				SortBy: "NAME",
			},
			expectError: true,
			errorIs:     constant.ErrInvalidSortColumn,
		},
		{
			name: "rejects invalid status",
			filter: ListLimitsFilter{
				Limit:  10,
				Status: testutil.Ptr(LimitStatus("INVALID")),
			},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidStatusFilter,
		},
		{
			name: "rejects invalid limit type",
			filter: ListLimitsFilter{
				Limit:     10,
				LimitType: testutil.Ptr(LimitType("INVALID")),
			},
			expectError: true,
			errorIs:     constant.ErrLimitInvalidTypeFilter,
		},
		{
			name: "rejects zero limit",
			filter: ListLimitsFilter{
				Limit: 0,
			},
			expectError: true,
			errorIs:     constant.ErrPaginationLimitInvalid,
		},
		{
			name: "rejects negative limit",
			filter: ListLimitsFilter{
				Limit: -10,
			},
			expectError: true,
			errorIs:     constant.ErrPaginationLimitInvalid,
		},
		{
			name: "rejects limit exceeding maximum",
			filter: ListLimitsFilter{
				Limit: constant.MaxPaginationLimit + 1,
			},
			expectError: true,
			errorIs:     constant.ErrPaginationLimitExceeded,
		},
		{
			name: "accepts snake_case sort field created_at",
			filter: ListLimitsFilter{
				Limit:  10,
				SortBy: "created_at",
			},
			expectError: false,
		},
		{
			name: "accepts snake_case sort field updated_at",
			filter: ListLimitsFilter{
				Limit:  10,
				SortBy: "updated_at",
			},
			expectError: false,
		},
		{
			name: "accepts snake_case sort field max_amount",
			filter: ListLimitsFilter{
				Limit:  10,
				SortBy: "max_amount",
			},
			expectError: false,
		},
		{
			name: "rejects camelCase sort field createdAt",
			filter: ListLimitsFilter{
				Limit:  10,
				SortBy: "createdAt",
			},
			expectError: true,
			errorIs:     constant.ErrInvalidSortColumn,
		},
		{
			name: "rejects invalid sort field",
			filter: ListLimitsFilter{
				Limit:  10,
				SortBy: "invalid_field",
			},
			expectError: true,
			errorIs:     constant.ErrInvalidSortColumn,
		},
		{
			name: "rejects invalid sort order",
			filter: ListLimitsFilter{
				Limit:     10,
				SortOrder: "random",
			},
			expectError: true,
			errorIs:     constant.ErrInvalidSortOrder,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.filter.Validate()

			if tc.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.errorIs)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestUsageCounter_ScanFields verifies that ScanFields returns the correct number of
// field pointers matching the UsageCounter struct fields. This test will fail if fields
// are added/removed from UsageCounter without updating ScanFields, catching drift early.
func TestUsageCounter_ScanFields(t *testing.T) {
	counter := &UsageCounter{
		ID:            testutil.MustDeterministicUUID(70),
		LimitID:       testutil.MustDeterministicUUID(71),
		ScopeKey:      "acct:123",
		PeriodKey:     "2025-01",
		CurrentUsage:  decimal.RequireFromString("50"),
		LastUpdatedAt: testutil.FixedTime(),
	}

	scanFields := counter.ScanFields()

	// Use reflection to count the actual number of fields in UsageCounter struct
	expectedFieldCount := reflect.TypeOf(UsageCounter{}).NumField()

	assert.Equal(t, expectedFieldCount, len(scanFields),
		"ScanFields() returned %d pointers but UsageCounter has %d fields; update ScanFields to match struct",
		len(scanFields), expectedFieldCount)

	// Verify all returned values are non-nil pointers
	for i, field := range scanFields {
		assert.NotNil(t, field, "ScanFields()[%d] should not be nil", i)
	}
}

// TestNewUsageSnapshot_DailyLimit tests UsageSnapshot creation for DAILY limits.
func TestNewUsageSnapshot_DailyLimit(t *testing.T) {
	limit := newTestLimit(t) // Creates a DAILY limit with MaxAmount=1000

	counters := []UsageCounter{
		{CurrentUsage: decimal.RequireFromString("300")},
		{CurrentUsage: decimal.RequireFromString("200")},
	}

	snapshot := NewUsageSnapshot(limit, counters)

	assert.Equal(t, limit.ID, snapshot.LimitID)
	assert.True(t, decimal.RequireFromString("500").Equal(snapshot.CurrentUsage), "should sum all counters")
	assert.True(t, decimal.RequireFromString("1000").Equal(snapshot.LimitAmount))
	assert.Equal(t, 50.0, snapshot.UtilizationPercent)
	assert.False(t, snapshot.NearLimit, "50% should not be near limit")
	assert.NotNil(t, snapshot.ResetAt, "DAILY limit should have resetAt")
}

// TestNewUsageSnapshot_NearLimitThreshold tests nearLimit flag at boundary.
func TestNewUsageSnapshot_NearLimitThreshold(t *testing.T) {
	tests := []struct {
		name            string
		currentUsage    decimal.Decimal
		maxAmount       decimal.Decimal
		expectedNear    bool
		expectedPercent float64
	}{
		{
			name:            "at 80% - not near (>80%, not >=80%)",
			currentUsage:    decimal.RequireFromString("800"),
			maxAmount:       decimal.RequireFromString("1000"),
			expectedNear:    false,
			expectedPercent: 80.0,
		},
		{
			name:            "at 80.01% - near",
			currentUsage:    decimal.RequireFromString("800.10"),
			maxAmount:       decimal.RequireFromString("1000"),
			expectedNear:    true,
			expectedPercent: 80.01,
		},
		{
			name:            "at 85% - near",
			currentUsage:    decimal.RequireFromString("850"),
			maxAmount:       decimal.RequireFromString("1000"),
			expectedNear:    true,
			expectedPercent: 85.0,
		},
		{
			name:            "at 100% - near",
			currentUsage:    decimal.RequireFromString("1000"),
			maxAmount:       decimal.RequireFromString("1000"),
			expectedNear:    true,
			expectedPercent: 100.0,
		},
		{
			name:            "at 0% - not near",
			currentUsage:    decimal.RequireFromString("0"),
			maxAmount:       decimal.RequireFromString("1000"),
			expectedNear:    false,
			expectedPercent: 0.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			limit, err := NewLimit(
				"Test Limit",
				LimitTypeDaily,
				tc.maxAmount,
				"USD",
				[]Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(80))}},
				nil,
				testutil.FixedTime(),
			)
			require.NoError(t, err)

			counters := []UsageCounter{{CurrentUsage: tc.currentUsage}}

			snapshot := NewUsageSnapshot(limit, counters)

			assert.Equal(t, tc.expectedNear, snapshot.NearLimit)
			assert.InDelta(t, tc.expectedPercent, snapshot.UtilizationPercent, 0.01)
		})
	}
}

// TestNewUsageSnapshot_PerTransactionLimit tests PER_TRANSACTION limits have zero usage and nil resetAt.
func TestNewUsageSnapshot_PerTransactionLimit(t *testing.T) {
	limit, err := NewLimit(
		"Per Transaction Limit",
		LimitTypePerTransaction,
		decimal.RequireFromString("1000"),
		"USD",
		[]Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(81))}},
		nil,
		testutil.FixedTime(),
	)
	require.NoError(t, err)

	// Even with counters (which shouldn't exist for PER_TRANSACTION), currentUsage should be 0
	counters := []UsageCounter{
		{CurrentUsage: decimal.RequireFromString("300")},
		{CurrentUsage: decimal.RequireFromString("200")},
	}

	snapshot := NewUsageSnapshot(limit, counters)

	assert.Equal(t, limit.ID, snapshot.LimitID)
	assert.True(t, decimal.RequireFromString("0").Equal(snapshot.CurrentUsage), "PER_TRANSACTION should always have 0 usage")
	assert.True(t, decimal.RequireFromString("1000").Equal(snapshot.LimitAmount))
	assert.Equal(t, 0.0, snapshot.UtilizationPercent)
	assert.False(t, snapshot.NearLimit)
	assert.Nil(t, snapshot.ResetAt, "PER_TRANSACTION should have nil resetAt")
}

// TestNewUsageSnapshot_EmptyCounters tests snapshot with no counters.
func TestNewUsageSnapshot_EmptyCounters(t *testing.T) {
	limit := newTestLimit(t)

	snapshot := NewUsageSnapshot(limit, []UsageCounter{})

	assert.True(t, decimal.RequireFromString("0").Equal(snapshot.CurrentUsage))
	assert.Equal(t, 0.0, snapshot.UtilizationPercent)
	assert.False(t, snapshot.NearLimit)
}

// TestNewUsageSnapshot_NilCounters tests snapshot with nil counters slice.
func TestNewUsageSnapshot_NilCounters(t *testing.T) {
	limit := newTestLimit(t)

	snapshot := NewUsageSnapshot(limit, nil)

	assert.True(t, decimal.RequireFromString("0").Equal(snapshot.CurrentUsage))
	assert.Equal(t, 0.0, snapshot.UtilizationPercent)
	assert.False(t, snapshot.NearLimit)
}

// TestNewUsageSnapshot_MonthlyLimit tests MONTHLY limits include resetAt.
func TestNewUsageSnapshot_MonthlyLimit(t *testing.T) {
	limit, err := NewLimit(
		"Monthly Limit",
		LimitTypeMonthly,
		decimal.RequireFromString("10000"),
		"USD",
		[]Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(82))}},
		nil,
		testutil.FixedTime(),
	)
	require.NoError(t, err)

	counters := []UsageCounter{{CurrentUsage: decimal.RequireFromString("5000")}}

	snapshot := NewUsageSnapshot(limit, counters)

	assert.True(t, decimal.RequireFromString("5000").Equal(snapshot.CurrentUsage))
	assert.True(t, decimal.RequireFromString("10000").Equal(snapshot.LimitAmount))
	assert.Equal(t, 50.0, snapshot.UtilizationPercent)
	assert.NotNil(t, snapshot.ResetAt, "MONTHLY limit should have resetAt")
}

func TestDefaultLimitSortField_IsSnakeCase(t *testing.T) {
	assert.Equal(t, "created_at", DefaultLimitSortField)
}

// TestNewLimit_NormalizesScopeSubType verifies that NewLimit normalizes every
// Scope.SubType in the provided slice to trimmed lowercase canonical form.
// Persisted scopes must share the same canonical shape regardless of input casing
// so DB state matches the case-insensitive runtime matching semantics.
func TestNewLimit_NormalizesScopeSubType(t *testing.T) {
	accountID := testutil.UUIDPtr(testutil.MustDeterministicUUID(200))

	scopes := []Scope{
		{AccountID: accountID, SubType: testutil.StringPtr("SELL")},
		{AccountID: accountID, SubType: testutil.StringPtr("  buy  ")},
		{AccountID: accountID, SubType: testutil.StringPtr("Credit")},
		{AccountID: accountID, SubType: nil},
	}

	limit, err := NewLimit(
		"test limit",
		LimitTypeDaily,
		decimal.RequireFromString("1000"),
		"USD",
		scopes,
		nil,
		testutil.FixedTime(),
	)

	require.NoError(t, err)
	require.Len(t, limit.Scopes, 4)

	require.NotNil(t, limit.Scopes[0].SubType)
	require.Equal(t, "sell", *limit.Scopes[0].SubType)

	require.NotNil(t, limit.Scopes[1].SubType)
	require.Equal(t, "buy", *limit.Scopes[1].SubType)

	require.NotNil(t, limit.Scopes[2].SubType)
	require.Equal(t, "credit", *limit.Scopes[2].SubType)

	assert.Nil(t, limit.Scopes[3].SubType)
}

// TestLimit_Update_NormalizesScopeSubType verifies that Limit.Update normalizes
// every Scope.SubType in the updated slice to trimmed lowercase canonical form.
func TestLimit_Update_NormalizesScopeSubType(t *testing.T) {
	limit := newTestLimit(t)
	accountID := testutil.UUIDPtr(testutil.MustDeterministicUUID(201))

	newScopes := []Scope{
		{AccountID: accountID, SubType: testutil.StringPtr("SELL")},
		{AccountID: accountID, SubType: testutil.StringPtr("  BUY  ")},
		{AccountID: accountID, SubType: nil},
	}

	err := limit.Update(nil, nil, nil, &newScopes, nil, nil, nil, nil, testutil.FixedTime())
	require.NoError(t, err)

	require.Len(t, limit.Scopes, 3)

	require.NotNil(t, limit.Scopes[0].SubType)
	require.Equal(t, "sell", *limit.Scopes[0].SubType)

	require.NotNil(t, limit.Scopes[1].SubType)
	require.Equal(t, "buy", *limit.Scopes[1].SubType)

	assert.Nil(t, limit.Scopes[2].SubType)
}
