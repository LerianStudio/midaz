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

	"tracer/internal/testutil"
	"tracer/pkg/model"
)

// TestRulePostgreSQLModel_ToEntity tests the conversion from database model to domain entity.
// This test follows the ToEntity/FromEntity pattern from Ring Standards (golang/domain.md).
func TestRulePostgreSQLModel_ToEntity(t *testing.T) {
	t.Parallel()

	// Deterministic test data following PROJECT_RULES.md
	testID := testutil.MustDeterministicUUID(1)
	testAccountID := testutil.MustDeterministicUUID(2)
	testSegmentID := testutil.MustDeterministicUUID(3)
	testPortfolioID := testutil.MustDeterministicUUID(4)
	testMerchantID := testutil.MustDeterministicUUID(5)
	fixedTime := testutil.FixedTime()
	activatedAt := fixedTime.Add(1 * time.Hour)
	deactivatedAt := fixedTime.Add(2 * time.Hour)
	deletedAt := fixedTime.Add(3 * time.Hour)

	cardType := model.TransactionTypeCard
	subType := "debit"

	tests := []struct {
		name     string
		dbModel  RulePostgreSQLModel
		expected *model.Rule
	}{
		{
			name: "converts basic rule without optional fields",
			dbModel: RulePostgreSQLModel{
				ID:            testID.String(),
				Name:          "Test Rule",
				Description:   sql.NullString{Valid: false},
				Expression:    "amount > 1000",
				Action:        "DENY",
				Scopes:        "[]",
				Status:        "DRAFT",
				CreatedAt:     fixedTime,
				UpdatedAt:     fixedTime,
				ActivatedAt:   sql.NullTime{Valid: false},
				DeactivatedAt: sql.NullTime{Valid: false},
				DeletedAt:     sql.NullTime{Valid: false},
			},
			expected: &model.Rule{
				ID:            testID,
				Name:          "Test Rule",
				Description:   nil,
				Expression:    "amount > 1000",
				Action:        model.DecisionDeny,
				Scopes:        []model.Scope{},
				Status:        model.RuleStatusDraft,
				CreatedAt:     fixedTime,
				UpdatedAt:     fixedTime,
				ActivatedAt:   nil,
				DeactivatedAt: nil,
				DeletedAt:     nil,
			},
		},
		{
			name: "converts rule with all optional fields populated",
			dbModel: RulePostgreSQLModel{
				ID:            testID.String(),
				Name:          "Full Rule",
				Description:   sql.NullString{String: "A detailed description", Valid: true},
				Expression:    "amount > 5000",
				Action:        "REVIEW",
				Scopes:        `[{"accountId":"` + testAccountID.String() + `","segmentId":"` + testSegmentID.String() + `"}]`,
				Status:        "ACTIVE",
				CreatedAt:     fixedTime,
				UpdatedAt:     fixedTime.Add(30 * time.Minute),
				ActivatedAt:   sql.NullTime{Time: activatedAt, Valid: true},
				DeactivatedAt: sql.NullTime{Time: deactivatedAt, Valid: true},
				DeletedAt:     sql.NullTime{Valid: false},
			},
			expected: &model.Rule{
				ID:            testID,
				Name:          "Full Rule",
				Description:   testutil.StringPtr("A detailed description"),
				Expression:    "amount > 5000",
				Action:        model.DecisionReview,
				Scopes:        []model.Scope{{AccountID: &testAccountID, SegmentID: &testSegmentID}},
				Status:        model.RuleStatusActive,
				CreatedAt:     fixedTime,
				UpdatedAt:     fixedTime.Add(30 * time.Minute),
				ActivatedAt:   &activatedAt,
				DeactivatedAt: &deactivatedAt,
				DeletedAt:     nil,
			},
		},
		{
			name: "converts deleted rule with deletedAt timestamp",
			dbModel: RulePostgreSQLModel{
				ID:            testID.String(),
				Name:          "Deleted Rule",
				Description:   sql.NullString{Valid: false},
				Expression:    "true",
				Action:        "ALLOW",
				Scopes:        "[]",
				Status:        "DELETED",
				CreatedAt:     fixedTime,
				UpdatedAt:     deletedAt,
				ActivatedAt:   sql.NullTime{Valid: false},
				DeactivatedAt: sql.NullTime{Valid: false},
				DeletedAt:     sql.NullTime{Time: deletedAt, Valid: true},
			},
			expected: &model.Rule{
				ID:            testID,
				Name:          "Deleted Rule",
				Description:   nil,
				Expression:    "true",
				Action:        model.DecisionAllow,
				Scopes:        []model.Scope{},
				Status:        model.RuleStatusDeleted,
				CreatedAt:     fixedTime,
				UpdatedAt:     deletedAt,
				ActivatedAt:   nil,
				DeactivatedAt: nil,
				DeletedAt:     &deletedAt,
			},
		},
		{
			name: "converts rule with complex scopes",
			dbModel: RulePostgreSQLModel{
				ID:          testID.String(),
				Name:        "Scoped Rule",
				Description: sql.NullString{Valid: false},
				Expression:  "amount > 100",
				Action:      "DENY",
				Scopes: `[
					{"accountId":"` + testAccountID.String() + `","merchantId":"` + testMerchantID.String() + `","transactionType":"CARD","subType":"debit"},
					{"portfolioId":"` + testPortfolioID.String() + `","segmentId":"` + testSegmentID.String() + `"}
				]`,
				Status:        "INACTIVE",
				CreatedAt:     fixedTime,
				UpdatedAt:     fixedTime,
				ActivatedAt:   sql.NullTime{Valid: false},
				DeactivatedAt: sql.NullTime{Valid: false},
				DeletedAt:     sql.NullTime{Valid: false},
			},
			expected: &model.Rule{
				ID:          testID,
				Name:        "Scoped Rule",
				Description: nil,
				Expression:  "amount > 100",
				Action:      model.DecisionDeny,
				Scopes: []model.Scope{
					{AccountID: &testAccountID, MerchantID: &testMerchantID, TransactionType: &cardType, SubType: &subType},
					{PortfolioID: &testPortfolioID, SegmentID: &testSegmentID},
				},
				Status:        model.RuleStatusInactive,
				CreatedAt:     fixedTime,
				UpdatedAt:     fixedTime,
				ActivatedAt:   nil,
				DeactivatedAt: nil,
				DeletedAt:     nil,
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
			assert.Equal(t, tt.expected.Expression, result.Expression, "Expression mismatch")
			assert.Equal(t, tt.expected.Action, result.Action, "Action mismatch")
			assert.Equal(t, tt.expected.Status, result.Status, "Status mismatch")
			assert.Equal(t, tt.expected.CreatedAt, result.CreatedAt, "CreatedAt mismatch")
			assert.Equal(t, tt.expected.UpdatedAt, result.UpdatedAt, "UpdatedAt mismatch")
			assert.Equal(t, tt.expected.ActivatedAt, result.ActivatedAt, "ActivatedAt mismatch")
			assert.Equal(t, tt.expected.DeactivatedAt, result.DeactivatedAt, "DeactivatedAt mismatch")
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

// TestRulePostgreSQLModel_FromEntity tests the conversion from domain entity to database model.
// This test follows the ToEntity/FromEntity pattern from Ring Standards (golang/domain.md).
func TestRulePostgreSQLModel_FromEntity(t *testing.T) {
	t.Parallel()

	// Deterministic test data following PROJECT_RULES.md
	testID := testutil.MustDeterministicUUID(10)
	testAccountID := testutil.MustDeterministicUUID(11)
	testSegmentID := testutil.MustDeterministicUUID(12)
	fixedTime := testutil.FixedTime()
	activatedAt := fixedTime.Add(1 * time.Hour)

	tests := []struct {
		name     string
		entity   *model.Rule
		assertFn func(t *testing.T, dbModel *RulePostgreSQLModel)
	}{
		{
			name: "converts basic entity without optional fields",
			entity: &model.Rule{
				ID:            testID,
				Name:          "Simple Rule",
				Description:   nil,
				Expression:    "amount < 500",
				Action:        model.DecisionAllow,
				Scopes:        []model.Scope{},
				Status:        model.RuleStatusDraft,
				CreatedAt:     fixedTime,
				UpdatedAt:     fixedTime,
				ActivatedAt:   nil,
				DeactivatedAt: nil,
				DeletedAt:     nil,
			},
			assertFn: func(t *testing.T, dbModel *RulePostgreSQLModel) {
				t.Helper()
				assert.Equal(t, testID.String(), dbModel.ID, "ID should be string representation of UUID")
				assert.Equal(t, "Simple Rule", dbModel.Name)
				assert.False(t, dbModel.Description.Valid, "Description should be invalid for nil")
				assert.Equal(t, "amount < 500", dbModel.Expression)
				assert.Equal(t, "ALLOW", dbModel.Action)
				assert.Equal(t, "DRAFT", dbModel.Status)
				assert.Equal(t, fixedTime, dbModel.CreatedAt)
				assert.Equal(t, fixedTime, dbModel.UpdatedAt)
				assert.False(t, dbModel.ActivatedAt.Valid, "ActivatedAt should be invalid for nil")
				assert.False(t, dbModel.DeactivatedAt.Valid, "DeactivatedAt should be invalid for nil")
				assert.False(t, dbModel.DeletedAt.Valid, "DeletedAt should be invalid for nil")
				// Scopes should be serialized as JSON array (empty)
				assert.Equal(t, "[]", dbModel.Scopes, "Empty scopes should serialize to empty JSON array")
			},
		},
		{
			name: "converts entity with all optional fields populated",
			entity: &model.Rule{
				ID:            testID,
				Name:          "Full Entity",
				Description:   testutil.StringPtr("Complete rule with all fields"),
				Expression:    "amount > 10000",
				Action:        model.DecisionReview,
				Scopes:        []model.Scope{{AccountID: &testAccountID, SegmentID: &testSegmentID}},
				Status:        model.RuleStatusActive,
				CreatedAt:     fixedTime,
				UpdatedAt:     fixedTime.Add(15 * time.Minute),
				ActivatedAt:   &activatedAt,
				DeactivatedAt: nil,
				DeletedAt:     nil,
			},
			assertFn: func(t *testing.T, dbModel *RulePostgreSQLModel) {
				t.Helper()
				assert.Equal(t, testID.String(), dbModel.ID)
				assert.Equal(t, "Full Entity", dbModel.Name)
				assert.True(t, dbModel.Description.Valid, "Description should be valid when set")
				assert.Equal(t, "Complete rule with all fields", dbModel.Description.String)
				assert.Equal(t, "amount > 10000", dbModel.Expression)
				assert.Equal(t, "REVIEW", dbModel.Action)
				assert.Equal(t, "ACTIVE", dbModel.Status)
				assert.Equal(t, fixedTime, dbModel.CreatedAt)
				assert.Equal(t, fixedTime.Add(15*time.Minute), dbModel.UpdatedAt)
				assert.True(t, dbModel.ActivatedAt.Valid, "ActivatedAt should be valid when set")
				assert.Equal(t, activatedAt, dbModel.ActivatedAt.Time)
				assert.False(t, dbModel.DeactivatedAt.Valid)
				assert.False(t, dbModel.DeletedAt.Valid)
				// Scopes should contain the serialized JSON
				assert.Contains(t, dbModel.Scopes, testAccountID.String(), "Scopes JSON should contain account ID")
				assert.Contains(t, dbModel.Scopes, testSegmentID.String(), "Scopes JSON should contain segment ID")
			},
		},
		{
			name: "converts entity with nil scopes to empty array",
			entity: &model.Rule{
				ID:          testID,
				Name:        "Nil Scopes Rule",
				Description: nil,
				Expression:  "true",
				Action:      model.DecisionAllow,
				Scopes:      nil, // Explicitly nil
				Status:      model.RuleStatusDraft,
				CreatedAt:   fixedTime,
				UpdatedAt:   fixedTime,
			},
			assertFn: func(t *testing.T, dbModel *RulePostgreSQLModel) {
				t.Helper()
				// Nil scopes should be serialized as empty JSON array to avoid NULL in DB
				assert.Equal(t, "[]", dbModel.Scopes, "Nil scopes should serialize to empty JSON array")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			var dbModel RulePostgreSQLModel
			err := dbModel.FromEntity(tt.entity)
			require.NoError(t, err, "FromEntity should not return error for valid entity")

			// Assert
			tt.assertFn(t, &dbModel)
		})
	}
}

// TestRulePostgreSQLModel_RoundTrip tests that ToEntity and FromEntity are inverses.
// entity -> FromEntity -> dbModel -> ToEntity -> entity (should be equal)
func TestRulePostgreSQLModel_RoundTrip(t *testing.T) {
	t.Parallel()

	testID := testutil.MustDeterministicUUID(20)
	testAccountID := testutil.MustDeterministicUUID(21)
	fixedTime := testutil.FixedTime()
	activatedAt := fixedTime.Add(1 * time.Hour)
	cardType := model.TransactionTypeCard

	original := &model.Rule{
		ID:          testID,
		Name:        "RoundTrip Rule",
		Description: testutil.StringPtr("Testing round-trip conversion"),
		Expression:  "amount > 1000 && transactionType == 'CARD'",
		Action:      model.DecisionDeny,
		Scopes: []model.Scope{
			{AccountID: &testAccountID, TransactionType: &cardType},
		},
		Status:        model.RuleStatusActive,
		CreatedAt:     fixedTime,
		UpdatedAt:     fixedTime.Add(30 * time.Minute),
		ActivatedAt:   &activatedAt,
		DeactivatedAt: nil,
		DeletedAt:     nil,
	}

	// entity -> dbModel
	var dbModel RulePostgreSQLModel
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
	assert.Equal(t, original.Expression, result.Expression, "Round-trip Expression mismatch")
	assert.Equal(t, original.Action, result.Action, "Round-trip Action mismatch")
	assert.Equal(t, original.Status, result.Status, "Round-trip Status mismatch")
	assert.Equal(t, original.CreatedAt, result.CreatedAt, "Round-trip CreatedAt mismatch")
	assert.Equal(t, original.UpdatedAt, result.UpdatedAt, "Round-trip UpdatedAt mismatch")
	assert.Equal(t, original.ActivatedAt, result.ActivatedAt, "Round-trip ActivatedAt mismatch")
	assert.Equal(t, original.DeactivatedAt, result.DeactivatedAt, "Round-trip DeactivatedAt mismatch")
	assert.Equal(t, original.DeletedAt, result.DeletedAt, "Round-trip DeletedAt mismatch")

	// Scopes comparison
	require.Len(t, result.Scopes, len(original.Scopes), "Round-trip Scopes length mismatch")
	for i := range original.Scopes {
		assert.Equal(t, original.Scopes[i].AccountID, result.Scopes[i].AccountID, "Round-trip Scope[%d] AccountID mismatch", i)
		assert.Equal(t, original.Scopes[i].TransactionType, result.Scopes[i].TransactionType, "Round-trip Scope[%d] TransactionType mismatch", i)
	}
}

// TestRulePostgreSQLModel_ToEntity_EdgeCases tests edge cases for ToEntity conversion.
func TestRulePostgreSQLModel_ToEntity_EdgeCases(t *testing.T) {
	t.Parallel()

	fixedTime := testutil.FixedTime()

	tests := []struct {
		name     string
		dbModel  RulePostgreSQLModel
		validate func(t *testing.T, result *model.Rule, err error)
	}{
		{
			name: "handles empty scopes JSON",
			dbModel: RulePostgreSQLModel{
				ID:         testutil.MustDeterministicUUID(30).String(),
				Name:       "Empty Scopes",
				Expression: "true",
				Action:     "ALLOW",
				Scopes:     "[]",
				Status:     "DRAFT",
				CreatedAt:  fixedTime,
				UpdatedAt:  fixedTime,
			},
			validate: func(t *testing.T, result *model.Rule, err error) {
				t.Helper()
				require.NoError(t, err, "should not error on empty scopes JSON")
				require.NotNil(t, result)
				assert.Empty(t, result.Scopes, "scopes should be empty slice, not nil")
				assert.NotNil(t, result.Scopes, "scopes should never be nil")
			},
		},
		{
			name: "handles whitespace in description",
			dbModel: RulePostgreSQLModel{
				ID:          testutil.MustDeterministicUUID(31).String(),
				Name:        "Whitespace Description",
				Description: sql.NullString{String: "  Description with spaces  ", Valid: true},
				Expression:  "true",
				Action:      "ALLOW",
				Scopes:      "[]",
				Status:      "DRAFT",
				CreatedAt:   fixedTime,
				UpdatedAt:   fixedTime,
			},
			validate: func(t *testing.T, result *model.Rule, err error) {
				t.Helper()
				require.NoError(t, err)
				require.NotNil(t, result)
				require.NotNil(t, result.Description)
				// ToEntity should preserve the value as-is (normalization happens at input layer)
				assert.Equal(t, "  Description with spaces  ", *result.Description)
			},
		},
		{
			name: "handles ALLOW decision type",
			dbModel: RulePostgreSQLModel{
				ID:         testutil.MustDeterministicUUID(32).String(),
				Name:       "Allow Rule",
				Expression: "true",
				Action:     "ALLOW",
				Scopes:     "[]",
				Status:     "ACTIVE",
				CreatedAt:  fixedTime,
				UpdatedAt:  fixedTime,
			},
			validate: func(t *testing.T, result *model.Rule, err error) {
				t.Helper()
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, model.DecisionAllow, result.Action)
			},
		},
		{
			name: "handles DENY decision type",
			dbModel: RulePostgreSQLModel{
				ID:         testutil.MustDeterministicUUID(35).String(),
				Name:       "Deny Rule",
				Expression: "true",
				Action:     "DENY",
				Scopes:     "[]",
				Status:     "ACTIVE",
				CreatedAt:  fixedTime,
				UpdatedAt:  fixedTime,
			},
			validate: func(t *testing.T, result *model.Rule, err error) {
				t.Helper()
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, model.DecisionDeny, result.Action)
			},
		},
		{
			name: "handles REVIEW decision type",
			dbModel: RulePostgreSQLModel{
				ID:         testutil.MustDeterministicUUID(36).String(),
				Name:       "Review Rule",
				Expression: "true",
				Action:     "REVIEW",
				Scopes:     "[]",
				Status:     "ACTIVE",
				CreatedAt:  fixedTime,
				UpdatedAt:  fixedTime,
			},
			validate: func(t *testing.T, result *model.Rule, err error) {
				t.Helper()
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, model.DecisionReview, result.Action)
			},
		},
		{
			name: "returns error for invalid scopes JSON",
			dbModel: RulePostgreSQLModel{
				ID:         testutil.MustDeterministicUUID(33).String(),
				Name:       "Invalid Scopes",
				Expression: "true",
				Action:     "ALLOW",
				Scopes:     "not-valid-json",
				Status:     "DRAFT",
				CreatedAt:  fixedTime,
				UpdatedAt:  fixedTime,
			},
			validate: func(t *testing.T, result *model.Rule, err error) {
				t.Helper()
				require.Error(t, err, "should return error for invalid JSON")
				require.Nil(t, result, "result should be nil on error")
				assert.Contains(t, err.Error(), "failed to unmarshal scopes")
			},
		},
		{
			name: "returns error for invalid UUID in ID",
			dbModel: RulePostgreSQLModel{
				ID:         "not-a-valid-uuid",
				Name:       "Invalid ID",
				Expression: "true",
				Action:     "ALLOW",
				Scopes:     "[]",
				Status:     "DRAFT",
				CreatedAt:  fixedTime,
				UpdatedAt:  fixedTime,
			},
			validate: func(t *testing.T, result *model.Rule, err error) {
				t.Helper()
				require.Error(t, err, "should return error for invalid UUID")
				require.Nil(t, result, "result should be nil on error")
				assert.Contains(t, err.Error(), "invalid Rule ID")
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

func TestRulePostgreSQLModel_FromEntity_NilEntity(t *testing.T) {
	t.Parallel()

	var dbModel RulePostgreSQLModel

	err := dbModel.FromEntity(nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot be nil")
}
