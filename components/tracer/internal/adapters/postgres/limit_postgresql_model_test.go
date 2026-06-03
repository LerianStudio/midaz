// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopspring/decimal"

	"tracer/internal/testutil"
	"tracer/pkg/model"
)

// TestLimitPostgreSQLModel_ToEntity tests the conversion from database model to domain entity.
// This test follows the ToEntity/FromEntity pattern from Ring Standards (golang/domain.md).
func TestLimitPostgreSQLModel_ToEntity(t *testing.T) {
	t.Parallel()

	// Deterministic test data following PROJECT_RULES.md
	testID := testutil.MustDeterministicUUID(1)
	testAccountID := testutil.MustDeterministicUUID(2)
	testSegmentID := testutil.MustDeterministicUUID(3)
	testPortfolioID := testutil.MustDeterministicUUID(4)
	testMerchantID := testutil.MustDeterministicUUID(5)
	fixedTime := testutil.FixedTime()
	resetAt := fixedTime.Add(24 * time.Hour)
	deletedAt := fixedTime.Add(48 * time.Hour)

	cardType := model.TransactionTypeCard
	subType := "debit"

	tests := []struct {
		name     string
		dbModel  LimitPostgreSQLModel
		expected *model.Limit
	}{
		{
			name: "converts basic limit without optional fields",
			dbModel: LimitPostgreSQLModel{
				ID:          testID.String(),
				Name:        "Test Limit",
				Description: sql.NullString{Valid: false},
				LimitType:   "DAILY",
				MaxAmount:   decimal.RequireFromString("1000"),
				Currency:    "BRL",
				Scopes:      "[]",
				Status:      "DRAFT",
				ResetAt:     sql.NullTime{Valid: false},
				CreatedAt:   fixedTime,
				UpdatedAt:   fixedTime,
				DeletedAt:   sql.NullTime{Valid: false},
			},
			expected: &model.Limit{
				ID:          testID,
				Name:        "Test Limit",
				Description: nil,
				LimitType:   model.LimitTypeDaily,
				MaxAmount:   decimal.RequireFromString("1000"),
				Currency:    "BRL",
				Scopes:      []model.Scope{},
				Status:      model.LimitStatusDraft,
				ResetAt:     nil,
				CreatedAt:   fixedTime,
				UpdatedAt:   fixedTime,
				DeletedAt:   nil,
			},
		},
		{
			name: "converts limit with all optional fields populated",
			dbModel: LimitPostgreSQLModel{
				ID:          testID.String(),
				Name:        "Full Limit",
				Description: sql.NullString{String: "A detailed description", Valid: true},
				LimitType:   "MONTHLY",
				MaxAmount:   decimal.RequireFromString("5000"),
				Currency:    "USD",
				Scopes:      `[{"accountId":"` + testAccountID.String() + `","segmentId":"` + testSegmentID.String() + `"}]`,
				Status:      "ACTIVE",
				ResetAt:     sql.NullTime{Time: resetAt, Valid: true},
				CreatedAt:   fixedTime,
				UpdatedAt:   fixedTime.Add(30 * time.Minute),
				DeletedAt:   sql.NullTime{Valid: false},
			},
			expected: &model.Limit{
				ID:          testID,
				Name:        "Full Limit",
				Description: testutil.StringPtr("A detailed description"),
				LimitType:   model.LimitTypeMonthly,
				MaxAmount:   decimal.RequireFromString("5000"),
				Currency:    "USD",
				Scopes:      []model.Scope{{AccountID: &testAccountID, SegmentID: &testSegmentID}},
				Status:      model.LimitStatusActive,
				ResetAt:     &resetAt,
				CreatedAt:   fixedTime,
				UpdatedAt:   fixedTime.Add(30 * time.Minute),
				DeletedAt:   nil,
			},
		},
		{
			name: "converts deleted limit with deletedAt timestamp",
			dbModel: LimitPostgreSQLModel{
				ID:          testID.String(),
				Name:        "Deleted Limit",
				Description: sql.NullString{Valid: false},
				LimitType:   "PER_TRANSACTION",
				MaxAmount:   decimal.RequireFromString("100"),
				Currency:    "EUR",
				Scopes:      "[]",
				Status:      "DELETED",
				ResetAt:     sql.NullTime{Valid: false},
				CreatedAt:   fixedTime,
				UpdatedAt:   deletedAt,
				DeletedAt:   sql.NullTime{Time: deletedAt, Valid: true},
			},
			expected: &model.Limit{
				ID:          testID,
				Name:        "Deleted Limit",
				Description: nil,
				LimitType:   model.LimitTypePerTransaction,
				MaxAmount:   decimal.RequireFromString("100"),
				Currency:    "EUR",
				Scopes:      []model.Scope{},
				Status:      model.LimitStatusDeleted,
				ResetAt:     nil,
				CreatedAt:   fixedTime,
				UpdatedAt:   deletedAt,
				DeletedAt:   &deletedAt,
			},
		},
		{
			name: "converts limit with complex scopes",
			dbModel: LimitPostgreSQLModel{
				ID:          testID.String(),
				Name:        "Scoped Limit",
				Description: sql.NullString{Valid: false},
				LimitType:   "DAILY",
				MaxAmount:   decimal.RequireFromString("500"),
				Currency:    "BRL",
				Scopes: `[
					{"accountId":"` + testAccountID.String() + `","merchantId":"` + testMerchantID.String() + `","transactionType":"CARD","subType":"debit"},
					{"portfolioId":"` + testPortfolioID.String() + `","segmentId":"` + testSegmentID.String() + `"}
				]`,
				Status:    "INACTIVE",
				ResetAt:   sql.NullTime{Time: resetAt, Valid: true},
				CreatedAt: fixedTime,
				UpdatedAt: fixedTime,
				DeletedAt: sql.NullTime{Valid: false},
			},
			expected: &model.Limit{
				ID:          testID,
				Name:        "Scoped Limit",
				Description: nil,
				LimitType:   model.LimitTypeDaily,
				MaxAmount:   decimal.RequireFromString("500"),
				Currency:    "BRL",
				Scopes: []model.Scope{
					{AccountID: &testAccountID, MerchantID: &testMerchantID, TransactionType: &cardType, SubType: &subType},
					{PortfolioID: &testPortfolioID, SegmentID: &testSegmentID},
				},
				Status:    model.LimitStatusInactive,
				ResetAt:   &resetAt,
				CreatedAt: fixedTime,
				UpdatedAt: fixedTime,
				DeletedAt: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			result, err := tt.dbModel.ToEntity()

			// Assert
			require.NoError(t, err, "ToEntity should not return error")
			require.NotNil(t, result, "ToEntity should not return nil")
			assert.Equal(t, tt.expected.ID, result.ID, "ID mismatch")
			assert.Equal(t, tt.expected.Name, result.Name, "Name mismatch")
			assert.Equal(t, tt.expected.Description, result.Description, "Description mismatch")
			assert.Equal(t, tt.expected.LimitType, result.LimitType, "LimitType mismatch")
			assert.Equal(t, tt.expected.MaxAmount, result.MaxAmount, "MaxAmount mismatch")
			assert.Equal(t, tt.expected.Currency, result.Currency, "Currency mismatch")
			assert.Equal(t, tt.expected.Status, result.Status, "Status mismatch")
			assert.Equal(t, tt.expected.ResetAt, result.ResetAt, "ResetAt mismatch")
			assert.Equal(t, tt.expected.CreatedAt, result.CreatedAt, "CreatedAt mismatch")
			assert.Equal(t, tt.expected.UpdatedAt, result.UpdatedAt, "UpdatedAt mismatch")
			assert.Equal(t, tt.expected.DeletedAt, result.DeletedAt, "DeletedAt mismatch")

			// Validate scopes separately for better error messages
			require.Len(t, result.Scopes, len(tt.expected.Scopes), "Scopes length mismatch")
			for i, expectedScope := range tt.expected.Scopes {
				assert.Equal(t, expectedScope.AccountID, result.Scopes[i].AccountID, "Scope[%d] AccountID mismatch", i)
				assert.Equal(t, expectedScope.SegmentID, result.Scopes[i].SegmentID, "Scope[%d] SegmentID mismatch", i)
				assert.Equal(t, expectedScope.PortfolioID, result.Scopes[i].PortfolioID, "Scope[%d] PortfolioID mismatch", i)
				assert.Equal(t, expectedScope.MerchantID, result.Scopes[i].MerchantID, "Scope[%d] MerchantID mismatch", i)
				assert.Equal(t, expectedScope.TransactionType, result.Scopes[i].TransactionType, "Scope[%d] TransactionType mismatch", i)
				assert.Equal(t, expectedScope.SubType, result.Scopes[i].SubType, "Scope[%d] SubType mismatch", i)
			}
		})
	}
}

// TestLimitPostgreSQLModel_FromEntity tests the conversion from domain entity to database model.
// This test follows the ToEntity/FromEntity pattern from Ring Standards (golang/domain.md).
func TestLimitPostgreSQLModel_FromEntity(t *testing.T) {
	t.Parallel()

	// Deterministic test data following PROJECT_RULES.md
	testID := testutil.MustDeterministicUUID(10)
	testAccountID := testutil.MustDeterministicUUID(11)
	testSegmentID := testutil.MustDeterministicUUID(12)
	fixedTime := testutil.FixedTime()
	resetAt := fixedTime.Add(24 * time.Hour)

	tests := []struct {
		name     string
		entity   *model.Limit
		assertFn func(t *testing.T, dbModel *LimitPostgreSQLModel)
	}{
		{
			name: "converts basic entity without optional fields",
			entity: &model.Limit{
				ID:          testID,
				Name:        "Simple Limit",
				Description: nil,
				LimitType:   model.LimitTypeDaily,
				MaxAmount:   decimal.RequireFromString("1000"),
				Currency:    "BRL",
				Scopes:      []model.Scope{},
				Status:      model.LimitStatusDraft,
				ResetAt:     nil,
				CreatedAt:   fixedTime,
				UpdatedAt:   fixedTime,
				DeletedAt:   nil,
			},
			assertFn: func(t *testing.T, dbModel *LimitPostgreSQLModel) {
				t.Helper()
				assert.Equal(t, testID.String(), dbModel.ID, "ID should be string representation of UUID")
				assert.Equal(t, "Simple Limit", dbModel.Name)
				assert.False(t, dbModel.Description.Valid, "Description should be invalid for nil")
				assert.Equal(t, "DAILY", dbModel.LimitType)
				assert.True(t, decimal.RequireFromString("1000").Equal(dbModel.MaxAmount), "MaxAmount should be 1000")
				assert.Equal(t, "BRL", dbModel.Currency)
				assert.Equal(t, "DRAFT", dbModel.Status)
				assert.Equal(t, fixedTime, dbModel.CreatedAt)
				assert.Equal(t, fixedTime, dbModel.UpdatedAt)
				assert.False(t, dbModel.ResetAt.Valid, "ResetAt should be invalid for nil")
				assert.False(t, dbModel.DeletedAt.Valid, "DeletedAt should be invalid for nil")
				// Scopes should be serialized as JSON array (empty)
				assert.Equal(t, "[]", dbModel.Scopes, "Empty scopes should serialize to empty JSON array")
			},
		},
		{
			name: "converts entity with all optional fields populated",
			entity: &model.Limit{
				ID:          testID,
				Name:        "Full Entity",
				Description: testutil.StringPtr("Complete limit with all fields"),
				LimitType:   model.LimitTypeMonthly,
				MaxAmount:   decimal.RequireFromString("5000"),
				Currency:    "USD",
				Scopes:      []model.Scope{{AccountID: &testAccountID, SegmentID: &testSegmentID}},
				Status:      model.LimitStatusActive,
				ResetAt:     &resetAt,
				CreatedAt:   fixedTime,
				UpdatedAt:   fixedTime.Add(15 * time.Minute),
				DeletedAt:   nil,
			},
			assertFn: func(t *testing.T, dbModel *LimitPostgreSQLModel) {
				t.Helper()
				assert.Equal(t, testID.String(), dbModel.ID)
				assert.Equal(t, "Full Entity", dbModel.Name)
				assert.True(t, dbModel.Description.Valid, "Description should be valid when set")
				assert.Equal(t, "Complete limit with all fields", dbModel.Description.String)
				assert.Equal(t, "MONTHLY", dbModel.LimitType)
				assert.True(t, decimal.RequireFromString("5000").Equal(dbModel.MaxAmount), "MaxAmount should be 5000")
				assert.Equal(t, "USD", dbModel.Currency)
				assert.Equal(t, "ACTIVE", dbModel.Status)
				assert.Equal(t, fixedTime, dbModel.CreatedAt)
				assert.Equal(t, fixedTime.Add(15*time.Minute), dbModel.UpdatedAt)
				assert.True(t, dbModel.ResetAt.Valid, "ResetAt should be valid when set")
				assert.Equal(t, resetAt, dbModel.ResetAt.Time)
				assert.False(t, dbModel.DeletedAt.Valid)
				// Scopes should contain the serialized JSON
				assert.Contains(t, dbModel.Scopes, testAccountID.String(), "Scopes JSON should contain account ID")
				assert.Contains(t, dbModel.Scopes, testSegmentID.String(), "Scopes JSON should contain segment ID")
			},
		},
		{
			name: "converts entity with nil scopes to empty array",
			entity: &model.Limit{
				ID:          testID,
				Name:        "Nil Scopes Limit",
				Description: nil,
				LimitType:   model.LimitTypePerTransaction,
				MaxAmount:   decimal.RequireFromString("100"),
				Currency:    "EUR",
				Scopes:      nil, // Explicitly nil
				Status:      model.LimitStatusDraft,
				ResetAt:     nil,
				CreatedAt:   fixedTime,
				UpdatedAt:   fixedTime,
			},
			assertFn: func(t *testing.T, dbModel *LimitPostgreSQLModel) {
				t.Helper()
				// Nil scopes should be serialized as empty JSON array to avoid NULL in DB
				assert.Equal(t, "[]", dbModel.Scopes, "Nil scopes should serialize to empty JSON array")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			var dbModel LimitPostgreSQLModel
			err := dbModel.FromEntity(tt.entity)
			require.NoError(t, err, "FromEntity should not return error for valid entity")

			// Assert
			tt.assertFn(t, &dbModel)
		})
	}
}

// TestLimitPostgreSQLModel_RoundTrip tests that ToEntity and FromEntity are inverses.
// entity -> FromEntity -> dbModel -> ToEntity -> entity (should be equal)
func TestLimitPostgreSQLModel_RoundTrip(t *testing.T) {
	t.Parallel()

	testID := testutil.MustDeterministicUUID(20)
	testAccountID := testutil.MustDeterministicUUID(21)
	fixedTime := testutil.FixedTime()
	resetAt := fixedTime.Add(24 * time.Hour)
	cardType := model.TransactionTypeCard

	original := &model.Limit{
		ID:          testID,
		Name:        "RoundTrip Limit",
		Description: testutil.StringPtr("Testing round-trip conversion"),
		LimitType:   model.LimitTypeDaily,
		MaxAmount:   decimal.RequireFromString("1000"),
		Currency:    "BRL",
		Scopes: []model.Scope{
			{AccountID: &testAccountID, TransactionType: &cardType},
		},
		Status:    model.LimitStatusActive,
		ResetAt:   &resetAt,
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime.Add(30 * time.Minute),
		DeletedAt: nil,
	}

	// entity -> dbModel
	var dbModel LimitPostgreSQLModel
	err := dbModel.FromEntity(original)
	require.NoError(t, err, "FromEntity should not return error")

	// dbModel -> entity
	result, err := dbModel.ToEntity()

	// Assert equality
	require.NoError(t, err, "ToEntity should not return error")
	require.NotNil(t, result, "ToEntity should not return nil")
	assert.Equal(t, original.ID, result.ID, "Round-trip ID mismatch")
	assert.Equal(t, original.Name, result.Name, "Round-trip Name mismatch")
	assert.Equal(t, original.Description, result.Description, "Round-trip Description mismatch")
	assert.Equal(t, original.LimitType, result.LimitType, "Round-trip LimitType mismatch")
	assert.Equal(t, original.MaxAmount, result.MaxAmount, "Round-trip MaxAmount mismatch")
	assert.Equal(t, original.Currency, result.Currency, "Round-trip Currency mismatch")
	assert.Equal(t, original.Status, result.Status, "Round-trip Status mismatch")
	assert.Equal(t, original.ResetAt, result.ResetAt, "Round-trip ResetAt mismatch")
	assert.Equal(t, original.CreatedAt, result.CreatedAt, "Round-trip CreatedAt mismatch")
	assert.Equal(t, original.UpdatedAt, result.UpdatedAt, "Round-trip UpdatedAt mismatch")
	assert.Equal(t, original.DeletedAt, result.DeletedAt, "Round-trip DeletedAt mismatch")

	// Scopes comparison
	require.Len(t, result.Scopes, len(original.Scopes), "Round-trip Scopes length mismatch")
	for i := range original.Scopes {
		assert.Equal(t, original.Scopes[i].AccountID, result.Scopes[i].AccountID, "Round-trip Scope[%d] AccountID mismatch", i)
		assert.Equal(t, original.Scopes[i].TransactionType, result.Scopes[i].TransactionType, "Round-trip Scope[%d] TransactionType mismatch", i)
	}
}

// TestLimitPostgreSQLModel_ToEntity_EdgeCases tests edge cases for ToEntity conversion.
func TestLimitPostgreSQLModel_ToEntity_EdgeCases(t *testing.T) {
	t.Parallel()

	fixedTime := testutil.FixedTime()

	tests := []struct {
		name     string
		dbModel  LimitPostgreSQLModel
		validate func(t *testing.T, result *model.Limit, err error)
	}{
		{
			name: "handles empty scopes JSON",
			dbModel: LimitPostgreSQLModel{
				ID:        testutil.MustDeterministicUUID(30).String(),
				Name:      "Empty Scopes",
				LimitType: "DAILY",
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "BRL",
				Scopes:    "[]",
				Status:    "DRAFT",
				CreatedAt: fixedTime,
				UpdatedAt: fixedTime,
			},
			validate: func(t *testing.T, result *model.Limit, err error) {
				t.Helper()
				require.NoError(t, err, "should not error on empty scopes JSON")
				require.NotNil(t, result)
				assert.Empty(t, result.Scopes, "scopes should be empty slice, not nil")
				assert.NotNil(t, result.Scopes, "scopes should never be nil")
			},
		},
		{
			name: "handles whitespace in description",
			dbModel: LimitPostgreSQLModel{
				ID:          testutil.MustDeterministicUUID(31).String(),
				Name:        "Whitespace Description",
				Description: sql.NullString{String: "  Description with spaces  ", Valid: true},
				LimitType:   "MONTHLY",
				MaxAmount:   decimal.RequireFromString("2000"),
				Currency:    "USD",
				Scopes:      "[]",
				Status:      "DRAFT",
				CreatedAt:   fixedTime,
				UpdatedAt:   fixedTime,
			},
			validate: func(t *testing.T, result *model.Limit, err error) {
				t.Helper()
				require.NoError(t, err)
				require.NotNil(t, result)
				require.NotNil(t, result.Description)
				// ToEntity should preserve the value as-is (normalization happens at input layer)
				assert.Equal(t, "  Description with spaces  ", *result.Description)
			},
		},
		{
			name: "handles all limit types",
			dbModel: LimitPostgreSQLModel{
				ID:        testutil.MustDeterministicUUID(32).String(),
				Name:      "Per Transaction Limit",
				LimitType: "PER_TRANSACTION",
				MaxAmount: decimal.RequireFromString("50"),
				Currency:  "EUR",
				Scopes:    "[]",
				Status:    "ACTIVE",
				CreatedAt: fixedTime,
				UpdatedAt: fixedTime,
			},
			validate: func(t *testing.T, result *model.Limit, err error) {
				t.Helper()
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, model.LimitTypePerTransaction, result.LimitType)
			},
		},
		{
			name: "handles all status types",
			dbModel: LimitPostgreSQLModel{
				ID:        testutil.MustDeterministicUUID(33).String(),
				Name:      "Inactive Limit",
				LimitType: "DAILY",
				MaxAmount: decimal.RequireFromString("1000"),
				Currency:  "BRL",
				Scopes:    "[]",
				Status:    "INACTIVE",
				CreatedAt: fixedTime,
				UpdatedAt: fixedTime,
			},
			validate: func(t *testing.T, result *model.Limit, err error) {
				t.Helper()
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, model.LimitStatusInactive, result.Status)
			},
		},
		{
			name: "returns error for invalid scopes JSON",
			dbModel: LimitPostgreSQLModel{
				ID:        testutil.MustDeterministicUUID(34).String(),
				Name:      "Invalid Scopes",
				LimitType: "DAILY",
				MaxAmount: decimal.RequireFromString("100"),
				Currency:  "BRL",
				Scopes:    "not-valid-json",
				Status:    "DRAFT",
				CreatedAt: fixedTime,
				UpdatedAt: fixedTime,
			},
			validate: func(t *testing.T, result *model.Limit, err error) {
				t.Helper()
				require.Error(t, err, "should return error for invalid JSON")
				require.Nil(t, result, "result should be nil on error")
				assert.Contains(t, err.Error(), "failed to unmarshal scopes")
			},
		},
		{
			name: "returns error for invalid UUID in ID",
			dbModel: LimitPostgreSQLModel{
				ID:        "not-a-valid-uuid",
				Name:      "Invalid ID",
				LimitType: "DAILY",
				MaxAmount: decimal.RequireFromString("100"),
				Currency:  "BRL",
				Scopes:    "[]",
				Status:    "DRAFT",
				CreatedAt: fixedTime,
				UpdatedAt: fixedTime,
			},
			validate: func(t *testing.T, result *model.Limit, err error) {
				t.Helper()
				require.Error(t, err, "should return error for invalid UUID")
				require.Nil(t, result, "result should be nil on error")
				assert.Contains(t, err.Error(), "invalid Limit ID")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.dbModel.ToEntity()
			tt.validate(t, result, err)
		})
	}
}

func TestLimitPostgreSQLModel_FromEntity_NilEntity(t *testing.T) {
	t.Parallel()

	var dbModel LimitPostgreSQLModel

	err := dbModel.FromEntity(nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot be nil")
}
