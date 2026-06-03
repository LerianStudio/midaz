// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopspring/decimal"

	"tracer/internal/testutil"
	"tracer/pkg/model"
)

// TestTransactionValidationPostgreSQLModel_ToEntity tests the conversion from database model to domain entity.
// This test follows the ToEntity/FromEntity pattern from Ring Standards (golang/domain.md).
func TestTransactionValidationPostgreSQLModel_ToEntity(t *testing.T) {
	t.Parallel()

	// Deterministic test data following PROJECT_RULES.md
	testID := testutil.MustDeterministicUUID(1)
	testRequestID := testutil.MustDeterministicUUID(2)
	testAccountID := testutil.MustDeterministicUUID(3)
	testSegmentID := testutil.MustDeterministicUUID(4)
	testPortfolioID := testutil.MustDeterministicUUID(5)
	testMerchantID := testutil.MustDeterministicUUID(6)
	testMatchedRuleID := testutil.MustDeterministicUUID(7)
	testEvaluatedRuleID := testutil.MustDeterministicUUID(8)
	testLimitID := testutil.MustDeterministicUUID(9)
	fixedTime := testutil.FixedTime()
	txTimestamp := fixedTime.Add(-1 * time.Hour)

	subType := "debit"

	tests := []struct {
		name     string
		dbModel  TransactionValidationPostgreSQLModel
		expected *model.TransactionValidation
	}{
		{
			name: "converts basic validation without optional fields",
			dbModel: TransactionValidationPostgreSQLModel{
				ID:                   testID.String(),
				RequestID:            testRequestID.String(),
				TransactionType:      "CARD",
				SubType:              nil,
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: txTimestamp,
				Account:              `{"accountId":"` + testAccountID.String() + `","type":"checking","status":"active"}`,
				Segment:              nil,
				Portfolio:            nil,
				Merchant:             nil,
				Metadata:             "{}",
				Decision:             "ALLOW",
				Reason:               "No matching rules found",
				MatchedRuleIds:       "{}",
				EvaluatedRuleIds:     "{}",
				LimitUsageDetails:    "[]",
				ProcessingTimeMs:     15,
				CreatedAt:            fixedTime,
			},
			expected: &model.TransactionValidation{
				ID:                   testID,
				RequestID:            testRequestID,
				TransactionType:      model.TransactionTypeCard,
				SubType:              nil,
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: txTimestamp,
				Account: model.AccountContext{
					ID:     testAccountID,
					Type:   "checking",
					Status: "active",
				},
				Segment:   nil,
				Portfolio: nil,
				Merchant:  nil,
				Metadata:  nil,
				EvaluationResult: model.EvaluationResult{
					Decision:         model.DecisionAllow,
					Reason:           "No matching rules found",
					MatchedRuleIDs:   []uuid.UUID{},
					EvaluatedRuleIDs: []uuid.UUID{},
				},
				LimitUsageDetails: []model.LimitUsageDetail{},
				ProcessingTimeMs:  15,
				CreatedAt:         fixedTime,
			},
		},
		{
			name: "converts validation with all optional fields populated",
			dbModel: TransactionValidationPostgreSQLModel{
				ID:                   testID.String(),
				RequestID:            testRequestID.String(),
				TransactionType:      "PIX",
				SubType:              &subType,
				Amount:               decimal.RequireFromString("500"),
				Currency:             "USD",
				TransactionTimestamp: txTimestamp,
				Account:              `{"accountId":"` + testAccountID.String() + `","type":"savings","status":"active","metadata":{"tier":"gold"}}`,
				Segment:              testutil.StringPtr(`{"segmentId":"` + testSegmentID.String() + `","name":"Premium"}`),
				Portfolio:            testutil.StringPtr(`{"portfolioId":"` + testPortfolioID.String() + `","name":"Corporate"}`),
				Merchant:             testutil.StringPtr(`{"merchantId":"` + testMerchantID.String() + `","name":"Store","category":"5411","country":"BR"}`),
				Metadata:             `{"key1":"value1","key2":123}`,
				Decision:             "DENY",
				Reason:               "Rule matched: high_amount",
				MatchedRuleIds:       "{" + testMatchedRuleID.String() + "}",
				EvaluatedRuleIds:     "{" + testEvaluatedRuleID.String() + "," + testMatchedRuleID.String() + "}",
				LimitUsageDetails:    `[{"limitId":"` + testLimitID.String() + `","limitAmount":1000,"scope":"account:` + testAccountID.String() + `","period":"DAILY","currentUsage":500,"attemptedAmount":500,"exceeded":false}]`,
				ProcessingTimeMs:     25,
				CreatedAt:            fixedTime,
			},
			expected: &model.TransactionValidation{
				ID:                   testID,
				RequestID:            testRequestID,
				TransactionType:      model.TransactionTypePix,
				SubType:              &subType,
				Amount:               decimal.RequireFromString("500"),
				Currency:             "USD",
				TransactionTimestamp: txTimestamp,
				Account: model.AccountContext{
					ID:       testAccountID,
					Type:     "savings",
					Status:   "active",
					Metadata: map[string]any{"tier": "gold"},
				},
				Segment: &model.SegmentContext{
					ID:   testSegmentID,
					Name: "Premium",
				},
				Portfolio: &model.PortfolioContext{
					ID:   testPortfolioID,
					Name: "Corporate",
				},
				Merchant: &model.MerchantContext{
					ID:       testMerchantID,
					Name:     "Store",
					Category: "5411",
					Country:  "BR",
				},
				Metadata: map[string]any{"key1": "value1", "key2": float64(123)}, // JSON numbers are float64
				EvaluationResult: model.EvaluationResult{
					Decision:         model.DecisionDeny,
					Reason:           "Rule matched: high_amount",
					MatchedRuleIDs:   []uuid.UUID{testMatchedRuleID},
					EvaluatedRuleIDs: []uuid.UUID{testEvaluatedRuleID, testMatchedRuleID},
				},
				LimitUsageDetails: []model.LimitUsageDetail{
					{
						LimitID:         testLimitID,
						LimitAmount:     decimal.RequireFromString("1000"),
						Scope:           "account:" + testAccountID.String(),
						Period:          model.LimitTypeDaily,
						CurrentUsage:    decimal.RequireFromString("500"),
						AttemptedAmount: decimal.RequireFromString("500"),
						Exceeded:        false,
					},
				},
				ProcessingTimeMs: 25,
				CreatedAt:        fixedTime,
			},
		},
		{
			name: "converts validation with REVIEW decision",
			dbModel: TransactionValidationPostgreSQLModel{
				ID:                   testID.String(),
				RequestID:            testRequestID.String(),
				TransactionType:      "WIRE",
				SubType:              nil,
				Amount:               decimal.RequireFromString("10000"),
				Currency:             "EUR",
				TransactionTimestamp: txTimestamp,
				Account:              `{"accountId":"` + testAccountID.String() + `","type":"credit","status":"active"}`,
				Segment:              nil,
				Portfolio:            nil,
				Merchant:             nil,
				Metadata:             "{}",
				Decision:             "REVIEW",
				Reason:               "Large transaction requires manual review",
				MatchedRuleIds:       "{}",
				EvaluatedRuleIds:     "{}",
				LimitUsageDetails:    "[]",
				ProcessingTimeMs:     10,
				CreatedAt:            fixedTime,
			},
			expected: &model.TransactionValidation{
				ID:                   testID,
				RequestID:            testRequestID,
				TransactionType:      model.TransactionTypeWire,
				SubType:              nil,
				Amount:               decimal.RequireFromString("10000"),
				Currency:             "EUR",
				TransactionTimestamp: txTimestamp,
				Account: model.AccountContext{
					ID:     testAccountID,
					Type:   "credit",
					Status: "active",
				},
				EvaluationResult: model.EvaluationResult{
					Decision:         model.DecisionReview,
					Reason:           "Large transaction requires manual review",
					MatchedRuleIDs:   []uuid.UUID{},
					EvaluatedRuleIDs: []uuid.UUID{},
				},
				LimitUsageDetails: []model.LimitUsageDetail{},
				ProcessingTimeMs:  10,
				CreatedAt:         fixedTime,
			},
		},
		{
			name: "converts validation with multiple matched rules and exceeded limits",
			dbModel: TransactionValidationPostgreSQLModel{
				ID:                   testID.String(),
				RequestID:            testRequestID.String(),
				TransactionType:      "CARD",
				SubType:              &subType,
				Amount:               decimal.RequireFromString("1500"),
				Currency:             "BRL",
				TransactionTimestamp: txTimestamp,
				Account:              `{"accountId":"` + testAccountID.String() + `","type":"checking","status":"active"}`,
				Segment:              testutil.StringPtr(`{"segmentId":"` + testSegmentID.String() + `","name":"VIP"}`),
				Portfolio:            nil,
				Merchant:             nil,
				Metadata:             "{}",
				Decision:             "DENY",
				Reason:               "Limit exceeded: daily_limit",
				MatchedRuleIds:       "{" + testMatchedRuleID.String() + "," + testEvaluatedRuleID.String() + "}",
				EvaluatedRuleIds:     "{" + testMatchedRuleID.String() + "," + testEvaluatedRuleID.String() + "}",
				LimitUsageDetails:    `[{"limitId":"` + testLimitID.String() + `","limitAmount":1000,"scope":"account:` + testAccountID.String() + `","period":"DAILY","currentUsage":1500,"attemptedAmount":1500,"exceeded":true}]`,
				ProcessingTimeMs:     30,
				CreatedAt:            fixedTime,
			},
			expected: &model.TransactionValidation{
				ID:                   testID,
				RequestID:            testRequestID,
				TransactionType:      model.TransactionTypeCard,
				SubType:              &subType,
				Amount:               decimal.RequireFromString("1500"),
				Currency:             "BRL",
				TransactionTimestamp: txTimestamp,
				Account: model.AccountContext{
					ID:     testAccountID,
					Type:   "checking",
					Status: "active",
				},
				Segment: &model.SegmentContext{
					ID:   testSegmentID,
					Name: "VIP",
				},
				EvaluationResult: model.EvaluationResult{
					Decision:         model.DecisionDeny,
					Reason:           "Limit exceeded: daily_limit",
					MatchedRuleIDs:   []uuid.UUID{testMatchedRuleID, testEvaluatedRuleID},
					EvaluatedRuleIDs: []uuid.UUID{testMatchedRuleID, testEvaluatedRuleID},
				},
				LimitUsageDetails: []model.LimitUsageDetail{
					{
						LimitID:         testLimitID,
						LimitAmount:     decimal.RequireFromString("1000"),
						Scope:           "account:" + testAccountID.String(),
						Period:          model.LimitTypeDaily,
						CurrentUsage:    decimal.RequireFromString("1500"),
						AttemptedAmount: decimal.RequireFromString("1500"),
						Exceeded:        true,
					},
				},
				ProcessingTimeMs: 30,
				CreatedAt:        fixedTime,
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
			assert.Equal(t, tt.expected.RequestID, result.RequestID, "RequestID mismatch")
			assert.Equal(t, tt.expected.TransactionType, result.TransactionType, "TransactionType mismatch")
			assert.Equal(t, tt.expected.SubType, result.SubType, "SubType mismatch")
			assert.Equal(t, tt.expected.Amount, result.Amount, "Amount mismatch")
			assert.Equal(t, tt.expected.Currency, result.Currency, "Currency mismatch")
			assert.Equal(t, tt.expected.TransactionTimestamp, result.TransactionTimestamp, "TransactionTimestamp mismatch")
			assert.Equal(t, tt.expected.Account.ID, result.Account.ID, "Account.ID mismatch")
			assert.Equal(t, tt.expected.Account.Type, result.Account.Type, "Account.Type mismatch")
			assert.Equal(t, tt.expected.Account.Status, result.Account.Status, "Account.Status mismatch")
			assert.Equal(t, tt.expected.Decision, result.Decision, "Decision mismatch")
			assert.Equal(t, tt.expected.Reason, result.Reason, "Reason mismatch")
			assert.Equal(t, tt.expected.ProcessingTimeMs, result.ProcessingTimeMs, "ProcessingTimeMs mismatch")
			assert.Equal(t, tt.expected.CreatedAt, result.CreatedAt, "CreatedAt mismatch")

			// Validate optional context fields
			if tt.expected.Segment != nil {
				require.NotNil(t, result.Segment, "Segment should not be nil")
				assert.Equal(t, tt.expected.Segment.ID, result.Segment.ID, "Segment.ID mismatch")
				assert.Equal(t, tt.expected.Segment.Name, result.Segment.Name, "Segment.Name mismatch")
			} else {
				assert.Nil(t, result.Segment, "Segment should be nil")
			}

			if tt.expected.Portfolio != nil {
				require.NotNil(t, result.Portfolio, "Portfolio should not be nil")
				assert.Equal(t, tt.expected.Portfolio.ID, result.Portfolio.ID, "Portfolio.ID mismatch")
				assert.Equal(t, tt.expected.Portfolio.Name, result.Portfolio.Name, "Portfolio.Name mismatch")
			} else {
				assert.Nil(t, result.Portfolio, "Portfolio should be nil")
			}

			if tt.expected.Merchant != nil {
				require.NotNil(t, result.Merchant, "Merchant should not be nil")
				assert.Equal(t, tt.expected.Merchant.ID, result.Merchant.ID, "Merchant.ID mismatch")
				assert.Equal(t, tt.expected.Merchant.Name, result.Merchant.Name, "Merchant.Name mismatch")
				assert.Equal(t, tt.expected.Merchant.Category, result.Merchant.Category, "Merchant.Category mismatch")
				assert.Equal(t, tt.expected.Merchant.Country, result.Merchant.Country, "Merchant.Country mismatch")
			} else {
				assert.Nil(t, result.Merchant, "Merchant should be nil")
			}

			// Validate UUID arrays
			require.Len(t, result.MatchedRuleIDs, len(tt.expected.MatchedRuleIDs), "MatchedRuleIDs length mismatch")
			for i, expectedID := range tt.expected.MatchedRuleIDs {
				assert.Equal(t, expectedID, result.MatchedRuleIDs[i], "MatchedRuleIDs[%d] mismatch", i)
			}

			require.Len(t, result.EvaluatedRuleIDs, len(tt.expected.EvaluatedRuleIDs), "EvaluatedRuleIDs length mismatch")
			for i, expectedID := range tt.expected.EvaluatedRuleIDs {
				assert.Equal(t, expectedID, result.EvaluatedRuleIDs[i], "EvaluatedRuleIDs[%d] mismatch", i)
			}

			// Validate LimitUsageDetails
			require.Len(t, result.LimitUsageDetails, len(tt.expected.LimitUsageDetails), "LimitUsageDetails length mismatch")
			for i, expectedDetail := range tt.expected.LimitUsageDetails {
				assert.Equal(t, expectedDetail.LimitID, result.LimitUsageDetails[i].LimitID, "LimitUsageDetails[%d].LimitID mismatch", i)
				assert.Equal(t, expectedDetail.LimitAmount, result.LimitUsageDetails[i].LimitAmount, "LimitUsageDetails[%d].LimitAmount mismatch", i)
				assert.Equal(t, expectedDetail.CurrentUsage, result.LimitUsageDetails[i].CurrentUsage, "LimitUsageDetails[%d].CurrentUsage mismatch", i)
				assert.Equal(t, expectedDetail.Exceeded, result.LimitUsageDetails[i].Exceeded, "LimitUsageDetails[%d].Exceeded mismatch", i)
			}

			// Validate metadata (if expected)
			if tt.expected.Metadata != nil {
				require.NotNil(t, result.Metadata, "Metadata should not be nil")
				for k, v := range tt.expected.Metadata {
					assert.Equal(t, v, result.Metadata[k], "Metadata[%s] mismatch", k)
				}
			}
		})
	}
}

// TestTransactionValidationPostgreSQLModel_FromEntity tests the conversion from domain entity to database model.
// This test follows the ToEntity/FromEntity pattern from Ring Standards (golang/domain.md).
func TestTransactionValidationPostgreSQLModel_FromEntity(t *testing.T) {
	t.Parallel()

	// Deterministic test data following PROJECT_RULES.md
	testID := testutil.MustDeterministicUUID(10)
	testRequestID := testutil.MustDeterministicUUID(11)
	testAccountID := testutil.MustDeterministicUUID(12)
	testSegmentID := testutil.MustDeterministicUUID(13)
	testPortfolioID := testutil.MustDeterministicUUID(14)
	testMerchantID := testutil.MustDeterministicUUID(15)
	testMatchedRuleID := testutil.MustDeterministicUUID(16)
	testEvaluatedRuleID := testutil.MustDeterministicUUID(17)
	testLimitID := testutil.MustDeterministicUUID(18)
	fixedTime := testutil.FixedTime()
	txTimestamp := fixedTime.Add(-1 * time.Hour)

	subType := "debit"

	tests := []struct {
		name     string
		entity   *model.TransactionValidation
		assertFn func(t *testing.T, dbModel *TransactionValidationPostgreSQLModel)
	}{
		{
			name: "converts basic entity without optional fields",
			entity: &model.TransactionValidation{
				ID:                   testID,
				RequestID:            testRequestID,
				TransactionType:      model.TransactionTypeCard,
				SubType:              nil,
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: txTimestamp,
				Account: model.AccountContext{
					ID:     testAccountID,
					Type:   "checking",
					Status: "active",
				},
				Segment:   nil,
				Portfolio: nil,
				Merchant:  nil,
				Metadata:  nil,
				EvaluationResult: model.EvaluationResult{
					Decision:         model.DecisionAllow,
					Reason:           "No matching rules found",
					MatchedRuleIDs:   []uuid.UUID{},
					EvaluatedRuleIDs: []uuid.UUID{},
				},
				LimitUsageDetails: []model.LimitUsageDetail{},
				ProcessingTimeMs:  15,
				CreatedAt:         fixedTime,
			},
			assertFn: func(t *testing.T, dbModel *TransactionValidationPostgreSQLModel) {
				t.Helper()
				assert.Equal(t, testID.String(), dbModel.ID, "ID should be string representation of UUID")
				assert.Equal(t, testRequestID.String(), dbModel.RequestID, "RequestID should be string representation of UUID")
				assert.Equal(t, "CARD", dbModel.TransactionType)
				assert.Nil(t, dbModel.SubType, "SubType should be nil for nil input")
				assert.True(t, decimal.RequireFromString("100").Equal(dbModel.Amount), "Amount should be 100")
				assert.Equal(t, "BRL", dbModel.Currency)
				assert.Equal(t, txTimestamp, dbModel.TransactionTimestamp)
				assert.Equal(t, "ALLOW", dbModel.Decision)
				assert.Equal(t, "No matching rules found", dbModel.Reason)
				assert.Equal(t, float64(15), dbModel.ProcessingTimeMs)
				assert.Equal(t, fixedTime, dbModel.CreatedAt)

				// Validate JSONB fields
				assert.Contains(t, dbModel.Account, testAccountID.String(), "Account JSON should contain account ID")
				assert.Nil(t, dbModel.Segment, "Segment should be nil")
				assert.Nil(t, dbModel.Portfolio, "Portfolio should be nil")
				assert.Nil(t, dbModel.Merchant, "Merchant should be nil")
				assert.Equal(t, "{}", dbModel.Metadata, "Empty metadata should serialize to empty JSON object")
				assert.Equal(t, "[]", dbModel.LimitUsageDetails, "Empty limit usage should serialize to empty JSON array")

				// Validate UUID arrays
				assert.Equal(t, "{}", dbModel.MatchedRuleIds, "Empty matched rules should serialize to empty array")
				assert.Equal(t, "{}", dbModel.EvaluatedRuleIds, "Empty evaluated rules should serialize to empty array")
			},
		},
		{
			name: "converts entity with all optional fields populated",
			entity: &model.TransactionValidation{
				ID:                   testID,
				RequestID:            testRequestID,
				TransactionType:      model.TransactionTypePix,
				SubType:              &subType,
				Amount:               decimal.RequireFromString("500"),
				Currency:             "USD",
				TransactionTimestamp: txTimestamp,
				Account: model.AccountContext{
					ID:       testAccountID,
					Type:     "savings",
					Status:   "active",
					Metadata: map[string]any{"tier": "gold"},
				},
				Segment: &model.SegmentContext{
					ID:   testSegmentID,
					Name: "Premium",
				},
				Portfolio: &model.PortfolioContext{
					ID:   testPortfolioID,
					Name: "Corporate",
				},
				Merchant: &model.MerchantContext{
					ID:       testMerchantID,
					Name:     "Store",
					Category: "5411",
					Country:  "BR",
				},
				Metadata: map[string]any{"key1": "value1", "key2": 123},
				EvaluationResult: model.EvaluationResult{
					Decision:         model.DecisionDeny,
					Reason:           "Rule matched",
					MatchedRuleIDs:   []uuid.UUID{testMatchedRuleID},
					EvaluatedRuleIDs: []uuid.UUID{testEvaluatedRuleID, testMatchedRuleID},
				},
				LimitUsageDetails: []model.LimitUsageDetail{
					{
						LimitID:         testLimitID,
						LimitAmount:     decimal.RequireFromString("1000"),
						Scope:           "account:" + testAccountID.String(),
						Period:          model.LimitTypeDaily,
						CurrentUsage:    decimal.RequireFromString("500"),
						AttemptedAmount: decimal.RequireFromString("500"),
						Exceeded:        false,
					},
				},
				ProcessingTimeMs: 25,
				CreatedAt:        fixedTime,
			},
			assertFn: func(t *testing.T, dbModel *TransactionValidationPostgreSQLModel) {
				t.Helper()
				assert.Equal(t, testID.String(), dbModel.ID)
				assert.Equal(t, "PIX", dbModel.TransactionType)
				require.NotNil(t, dbModel.SubType, "SubType should not be nil")
				assert.Equal(t, "debit", *dbModel.SubType)
				assert.True(t, decimal.RequireFromString("500").Equal(dbModel.Amount), "Amount should be 500")
				assert.Equal(t, "USD", dbModel.Currency)
				assert.Equal(t, "DENY", dbModel.Decision)
				assert.Equal(t, "Rule matched", dbModel.Reason)

				// Validate JSONB fields contain expected data
				assert.Contains(t, dbModel.Account, testAccountID.String())
				assert.Contains(t, dbModel.Account, "savings")
				assert.Contains(t, dbModel.Account, "gold")

				require.NotNil(t, dbModel.Segment, "Segment should not be nil")
				assert.Contains(t, *dbModel.Segment, testSegmentID.String())
				assert.Contains(t, *dbModel.Segment, "Premium")

				require.NotNil(t, dbModel.Portfolio, "Portfolio should not be nil")
				assert.Contains(t, *dbModel.Portfolio, testPortfolioID.String())
				assert.Contains(t, *dbModel.Portfolio, "Corporate")

				require.NotNil(t, dbModel.Merchant, "Merchant should not be nil")
				assert.Contains(t, *dbModel.Merchant, testMerchantID.String())
				assert.Contains(t, *dbModel.Merchant, "Store")

				assert.Contains(t, dbModel.Metadata, "key1")
				assert.Contains(t, dbModel.Metadata, "value1")

				assert.Contains(t, dbModel.LimitUsageDetails, testLimitID.String())

				// Validate UUID arrays
				assert.Contains(t, dbModel.MatchedRuleIds, testMatchedRuleID.String())
				assert.Contains(t, dbModel.EvaluatedRuleIds, testEvaluatedRuleID.String())
				assert.Contains(t, dbModel.EvaluatedRuleIds, testMatchedRuleID.String())
			},
		},
		{
			name: "converts entity with nil slices to empty arrays",
			entity: &model.TransactionValidation{
				ID:                   testID,
				RequestID:            testRequestID,
				TransactionType:      model.TransactionTypeCard,
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: txTimestamp,
				Account: model.AccountContext{
					ID:     testAccountID,
					Type:   "checking",
					Status: "active",
				},
				EvaluationResult: model.EvaluationResult{
					Decision:         model.DecisionAllow,
					Reason:           "OK",
					MatchedRuleIDs:   nil, // Explicitly nil
					EvaluatedRuleIDs: nil, // Explicitly nil
				},
				LimitUsageDetails: nil, // Explicitly nil
				ProcessingTimeMs:  10,
				CreatedAt:         fixedTime,
			},
			assertFn: func(t *testing.T, dbModel *TransactionValidationPostgreSQLModel) {
				t.Helper()
				// Nil slices should be serialized as empty arrays
				assert.Equal(t, "{}", dbModel.MatchedRuleIds, "Nil matched rules should serialize to empty array")
				assert.Equal(t, "{}", dbModel.EvaluatedRuleIds, "Nil evaluated rules should serialize to empty array")
				assert.Equal(t, "[]", dbModel.LimitUsageDetails, "Nil limit usage should serialize to empty JSON array")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			var dbModel TransactionValidationPostgreSQLModel
			err := dbModel.FromEntity(tt.entity)
			require.NoError(t, err, "FromEntity should not return error for valid entity")

			// Assert
			tt.assertFn(t, &dbModel)
		})
	}
}

// TestTransactionValidationPostgreSQLModel_RoundTrip tests that ToEntity and FromEntity are inverses.
// entity -> FromEntity -> dbModel -> ToEntity -> entity (should be equal)
func TestTransactionValidationPostgreSQLModel_RoundTrip(t *testing.T) {
	t.Parallel()

	testID := testutil.MustDeterministicUUID(20)
	testRequestID := testutil.MustDeterministicUUID(21)
	testAccountID := testutil.MustDeterministicUUID(22)
	testSegmentID := testutil.MustDeterministicUUID(23)
	testMerchantID := testutil.MustDeterministicUUID(24)
	testMatchedRuleID := testutil.MustDeterministicUUID(25)
	testEvaluatedRuleID := testutil.MustDeterministicUUID(26)
	testLimitID := testutil.MustDeterministicUUID(27)
	fixedTime := testutil.FixedTime()
	txTimestamp := fixedTime.Add(-30 * time.Minute)
	subType := "purchase"

	original := &model.TransactionValidation{
		ID:                   testID,
		RequestID:            testRequestID,
		TransactionType:      model.TransactionTypeCard,
		SubType:              &subType,
		Amount:               decimal.RequireFromString("750"),
		Currency:             "BRL",
		TransactionTimestamp: txTimestamp,
		Account: model.AccountContext{
			ID:       testAccountID,
			Type:     "checking",
			Status:   "active",
			Metadata: map[string]any{"level": "premium"},
		},
		Segment: &model.SegmentContext{
			ID:   testSegmentID,
			Name: "VIP",
		},
		Portfolio: nil,
		Merchant: &model.MerchantContext{
			ID:       testMerchantID,
			Name:     "SuperStore",
			Category: "5411",
			Country:  "BR",
		},
		Metadata: map[string]any{"source": "mobile", "version": float64(2)},
		EvaluationResult: model.EvaluationResult{
			Decision:         model.DecisionAllow,
			Reason:           "Transaction allowed by rule evaluation",
			MatchedRuleIDs:   []uuid.UUID{testMatchedRuleID},
			EvaluatedRuleIDs: []uuid.UUID{testEvaluatedRuleID, testMatchedRuleID},
		},
		LimitUsageDetails: []model.LimitUsageDetail{
			{
				LimitID:         testLimitID,
				LimitAmount:     decimal.RequireFromString("1000"),
				Scope:           "account:" + testAccountID.String(),
				Period:          model.LimitTypeDaily,
				CurrentUsage:    decimal.RequireFromString("750"),
				AttemptedAmount: decimal.RequireFromString("750"),
				Exceeded:        false,
			},
		},
		ProcessingTimeMs: 20,
		CreatedAt:        fixedTime,
	}

	// entity -> dbModel
	var dbModel TransactionValidationPostgreSQLModel
	err := dbModel.FromEntity(original)
	require.NoError(t, err, "FromEntity should not return error in round-trip")

	// dbModel -> entity
	result, toErr := dbModel.ToEntity()

	// Assert equality
	require.NoError(t, toErr, "ToEntity should not return error")
	require.NotNil(t, result, "ToEntity should not return nil")
	assert.Equal(t, original.ID, result.ID, "Round-trip ID mismatch")
	assert.Equal(t, original.RequestID, result.RequestID, "Round-trip RequestID mismatch")
	assert.Equal(t, original.TransactionType, result.TransactionType, "Round-trip TransactionType mismatch")
	assert.Equal(t, original.SubType, result.SubType, "Round-trip SubType mismatch")
	assert.Equal(t, original.Amount, result.Amount, "Round-trip Amount mismatch")
	assert.Equal(t, original.Currency, result.Currency, "Round-trip Currency mismatch")
	assert.Equal(t, original.TransactionTimestamp, result.TransactionTimestamp, "Round-trip TransactionTimestamp mismatch")
	assert.Equal(t, original.Account.ID, result.Account.ID, "Round-trip Account.ID mismatch")
	assert.Equal(t, original.Account.Type, result.Account.Type, "Round-trip Account.Type mismatch")
	assert.Equal(t, original.Account.Status, result.Account.Status, "Round-trip Account.Status mismatch")
	assert.Equal(t, original.Decision, result.Decision, "Round-trip Decision mismatch")
	assert.Equal(t, original.Reason, result.Reason, "Round-trip Reason mismatch")
	assert.Equal(t, original.ProcessingTimeMs, result.ProcessingTimeMs, "Round-trip ProcessingTimeMs mismatch")
	assert.Equal(t, original.CreatedAt, result.CreatedAt, "Round-trip CreatedAt mismatch")

	// Validate optional context fields
	require.NotNil(t, result.Segment, "Segment should not be nil")
	assert.Equal(t, original.Segment.ID, result.Segment.ID, "Round-trip Segment.ID mismatch")
	assert.Equal(t, original.Segment.Name, result.Segment.Name, "Round-trip Segment.Name mismatch")

	assert.Nil(t, result.Portfolio, "Portfolio should be nil")

	require.NotNil(t, result.Merchant, "Merchant should not be nil")
	assert.Equal(t, original.Merchant.ID, result.Merchant.ID, "Round-trip Merchant.ID mismatch")
	assert.Equal(t, original.Merchant.Name, result.Merchant.Name, "Round-trip Merchant.Name mismatch")
	assert.Equal(t, original.Merchant.Category, result.Merchant.Category, "Round-trip Merchant.Category mismatch")
	assert.Equal(t, original.Merchant.Country, result.Merchant.Country, "Round-trip Merchant.Country mismatch")

	// Validate UUID arrays
	require.Len(t, result.MatchedRuleIDs, len(original.MatchedRuleIDs), "Round-trip MatchedRuleIDs length mismatch")
	for i := range original.MatchedRuleIDs {
		assert.Equal(t, original.MatchedRuleIDs[i], result.MatchedRuleIDs[i], "Round-trip MatchedRuleIDs[%d] mismatch", i)
	}

	require.Len(t, result.EvaluatedRuleIDs, len(original.EvaluatedRuleIDs), "Round-trip EvaluatedRuleIDs length mismatch")
	for i := range original.EvaluatedRuleIDs {
		assert.Equal(t, original.EvaluatedRuleIDs[i], result.EvaluatedRuleIDs[i], "Round-trip EvaluatedRuleIDs[%d] mismatch", i)
	}

	// Validate LimitUsageDetails
	require.Len(t, result.LimitUsageDetails, len(original.LimitUsageDetails), "Round-trip LimitUsageDetails length mismatch")
	for i := range original.LimitUsageDetails {
		assert.Equal(t, original.LimitUsageDetails[i].LimitID, result.LimitUsageDetails[i].LimitID, "Round-trip LimitUsageDetails[%d].LimitID mismatch", i)
		assert.Equal(t, original.LimitUsageDetails[i].LimitAmount, result.LimitUsageDetails[i].LimitAmount, "Round-trip LimitUsageDetails[%d].LimitAmount mismatch", i)
		assert.Equal(t, original.LimitUsageDetails[i].CurrentUsage, result.LimitUsageDetails[i].CurrentUsage, "Round-trip LimitUsageDetails[%d].CurrentUsage mismatch", i)
		assert.Equal(t, original.LimitUsageDetails[i].Exceeded, result.LimitUsageDetails[i].Exceeded, "Round-trip LimitUsageDetails[%d].Exceeded mismatch", i)
	}

	// Validate metadata
	require.NotNil(t, result.Metadata, "Metadata should not be nil")
	for k, v := range original.Metadata {
		assert.Equal(t, v, result.Metadata[k], "Round-trip Metadata[%s] mismatch", k)
	}
}

// TestTransactionValidationPostgreSQLModel_ToEntity_EdgeCases tests edge cases for ToEntity conversion.
func TestTransactionValidationPostgreSQLModel_ToEntity_EdgeCases(t *testing.T) {
	t.Parallel()

	fixedTime := testutil.FixedTime()
	txTimestamp := fixedTime.Add(-1 * time.Hour)
	testAccountID := testutil.MustDeterministicUUID(30)

	tests := []struct {
		name     string
		dbModel  TransactionValidationPostgreSQLModel
		validate func(t *testing.T, result *model.TransactionValidation)
	}{
		{
			name: "handles empty UUID arrays",
			dbModel: TransactionValidationPostgreSQLModel{
				ID:                   testutil.MustDeterministicUUID(31).String(),
				RequestID:            testutil.MustDeterministicUUID(32).String(),
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: txTimestamp,
				Account:              `{"accountId":"` + testAccountID.String() + `","type":"checking","status":"active"}`,
				Metadata:             "{}",
				Decision:             "ALLOW",
				Reason:               "OK",
				MatchedRuleIds:       "{}",
				EvaluatedRuleIds:     "{}",
				LimitUsageDetails:    "[]",
				ProcessingTimeMs:     10,
				CreatedAt:            fixedTime,
			},
			validate: func(t *testing.T, result *model.TransactionValidation) {
				t.Helper()
				require.NotNil(t, result)
				assert.Empty(t, result.MatchedRuleIDs, "MatchedRuleIDs should be empty slice, not nil")
				assert.NotNil(t, result.MatchedRuleIDs, "MatchedRuleIDs should never be nil")
				assert.Empty(t, result.EvaluatedRuleIDs, "EvaluatedRuleIDs should be empty slice, not nil")
				assert.NotNil(t, result.EvaluatedRuleIDs, "EvaluatedRuleIDs should never be nil")
			},
		},
		{
			name: "handles empty string for UUID arrays",
			dbModel: TransactionValidationPostgreSQLModel{
				ID:                   testutil.MustDeterministicUUID(33).String(),
				RequestID:            testutil.MustDeterministicUUID(34).String(),
				TransactionType:      "PIX",
				Amount:               decimal.RequireFromString("200"),
				Currency:             "USD",
				TransactionTimestamp: txTimestamp,
				Account:              `{"accountId":"` + testAccountID.String() + `","type":"savings","status":"active"}`,
				Metadata:             "{}",
				Decision:             "DENY",
				Reason:               "Denied",
				MatchedRuleIds:       "",
				EvaluatedRuleIds:     "",
				LimitUsageDetails:    "[]",
				ProcessingTimeMs:     5,
				CreatedAt:            fixedTime,
			},
			validate: func(t *testing.T, result *model.TransactionValidation) {
				t.Helper()
				require.NotNil(t, result)
				assert.Empty(t, result.MatchedRuleIDs)
				assert.Empty(t, result.EvaluatedRuleIDs)
			},
		},
		{
			name: "handles empty limit usage details",
			dbModel: TransactionValidationPostgreSQLModel{
				ID:                   testutil.MustDeterministicUUID(35).String(),
				RequestID:            testutil.MustDeterministicUUID(36).String(),
				TransactionType:      "WIRE",
				Amount:               decimal.RequireFromString("1000"),
				Currency:             "EUR",
				TransactionTimestamp: txTimestamp,
				Account:              `{"accountId":"` + testAccountID.String() + `","type":"checking","status":"active"}`,
				Metadata:             "{}",
				Decision:             "ALLOW",
				Reason:               "OK",
				MatchedRuleIds:       "{}",
				EvaluatedRuleIds:     "{}",
				LimitUsageDetails:    "[]",
				ProcessingTimeMs:     15,
				CreatedAt:            fixedTime,
			},
			validate: func(t *testing.T, result *model.TransactionValidation) {
				t.Helper()
				require.NotNil(t, result)
				assert.Empty(t, result.LimitUsageDetails, "LimitUsageDetails should be empty slice, not nil")
				assert.NotNil(t, result.LimitUsageDetails, "LimitUsageDetails should never be nil")
			},
		},
		{
			name: "handles nil optional JSONB fields",
			dbModel: TransactionValidationPostgreSQLModel{
				ID:                   testutil.MustDeterministicUUID(37).String(),
				RequestID:            testutil.MustDeterministicUUID(38).String(),
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("50"),
				Currency:             "BRL",
				TransactionTimestamp: txTimestamp,
				Account:              `{"accountId":"` + testAccountID.String() + `","type":"credit","status":"active"}`,
				Segment:              nil,
				Portfolio:            nil,
				Merchant:             nil,
				Metadata:             "{}",
				Decision:             "ALLOW",
				Reason:               "OK",
				MatchedRuleIds:       "{}",
				EvaluatedRuleIds:     "{}",
				LimitUsageDetails:    "[]",
				ProcessingTimeMs:     8,
				CreatedAt:            fixedTime,
			},
			validate: func(t *testing.T, result *model.TransactionValidation) {
				t.Helper()
				require.NotNil(t, result)
				assert.Nil(t, result.Segment, "Segment should be nil")
				assert.Nil(t, result.Portfolio, "Portfolio should be nil")
				assert.Nil(t, result.Merchant, "Merchant should be nil")
			},
		},
		{
			name: "handles empty string for optional JSONB fields",
			dbModel: TransactionValidationPostgreSQLModel{
				ID:                   testutil.MustDeterministicUUID(39).String(),
				RequestID:            testutil.MustDeterministicUUID(40).String(),
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("50"),
				Currency:             "BRL",
				TransactionTimestamp: txTimestamp,
				Account:              `{"accountId":"` + testAccountID.String() + `","type":"credit","status":"active"}`,
				Segment:              testutil.StringPtr(""),
				Portfolio:            testutil.StringPtr(""),
				Merchant:             testutil.StringPtr(""),
				Metadata:             "{}",
				Decision:             "ALLOW",
				Reason:               "OK",
				MatchedRuleIds:       "{}",
				EvaluatedRuleIds:     "{}",
				LimitUsageDetails:    "[]",
				ProcessingTimeMs:     8,
				CreatedAt:            fixedTime,
			},
			validate: func(t *testing.T, result *model.TransactionValidation) {
				t.Helper()
				require.NotNil(t, result)
				// Empty strings should result in nil pointers
				assert.Nil(t, result.Segment, "Segment should be nil for empty string")
				assert.Nil(t, result.Portfolio, "Portfolio should be nil for empty string")
				assert.Nil(t, result.Merchant, "Merchant should be nil for empty string")
			},
		},
		{
			name: "handles all decision types",
			dbModel: TransactionValidationPostgreSQLModel{
				ID:                   testutil.MustDeterministicUUID(41).String(),
				RequestID:            testutil.MustDeterministicUUID(42).String(),
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: txTimestamp,
				Account:              `{"accountId":"` + testAccountID.String() + `","type":"checking","status":"active"}`,
				Metadata:             "{}",
				Decision:             "REVIEW",
				Reason:               "Manual review required",
				MatchedRuleIds:       "{}",
				EvaluatedRuleIds:     "{}",
				LimitUsageDetails:    "[]",
				ProcessingTimeMs:     12,
				CreatedAt:            fixedTime,
			},
			validate: func(t *testing.T, result *model.TransactionValidation) {
				t.Helper()
				require.NotNil(t, result)
				assert.Equal(t, model.DecisionReview, result.Decision)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.dbModel.ToEntity()
			require.NoError(t, err, "ToEntity should not return error for edge cases")
			tt.validate(t, result)
		})
	}
}

// Test helper functions
func TestParseUUIDArrayString(t *testing.T) {
	t.Parallel()

	testUUID1 := testutil.MustDeterministicUUID(50)
	testUUID2 := testutil.MustDeterministicUUID(51)

	t.Run("valid inputs", func(t *testing.T) {
		tests := []struct {
			name     string
			input    string
			expected []uuid.UUID
		}{
			{
				name:     "empty string returns empty slice",
				input:    "",
				expected: []uuid.UUID{},
			},
			{
				name:     "empty braces returns empty slice",
				input:    "{}",
				expected: []uuid.UUID{},
			},
			{
				name:     "single UUID",
				input:    "{" + testUUID1.String() + "}",
				expected: []uuid.UUID{testUUID1},
			},
			{
				name:     "multiple UUIDs",
				input:    "{" + testUUID1.String() + "," + testUUID2.String() + "}",
				expected: []uuid.UUID{testUUID1, testUUID2},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := parseUUIDArrayString(tt.input)
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("invalid inputs return errors", func(t *testing.T) {
		tests := []struct {
			name  string
			input string
		}{
			{
				name:  "invalid UUID returns error",
				input: "{invalid-uuid}",
			},
			{
				name:  "partially invalid UUIDs return error",
				input: "{invalid-uuid," + testUUID1.String() + "}",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := parseUUIDArrayString(tt.input)
				require.Error(t, err)
				require.Contains(t, err.Error(), "invalid UUID")
			})
		}
	})
}

func TestFormatUUIDArrayString(t *testing.T) {
	t.Parallel()

	testUUID1 := testutil.MustDeterministicUUID(60)
	testUUID2 := testutil.MustDeterministicUUID(61)

	tests := []struct {
		name     string
		input    []uuid.UUID
		expected string
	}{
		{
			name:     "nil slice returns empty braces",
			input:    nil,
			expected: "{}",
		},
		{
			name:     "empty slice returns empty braces",
			input:    []uuid.UUID{},
			expected: "{}",
		},
		{
			name:     "single UUID",
			input:    []uuid.UUID{testUUID1},
			expected: "{" + testUUID1.String() + "}",
		},
		{
			name:     "multiple UUIDs",
			input:    []uuid.UUID{testUUID1, testUUID2},
			expected: "{" + testUUID1.String() + "," + testUUID2.String() + "}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatUUIDArrayString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTransactionValidationPostgreSQLModel_ToEntity_InvalidUUIDs tests error handling for invalid UUIDs.
func TestTransactionValidationPostgreSQLModel_ToEntity_InvalidUUIDs(t *testing.T) {
	t.Parallel()

	testID := testutil.MustDeterministicUUID(1)
	testRequestID := testutil.MustDeterministicUUID(2)
	fixedTime := testutil.FixedTime()

	tests := []struct {
		name             string
		matchedRuleIds   string
		evaluatedRuleIds string
		expectedError    string
	}{
		{
			name:             "invalid UUID in matched_rule_ids",
			matchedRuleIds:   "{not-a-uuid}",
			evaluatedRuleIds: "{}",
			expectedError:    "failed to parse matched_rule_ids",
		},
		{
			name:             "invalid UUID in evaluated_rule_ids",
			matchedRuleIds:   "{}",
			evaluatedRuleIds: "{invalid-uuid}",
			expectedError:    "failed to parse evaluated_rule_ids",
		},
		{
			name:             "partially invalid UUIDs in matched_rule_ids",
			matchedRuleIds:   "{" + testID.String() + ",not-valid}",
			evaluatedRuleIds: "{}",
			expectedError:    "failed to parse matched_rule_ids",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbModel := TransactionValidationPostgreSQLModel{
				ID:                   testID.String(),
				RequestID:            testRequestID.String(),
				TransactionType:      "CARD",
				Amount:               decimal.RequireFromString("100"),
				Currency:             "BRL",
				TransactionTimestamp: fixedTime,
				Account:              `{"id":"` + testID.String() + `","type":"checking"}`,
				Metadata:             "{}",
				Decision:             "ALLOW",
				Reason:               "no_limits_breached",
				MatchedRuleIds:       tt.matchedRuleIds,
				EvaluatedRuleIds:     tt.evaluatedRuleIds,
				LimitUsageDetails:    "[]",
				ProcessingTimeMs:     150,
				CreatedAt:            fixedTime,
			}

			_, err := dbModel.ToEntity()

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.expectedError)
		})
	}
}

func TestTransactionValidationPostgreSQLModel_FromEntity_NilEntity(t *testing.T) {
	t.Parallel()

	var dbModel TransactionValidationPostgreSQLModel

	err := dbModel.FromEntity(nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot be nil")
}
