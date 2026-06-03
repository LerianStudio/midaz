// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db"
	pgdbMocks "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/command"
	commandMocks "github.com/LerianStudio/midaz/v3/components/tracer/internal/services/command/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/query"
	queryMocks "github.com/LerianStudio/midaz/v3/components/tracer/internal/services/query/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

func TestValidateTransaction(t *testing.T) {
	// Common test fixtures
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	ruleID1 := testutil.MustDeterministicUUID(10)
	ruleID2 := testutil.MustDeterministicUUID(11)
	limitID := testutil.MustDeterministicUUID(20)

	baseRequest := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	tests := []struct {
		name             string
		request          *model.ValidationRequest
		setupMocks       func(ctrl *gomock.Controller, persistDone chan struct{}) (pgdb.TxBeginner, RuleEvaluator, LimitChecker, command.TransactionValidationRepository, query.TransactionValidationRepository, AuditWriter)
		expectedDecision model.Decision
		expectedReason   string
		expectError      bool
		expectedErr      error
		cancelContext    bool
	}{
		{
			name:    "DENY by rule - rule evaluation returns DENY",
			request: baseRequest,
			setupMocks: func(ctrl *gomock.Controller, persistDone chan struct{}) (pgdb.TxBeginner, RuleEvaluator, LimitChecker, command.TransactionValidationRepository, query.TransactionValidationRepository, AuditWriter) {
				mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
				ruleEval := mocks.NewMockRuleEvaluator(ctrl)
				limitCheck := mocks.NewMockLimitChecker(ctrl)
				transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
				transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

				// FindByRequestID returns nil (no existing record) - new request
				transactionValidationQueryRepo.EXPECT().
					FindByRequestID(gomock.Any(), gomock.Any()).
					Return(nil, nil).
					Times(1)

				// AuditWriter mock - expects RecordValidationEvent call (non-transactional)
				auditWriter := mocks.NewMockAuditWriter(ctrl)
				auditWriter.EXPECT().RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				// Rule evaluation returns DENY
				evalResult, err := model.NewEvaluationResult(
					model.DecisionDeny,
					[]uuid.UUID{ruleID1},
					[]uuid.UUID{ruleID1, ruleID2},
					"Rule blocked transaction",
				)
				require.NoError(t, err)
				ruleEval.EXPECT().
					Execute(gomock.Any(), gomock.Any()).
					Return(evalResult, nil)

				// DENY by rule: No transaction is started, no limit check
				// BeginTx should NOT be called
				mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
				limitCheck.EXPECT().CheckLimits(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

				// Audit should be inserted (non-transactional) - signal completion via channel
				transactionValidationRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ *model.TransactionValidation) error {
						close(persistDone)
						return nil
					})

				return mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter
			},
			expectedDecision: model.DecisionDeny,
			expectedReason:   "Rule blocked transaction",
			expectError:      false,
		},
		{
			name:    "DENY by exceeded limit - limit check returns exceeded",
			request: baseRequest,
			setupMocks: func(ctrl *gomock.Controller, persistDone chan struct{}) (pgdb.TxBeginner, RuleEvaluator, LimitChecker, command.TransactionValidationRepository, query.TransactionValidationRepository, AuditWriter) {
				mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
				mockTx := pgdbMocks.NewMockTx(ctrl)
				ruleEval := mocks.NewMockRuleEvaluator(ctrl)
				limitCheck := mocks.NewMockLimitChecker(ctrl)
				transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
				transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

				// FindByRequestID returns nil (no existing record) - new request
				transactionValidationQueryRepo.EXPECT().
					FindByRequestID(gomock.Any(), gomock.Any()).
					Return(nil, nil).
					Times(1)

				// AuditWriter mock - expects RecordValidationEvent call (non-transactional after rollback)
				auditWriter := mocks.NewMockAuditWriter(ctrl)
				auditWriter.EXPECT().RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				// Rule evaluation returns ALLOW (no blocking rules)
				evalResult, err := model.NewEvaluationResult(
					model.DecisionAllow,
					[]uuid.UUID{ruleID1},
					[]uuid.UUID{ruleID1},
					"Rule allowed transaction",
				)
				require.NoError(t, err)
				ruleEval.EXPECT().
					Execute(gomock.Any(), gomock.Any()).
					Return(evalResult, nil)

				// BeginTx is called
				mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil).Times(1)

				// Limit check returns exceeded (via CheckLimits)
				limitOutput := &model.CheckLimitsOutput{
					Allowed: false,
					LimitUsageDetails: []model.LimitUsageDetail{
						{
							LimitID:      limitID,
							LimitAmount:  decimal.RequireFromString("50"),
							Scope:        "account:" + limitID.String(),
							Period:       model.LimitTypeDaily,
							CurrentUsage: decimal.RequireFromString("60"),
							Exceeded:     true,
						},
					},
					ExceededLimitIDs: []uuid.UUID{limitID},
				}
				limitCheck.EXPECT().
					CheckLimits(gomock.Any(), mockTx, gomock.Any()).
					Return(limitOutput, nil)

				// Rollback is called to undo counter increments
				mockTx.EXPECT().Rollback().Return(nil).Times(1)

				// Audit should be inserted (non-transactional) - signal completion via channel
				transactionValidationRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ *model.TransactionValidation) error {
						close(persistDone)
						return nil
					})

				return mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter
			},
			expectedDecision: model.DecisionDeny,
			expectedReason:   "limit_exceeded",
			expectError:      false,
		},
		{
			name:    "REVIEW when REVIEW rules match - no DENY",
			request: baseRequest,
			setupMocks: func(ctrl *gomock.Controller, persistDone chan struct{}) (pgdb.TxBeginner, RuleEvaluator, LimitChecker, command.TransactionValidationRepository, query.TransactionValidationRepository, AuditWriter) {
				mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
				mockTx := pgdbMocks.NewMockTx(ctrl)
				ruleEval := mocks.NewMockRuleEvaluator(ctrl)
				limitCheck := mocks.NewMockLimitChecker(ctrl)
				transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
				transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

				// FindByRequestID returns nil (no existing record) - new request
				transactionValidationQueryRepo.EXPECT().
					FindByRequestID(gomock.Any(), gomock.Any()).
					Return(nil, nil).
					Times(1)

				// AuditWriter mock - expects RecordValidationEvent call (non-transactional after rollback)
				auditWriter := mocks.NewMockAuditWriter(ctrl)
				auditWriter.EXPECT().RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				// Rule evaluation returns REVIEW
				evalResult, err := model.NewEvaluationResult(
					model.DecisionReview,
					[]uuid.UUID{ruleID1},
					[]uuid.UUID{ruleID1, ruleID2},
					"Rule requires review",
				)
				require.NoError(t, err)
				ruleEval.EXPECT().
					Execute(gomock.Any(), gomock.Any()).
					Return(evalResult, nil)

				// BeginTx is called
				mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil).Times(1)

				// Limit check via CheckLimits (REVIEW doesn't short-circuit)
				limitOutput := &model.CheckLimitsOutput{
					Allowed: true,
					LimitUsageDetails: []model.LimitUsageDetail{
						{
							LimitID:           limitID,
							LimitAmount:       decimal.RequireFromString("1000"),
							Scope:             "acct:" + accountID.String(),
							Period:            model.LimitTypeDaily,
							CurrentUsage:      decimal.RequireFromString("100"),
							AttemptedAmount:   decimal.RequireFromString("100"),
							Exceeded:          false,
							InternalLimitType: model.LimitTypeDaily,
							Scopes:            []model.Scope{{AccountID: &accountID}},
							InternalPeriodKey: "2025-01-15",
						},
					},
					ExceededLimitIDs: []uuid.UUID{},
				}
				limitCheck.EXPECT().
					CheckLimits(gomock.Any(), mockTx, gomock.Any()).
					Return(limitOutput, nil)

				// REVIEW decision triggers tx.Rollback to undo counter increments
				mockTx.EXPECT().Rollback().Return(nil).Times(1)

				// Audit should be inserted (non-transactional) - signal completion via channel
				transactionValidationRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ *model.TransactionValidation) error {
						close(persistDone)
						return nil
					})

				return mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter
			},
			expectedDecision: model.DecisionReview,
			expectedReason:   "Rule requires review",
			expectError:      false,
		},
		{
			name:    "REVIEW rollback failure is non-fatal",
			request: baseRequest,
			setupMocks: func(ctrl *gomock.Controller, persistDone chan struct{}) (pgdb.TxBeginner, RuleEvaluator, LimitChecker, command.TransactionValidationRepository, query.TransactionValidationRepository, AuditWriter) {
				mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
				mockTx := pgdbMocks.NewMockTx(ctrl)
				ruleEval := mocks.NewMockRuleEvaluator(ctrl)
				limitCheck := mocks.NewMockLimitChecker(ctrl)
				transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
				transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

				// FindByRequestID returns nil (no existing record) - new request
				transactionValidationQueryRepo.EXPECT().
					FindByRequestID(gomock.Any(), gomock.Any()).
					Return(nil, nil).
					Times(1)

				// AuditWriter mock - expects RecordValidationEvent call (non-transactional after rollback)
				auditWriter := mocks.NewMockAuditWriter(ctrl)
				auditWriter.EXPECT().RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				// Rule evaluation returns REVIEW
				evalResult, err := model.NewEvaluationResult(
					model.DecisionReview,
					[]uuid.UUID{ruleID1},
					[]uuid.UUID{ruleID1, ruleID2},
					"Rule requires review",
				)
				require.NoError(t, err)
				ruleEval.EXPECT().
					Execute(gomock.Any(), gomock.Any()).
					Return(evalResult, nil)

				// BeginTx is called
				mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil).Times(1)

				// Limit check via CheckLimits
				limitOutput := &model.CheckLimitsOutput{
					Allowed: true,
					LimitUsageDetails: []model.LimitUsageDetail{
						{
							LimitID:           limitID,
							LimitAmount:       decimal.RequireFromString("1000"),
							Scope:             "acct:" + accountID.String(),
							Period:            model.LimitTypeDaily,
							CurrentUsage:      decimal.RequireFromString("100"),
							AttemptedAmount:   decimal.RequireFromString("100"),
							Exceeded:          false,
							InternalLimitType: model.LimitTypeDaily,
							Scopes:            []model.Scope{{AccountID: &accountID}},
							InternalPeriodKey: "2025-01-15",
						},
					},
					ExceededLimitIDs: []uuid.UUID{},
				}
				limitCheck.EXPECT().
					CheckLimits(gomock.Any(), mockTx, gomock.Any()).
					Return(limitOutput, nil)

				// REVIEW decision triggers tx.Rollback - ROLLBACK FAILS (DB timeout)
				// Rollback failure is logged but should NOT fail the validation
				mockTx.EXPECT().Rollback().Return(errors.New("database timeout during rollback")).Times(1)

				// Audit should still be inserted despite rollback failure
				transactionValidationRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ *model.TransactionValidation) error {
						close(persistDone)
						return nil
					})

				return mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter
			},
			expectedDecision: model.DecisionReview,
			expectedReason:   "Rule requires review",
			expectError:      false, // Rollback failure should NOT fail the validation
		},
		{
			name:    "DENY by limit takes precedence over REVIEW",
			request: baseRequest,
			setupMocks: func(ctrl *gomock.Controller, persistDone chan struct{}) (pgdb.TxBeginner, RuleEvaluator, LimitChecker, command.TransactionValidationRepository, query.TransactionValidationRepository, AuditWriter) {
				mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
				mockTx := pgdbMocks.NewMockTx(ctrl)
				ruleEval := mocks.NewMockRuleEvaluator(ctrl)
				limitCheck := mocks.NewMockLimitChecker(ctrl)
				transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
				transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

				// FindByRequestID returns nil (no existing record) - new request
				transactionValidationQueryRepo.EXPECT().
					FindByRequestID(gomock.Any(), gomock.Any()).
					Return(nil, nil).
					Times(1)

				// AuditWriter mock - expects RecordValidationEvent call (non-transactional after rollback)
				auditWriter := mocks.NewMockAuditWriter(ctrl)
				auditWriter.EXPECT().RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				// Rule evaluation returns REVIEW
				evalResult, err := model.NewEvaluationResult(
					model.DecisionReview,
					[]uuid.UUID{ruleID1},
					[]uuid.UUID{ruleID1},
					"Rule requires review",
				)
				require.NoError(t, err)
				ruleEval.EXPECT().
					Execute(gomock.Any(), gomock.Any()).
					Return(evalResult, nil)

				// BeginTx is called
				mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil).Times(1)

				// Limit check returns exceeded (via CheckLimits) - should override REVIEW
				limitOutput := &model.CheckLimitsOutput{
					Allowed: false,
					LimitUsageDetails: []model.LimitUsageDetail{
						{
							LimitID:      limitID,
							LimitAmount:  decimal.RequireFromString("50"),
							Scope:        "account:" + limitID.String(),
							Period:       model.LimitTypeDaily,
							CurrentUsage: decimal.RequireFromString("60"),
							Exceeded:     true,
						},
					},
					ExceededLimitIDs: []uuid.UUID{limitID},
				}
				limitCheck.EXPECT().
					CheckLimits(gomock.Any(), mockTx, gomock.Any()).
					Return(limitOutput, nil)

				// Rollback is called to undo counter increments
				mockTx.EXPECT().Rollback().Return(nil).Times(1)

				// Audit should be inserted (non-transactional) - signal completion via channel
				transactionValidationRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ *model.TransactionValidation) error {
						close(persistDone)
						return nil
					})

				return mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter
			},
			expectedDecision: model.DecisionDeny,
			expectedReason:   "limit_exceeded",
			expectError:      false,
		},
		{
			name:    "ALLOW with matched ALLOW rules",
			request: baseRequest,
			setupMocks: func(ctrl *gomock.Controller, persistDone chan struct{}) (pgdb.TxBeginner, RuleEvaluator, LimitChecker, command.TransactionValidationRepository, query.TransactionValidationRepository, AuditWriter) {
				mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
				mockTx := pgdbMocks.NewMockTx(ctrl)
				ruleEval := mocks.NewMockRuleEvaluator(ctrl)
				limitCheck := mocks.NewMockLimitChecker(ctrl)
				transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
				transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

				// FindByRequestID returns nil (no existing record) - new request
				transactionValidationQueryRepo.EXPECT().
					FindByRequestID(gomock.Any(), gomock.Any()).
					Return(nil, nil).
					Times(1)

				// AuditWriter mock - expects RecordValidationEventWithTx call (transactional for ALLOW)
				auditWriter := mocks.NewMockAuditWriter(ctrl)
				auditWriter.EXPECT().RecordValidationEventWithTx(gomock.Any(), mockTx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				// Rule evaluation returns ALLOW with matched rules
				evalResult, err := model.NewEvaluationResult(
					model.DecisionAllow,
					[]uuid.UUID{ruleID1},
					[]uuid.UUID{ruleID1, ruleID2},
					"Rule allowed transaction",
				)
				require.NoError(t, err)
				ruleEval.EXPECT().
					Execute(gomock.Any(), gomock.Any()).
					Return(evalResult, nil)

				// BeginTx is called
				mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil).Times(1)

				// Limit check passes via CheckLimits
				limitOutput := &model.CheckLimitsOutput{
					Allowed: true,
					LimitUsageDetails: []model.LimitUsageDetail{
						{
							LimitID:      limitID,
							LimitAmount:  decimal.RequireFromString("500"),
							Scope:        "account:" + limitID.String(),
							Period:       model.LimitTypeDaily,
							CurrentUsage: decimal.RequireFromString("100"),
							Exceeded:     false,
						},
					},
					ExceededLimitIDs: []uuid.UUID{},
				}
				limitCheck.EXPECT().
					CheckLimits(gomock.Any(), mockTx, gomock.Any()).
					Return(limitOutput, nil)

				// Audit should be inserted with transaction (InsertWithTx) - signal completion via channel
				transactionValidationRepo.EXPECT().InsertWithTx(gomock.Any(), mockTx, gomock.Any()).DoAndReturn(
					func(_ context.Context, _ pgdb.DB, _ *model.TransactionValidation) error {
						close(persistDone)
						return nil
					})

				// Commit is called after successful ALLOW
				mockTx.EXPECT().Commit().Return(nil).Times(1)

				return mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter
			},
			expectedDecision: model.DecisionAllow,
			expectedReason:   "Rule allowed transaction",
			expectError:      false,
		},
		{
			name:    "ALLOW with default decision - no rules matched",
			request: baseRequest,
			setupMocks: func(ctrl *gomock.Controller, persistDone chan struct{}) (pgdb.TxBeginner, RuleEvaluator, LimitChecker, command.TransactionValidationRepository, query.TransactionValidationRepository, AuditWriter) {
				mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
				mockTx := pgdbMocks.NewMockTx(ctrl)
				ruleEval := mocks.NewMockRuleEvaluator(ctrl)
				limitCheck := mocks.NewMockLimitChecker(ctrl)
				transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
				transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

				// FindByRequestID returns nil (no existing record) - new request
				transactionValidationQueryRepo.EXPECT().
					FindByRequestID(gomock.Any(), gomock.Any()).
					Return(nil, nil).
					Times(1)

				// AuditWriter mock - expects RecordValidationEventWithTx call (transactional for ALLOW)
				auditWriter := mocks.NewMockAuditWriter(ctrl)
				auditWriter.EXPECT().RecordValidationEventWithTx(gomock.Any(), mockTx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

				// Rule evaluation returns ALLOW with no matched rules (default)
				evalResult, err := model.NewNoMatchResult(model.DecisionAllow, []uuid.UUID{ruleID1, ruleID2})
				require.NoError(t, err)
				ruleEval.EXPECT().
					Execute(gomock.Any(), gomock.Any()).
					Return(evalResult, nil)

				// BeginTx is called
				mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil).Times(1)

				// Limit check passes via CheckLimits
				limitOutput := &model.CheckLimitsOutput{
					Allowed:           true,
					LimitUsageDetails: []model.LimitUsageDetail{},
					ExceededLimitIDs:  []uuid.UUID{},
				}
				limitCheck.EXPECT().
					CheckLimits(gomock.Any(), mockTx, gomock.Any()).
					Return(limitOutput, nil)

				// Audit should be inserted with transaction (InsertWithTx) - signal completion via channel
				transactionValidationRepo.EXPECT().InsertWithTx(gomock.Any(), mockTx, gomock.Any()).DoAndReturn(
					func(_ context.Context, _ pgdb.DB, _ *model.TransactionValidation) error {
						close(persistDone)
						return nil
					})

				// Commit is called after successful ALLOW
				mockTx.EXPECT().Commit().Return(nil).Times(1)

				return mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter
			},
			expectedDecision: model.DecisionAllow,
			expectedReason:   "No matching rules found",
			expectError:      false,
		},
		{
			name:    "context cancellation at start - returns error immediately",
			request: baseRequest,
			setupMocks: func(ctrl *gomock.Controller, _ chan struct{}) (pgdb.TxBeginner, RuleEvaluator, LimitChecker, command.TransactionValidationRepository, query.TransactionValidationRepository, AuditWriter) {
				mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
				ruleEval := mocks.NewMockRuleEvaluator(ctrl)
				limitCheck := mocks.NewMockLimitChecker(ctrl)
				transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
				transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

				// AuditWriter mock
				auditWriter := mocks.NewMockAuditWriter(ctrl)

				// No calls should be made when context is cancelled (checked before FindByRequestID)
				mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
				transactionValidationQueryRepo.EXPECT().FindByRequestID(gomock.Any(), gomock.Any()).Times(0)
				ruleEval.EXPECT().Execute(gomock.Any(), gomock.Any()).Times(0)
				limitCheck.EXPECT().CheckLimits(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				transactionValidationRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).Times(0)
				transactionValidationRepo.EXPECT().InsertWithTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				auditWriter.EXPECT().RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				auditWriter.EXPECT().RecordValidationEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

				return mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter
			},
			expectError:   true,
			expectedErr:   context.Canceled,
			cancelContext: true,
		},
		{
			name:    "error from rule evaluator - propagates error",
			request: baseRequest,
			setupMocks: func(ctrl *gomock.Controller, _ chan struct{}) (pgdb.TxBeginner, RuleEvaluator, LimitChecker, command.TransactionValidationRepository, query.TransactionValidationRepository, AuditWriter) {
				mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
				ruleEval := mocks.NewMockRuleEvaluator(ctrl)
				limitCheck := mocks.NewMockLimitChecker(ctrl)
				transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
				transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

				// FindByRequestID returns nil (no existing record) - new request
				transactionValidationQueryRepo.EXPECT().
					FindByRequestID(gomock.Any(), gomock.Any()).
					Return(nil, nil).
					Times(1)

				// AuditWriter mock - should NOT be called on error
				auditWriter := mocks.NewMockAuditWriter(ctrl)
				auditWriter.EXPECT().RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				auditWriter.EXPECT().RecordValidationEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

				// Rule evaluation returns error
				ruleEval.EXPECT().
					Execute(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("rule evaluation failed"))

				// No transaction or limit check when rule eval fails
				mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
				limitCheck.EXPECT().CheckLimits(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

				// Audit should NOT be inserted on error
				transactionValidationRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).Times(0)
				transactionValidationRepo.EXPECT().InsertWithTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

				return mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter
			},
			expectError: true,
		},
		{
			name:    "error from limit checker - propagates error",
			request: baseRequest,
			setupMocks: func(ctrl *gomock.Controller, _ chan struct{}) (pgdb.TxBeginner, RuleEvaluator, LimitChecker, command.TransactionValidationRepository, query.TransactionValidationRepository, AuditWriter) {
				mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
				mockTx := pgdbMocks.NewMockTx(ctrl)
				ruleEval := mocks.NewMockRuleEvaluator(ctrl)
				limitCheck := mocks.NewMockLimitChecker(ctrl)
				transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
				transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

				// FindByRequestID returns nil (no existing record) - new request
				transactionValidationQueryRepo.EXPECT().
					FindByRequestID(gomock.Any(), gomock.Any()).
					Return(nil, nil).
					Times(1)

				// AuditWriter mock - should NOT be called on error
				auditWriter := mocks.NewMockAuditWriter(ctrl)
				auditWriter.EXPECT().RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				auditWriter.EXPECT().RecordValidationEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

				// Rule evaluation returns ALLOW
				evalResult, err := model.NewEvaluationResult(
					model.DecisionAllow,
					[]uuid.UUID{ruleID1},
					[]uuid.UUID{ruleID1},
					"Rule allowed transaction",
				)
				require.NoError(t, err)
				ruleEval.EXPECT().
					Execute(gomock.Any(), gomock.Any()).
					Return(evalResult, nil)

				// BeginTx is called
				mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil).Times(1)

				// Limit check returns error via CheckLimits
				limitCheck.EXPECT().
					CheckLimits(gomock.Any(), mockTx, gomock.Any()).
					Return(nil, errors.New("limit check failed"))

				// Rollback is called by defer when error occurs
				mockTx.EXPECT().Rollback().Return(nil).AnyTimes()

				// Audit should NOT be inserted on error
				transactionValidationRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).Times(0)
				transactionValidationRepo.EXPECT().InsertWithTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

				return mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			// Create channel to signal when audit is done (for deterministic waiting)
			persistDone := make(chan struct{})

			txBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter := tt.setupMocks(ctrl, persistDone)

			service, err := NewValidationService(txBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
			require.NoError(t, err)

			// Create context - cancelled for context cancellation test
			ctx := context.Background()
			if tt.cancelContext {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel() // Cancel immediately
			}

			result, err := service.Validate(ctx, tt.request)

			// Wait for async audit goroutine to complete (if audit was expected)
			// Audit is only inserted for successful validations (non-error cases)
			if !tt.expectError {
				select {
				case <-persistDone:
					// Audit completed
				case <-time.After(1 * time.Second):
					t.Fatal("Timed out waiting for audit to complete")
				}
			}

			if tt.expectError {
				require.Error(t, err)

				if tt.expectedErr != nil {
					require.ErrorIs(t, err, tt.expectedErr)
				}

				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedDecision, result.Response.Decision)
				assert.Contains(t, result.Response.Reason, tt.expectedReason)
			}
		})
	}
}

// TestValidateTransaction_AuditFieldsPopulated verifies that individual audit fields
// are correctly populated in the audit record for SOX/GLBA compliance.
func TestValidateTransaction_AuditFieldsPopulated(t *testing.T) {
	// Setup
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(100)
	accountID := testutil.MustDeterministicUUID(101)
	merchantID := testutil.MustDeterministicUUID(102)
	ruleID1 := testutil.MustDeterministicUUID(110)
	ruleID2 := testutil.MustDeterministicUUID(111)
	limitID := testutil.MustDeterministicUUID(120)
	subType := "credit"

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		SubType:              &subType,
		Amount:               decimal.RequireFromString("250"), // $250.00
		Currency:             "BRL",
		TransactionTimestamp: fixedTime,
		Account: model.AccountContext{
			ID:     accountID,
			Type:   "checking",
			Status: "active",
		},
		Merchant: &model.MerchantContext{
			ID:       merchantID,
			Category: "5411",
			Country:  "BR",
		},
		Metadata: map[string]any{
			"source": "mobile",
		},
	}

	ctrl := gomock.NewController(t)
	persistDone := make(chan struct{})

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

	// FindByRequestID returns nil (no existing record) - new request
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)

	// AuditWriter mock - expects RecordValidationEventWithTx call (transactional for ALLOW)
	auditWriter := mocks.NewMockAuditWriter(ctrl)
	auditWriter.EXPECT().RecordValidationEventWithTx(gomock.Any(), mockTx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// Rule evaluation returns ALLOW with matched rules
	evalResult, err := model.NewEvaluationResult(
		model.DecisionAllow,
		[]uuid.UUID{ruleID1},
		[]uuid.UUID{ruleID1, ruleID2},
		"Transaction allowed",
	)
	require.NoError(t, err)
	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil)

	// BeginTx is called
	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil).Times(1)

	// Limit check passes with usage details via CheckLimits
	limitOutput := &model.CheckLimitsOutput{
		Allowed: true,
		LimitUsageDetails: []model.LimitUsageDetail{
			{
				LimitID:      limitID,
				LimitAmount:  decimal.RequireFromString("1000"), // $1000.00
				Scope:        "account:" + limitID.String(),
				Period:       model.LimitTypeDaily,
				CurrentUsage: decimal.RequireFromString("250"),
				Exceeded:     false,
			},
		},
		ExceededLimitIDs: []uuid.UUID{},
	}
	limitCheck.EXPECT().
		CheckLimits(gomock.Any(), mockTx, gomock.Any()).
		Return(limitOutput, nil)

	// Capture the audit record to verify fields via InsertWithTx
	var capturedTV *model.TransactionValidation
	transactionValidationRepo.EXPECT().InsertWithTx(gomock.Any(), mockTx, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ pgdb.DB, audit *model.TransactionValidation) error {
			capturedTV = audit
			close(persistDone)
			return nil
		})

	// Commit is called after successful ALLOW
	mockTx.EXPECT().Commit().Return(nil).Times(1)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Act
	result, err := service.Validate(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Wait for audit to complete
	select {
	case <-persistDone:
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for audit to complete")
	}

	// Assert - Verify individual audit fields are populated
	require.NotNil(t, capturedTV, "Audit record should be captured")

	// Request fields
	assert.Equal(t, requestID, capturedTV.RequestID)
	assert.Equal(t, model.TransactionTypeCard, capturedTV.TransactionType)
	assert.Equal(t, &subType, capturedTV.SubType)
	assert.True(t, decimal.RequireFromString("250").Equal(capturedTV.Amount), "Amount should be 250")
	assert.Equal(t, "BRL", capturedTV.Currency)
	assert.Equal(t, fixedTime, capturedTV.TransactionTimestamp)

	// Account context
	assert.Equal(t, accountID, capturedTV.Account.ID)
	assert.Equal(t, "checking", capturedTV.Account.Type)
	assert.Equal(t, "active", capturedTV.Account.Status)

	// Merchant context
	require.NotNil(t, capturedTV.Merchant)
	assert.Equal(t, merchantID, capturedTV.Merchant.ID)
	assert.Equal(t, "5411", capturedTV.Merchant.Category)
	assert.Equal(t, "BR", capturedTV.Merchant.Country)

	// Metadata
	require.NotNil(t, capturedTV.Metadata)
	assert.Equal(t, "mobile", capturedTV.Metadata["source"])

	// Response fields (from EvaluationResult)
	assert.Equal(t, model.DecisionAllow, capturedTV.Decision)
	assert.Equal(t, "Transaction allowed", capturedTV.Reason)

	// Matched and evaluated rule IDs
	require.Len(t, capturedTV.MatchedRuleIDs, 1)
	assert.Equal(t, ruleID1, capturedTV.MatchedRuleIDs[0])
	require.Len(t, capturedTV.EvaluatedRuleIDs, 2)

	// Limit usage details
	require.Len(t, capturedTV.LimitUsageDetails, 1)
	assert.Equal(t, limitID, capturedTV.LimitUsageDetails[0].LimitID)
	assert.Equal(t, decimal.RequireFromString("1000").String(), capturedTV.LimitUsageDetails[0].LimitAmount.String())
	assert.Equal(t, model.LimitTypeDaily, capturedTV.LimitUsageDetails[0].Period)
	assert.Equal(t, decimal.RequireFromString("250").String(), capturedTV.LimitUsageDetails[0].CurrentUsage.String())
	assert.False(t, capturedTV.LimitUsageDetails[0].Exceeded)
}

func TestNewValidationService_NilDependencies(t *testing.T) {
	ctrl := gomock.NewController(t)

	validTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	validRuleEval := mocks.NewMockRuleEvaluator(ctrl)
	validLimitCheck := mocks.NewMockLimitChecker(ctrl)
	validTransactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	validTransactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	validAuditWriter := mocks.NewMockAuditWriter(ctrl)

	tests := []struct {
		name                           string
		txBeginner                     pgdb.TxBeginner
		ruleEval                       RuleEvaluator
		limitCheck                     LimitChecker
		transactionValidationRepo      command.TransactionValidationRepository
		transactionValidationQueryRepo query.TransactionValidationRepository
		auditWriter                    AuditWriter
		expectedErr                    error
	}{
		{
			name:                           "nil conn (TxBeginner)",
			txBeginner:                     nil,
			ruleEval:                       validRuleEval,
			limitCheck:                     validLimitCheck,
			transactionValidationRepo:      validTransactionValidationRepo,
			transactionValidationQueryRepo: validTransactionValidationQueryRepo,
			auditWriter:                    validAuditWriter,
			expectedErr:                    ErrNilConn,
		},
		{
			name:                           "nil rule evaluator",
			txBeginner:                     validTxBeginner,
			ruleEval:                       nil,
			limitCheck:                     validLimitCheck,
			transactionValidationRepo:      validTransactionValidationRepo,
			transactionValidationQueryRepo: validTransactionValidationQueryRepo,
			auditWriter:                    validAuditWriter,
			expectedErr:                    ErrNilRuleEvaluator,
		},
		{
			name:                           "nil limit checker",
			txBeginner:                     validTxBeginner,
			ruleEval:                       validRuleEval,
			limitCheck:                     nil,
			transactionValidationRepo:      validTransactionValidationRepo,
			transactionValidationQueryRepo: validTransactionValidationQueryRepo,
			auditWriter:                    validAuditWriter,
			expectedErr:                    ErrNilLimitChecker,
		},
		{
			name:                           "nil transaction validation repository",
			txBeginner:                     validTxBeginner,
			ruleEval:                       validRuleEval,
			limitCheck:                     validLimitCheck,
			transactionValidationRepo:      nil,
			transactionValidationQueryRepo: validTransactionValidationQueryRepo,
			auditWriter:                    validAuditWriter,
			expectedErr:                    ErrNilTransactionValidationRepo,
		},
		{
			name:                           "nil transaction validation query repository",
			txBeginner:                     validTxBeginner,
			ruleEval:                       validRuleEval,
			limitCheck:                     validLimitCheck,
			transactionValidationRepo:      validTransactionValidationRepo,
			transactionValidationQueryRepo: nil,
			auditWriter:                    validAuditWriter,
			expectedErr:                    ErrNilTransactionValidationQueryRepo,
		},
		{
			name:                           "nil audit writer",
			txBeginner:                     validTxBeginner,
			ruleEval:                       validRuleEval,
			limitCheck:                     validLimitCheck,
			transactionValidationRepo:      validTransactionValidationRepo,
			transactionValidationQueryRepo: validTransactionValidationQueryRepo,
			auditWriter:                    nil,
			expectedErr:                    ErrNilAuditWriter,
		},
		{
			name:                           "all valid dependencies",
			txBeginner:                     validTxBeginner,
			ruleEval:                       validRuleEval,
			limitCheck:                     validLimitCheck,
			transactionValidationRepo:      validTransactionValidationRepo,
			transactionValidationQueryRepo: validTransactionValidationQueryRepo,
			auditWriter:                    validAuditWriter,
			expectedErr:                    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewValidationService(tt.txBeginner, tt.ruleEval, tt.limitCheck, tt.transactionValidationRepo, tt.transactionValidationQueryRepo, tt.auditWriter, nil)

			if tt.expectedErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedErr)
				assert.Nil(t, service)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, service)
			}
		})
	}
}

func TestValidateTransactionValidation(t *testing.T) {
	t.Parallel()

	validID := testutil.MustDeterministicUUID(1)
	requestID := testutil.MustDeterministicUUID(2)
	accountID := testutil.MustDeterministicUUID(3)
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	// Helper to create a valid transaction validation record
	validTV := func() *model.TransactionValidation {
		return &model.TransactionValidation{
			ID:                   validID,
			RequestID:            requestID,
			TransactionType:      model.TransactionTypeCard,
			Amount:               decimal.RequireFromString("100"),
			Currency:             "USD",
			TransactionTimestamp: fixedTime,
			Account:              model.AccountContext{ID: accountID, Type: "checking", Status: "active"},
			EvaluationResult:     model.EvaluationResult{Decision: model.DecisionAllow},
			CreatedAt:            fixedTime,
		}
	}

	tests := []struct {
		name      string
		tv        *model.TransactionValidation
		wantError bool
		errMsg    string
	}{
		{
			name:      "valid transaction validation record",
			tv:        validTV(),
			wantError: false,
		},
		{
			name: "nil ID",
			tv: func() *model.TransactionValidation {
				v := validTV()
				v.ID = uuid.UUID{}
				return v
			}(),
			wantError: true,
			errMsg:    "transaction validation ID is nil",
		},
		{
			name: "invalid decision",
			tv: func() *model.TransactionValidation {
				v := validTV()
				v.Decision = model.Decision("INVALID")
				return v
			}(),
			wantError: true,
			errMsg:    "invalid decision",
		},
		{
			name: "nil request ID",
			tv: func() *model.TransactionValidation {
				v := validTV()
				v.RequestID = uuid.UUID{}
				return v
			}(),
			wantError: true,
			errMsg:    "transaction validation request ID is nil",
		},
		{
			name: "invalid transaction type",
			tv: func() *model.TransactionValidation {
				v := validTV()
				v.TransactionType = model.TransactionType("INVALID")
				return v
			}(),
			wantError: true,
			errMsg:    "invalid transaction type",
		},
		{
			name: "zero amount",
			tv: func() *model.TransactionValidation {
				v := validTV()
				v.Amount = decimal.RequireFromString("0")
				return v
			}(),
			wantError: true,
			errMsg:    "invalid amount",
		},
		{
			name: "negative amount",
			tv: func() *model.TransactionValidation {
				v := validTV()
				v.Amount = decimal.RequireFromString("-1")
				return v
			}(),
			wantError: true,
			errMsg:    "invalid amount",
		},
		{
			name: "empty currency",
			tv: func() *model.TransactionValidation {
				v := validTV()
				v.Currency = ""
				return v
			}(),
			wantError: true,
			errMsg:    "currency is empty",
		},
		{
			name: "zero transaction timestamp",
			tv: func() *model.TransactionValidation {
				v := validTV()
				v.TransactionTimestamp = time.Time{}
				return v
			}(),
			wantError: true,
			errMsg:    "transaction timestamp is zero",
		},
		{
			name: "nil account ID",
			tv: func() *model.TransactionValidation {
				v := validTV()
				v.Account.ID = uuid.UUID{}
				return v
			}(),
			wantError: true,
			errMsg:    "account ID is nil",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateTransactionValidation(tc.tv)

			if tc.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateTransactionValidation_AllRequiredRequestFields verifies that all required request fields
// are validated (ID, RequestID, TransactionType, Amount, Currency, Account.ID).
func TestValidateTransactionValidation_AllRequiredRequestFields(t *testing.T) {
	t.Parallel()

	validID := testutil.MustDeterministicUUID(1)
	requestID := testutil.MustDeterministicUUID(2)
	accountID := testutil.MustDeterministicUUID(3)
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	// Base valid record
	validTV := func() *model.TransactionValidation {
		return &model.TransactionValidation{
			ID:                   validID,
			RequestID:            requestID,
			TransactionType:      model.TransactionTypeCard,
			Amount:               decimal.RequireFromString("100"),
			Currency:             "USD",
			TransactionTimestamp: fixedTime,
			Account:              model.AccountContext{ID: accountID, Type: "checking", Status: "active"},
			EvaluationResult:     model.EvaluationResult{Decision: model.DecisionAllow},
			CreatedAt:            fixedTime,
		}
	}

	// Test that each required field triggers validation error when missing/invalid
	requiredFields := []struct {
		name      string
		modify    func(*model.TransactionValidation)
		errSubstr string
	}{
		{
			name: "ID is required",
			modify: func(tv *model.TransactionValidation) {
				tv.ID = uuid.UUID{}
			},
			errSubstr: "transaction validation ID is nil",
		},
		{
			name: "RequestID is required",
			modify: func(tv *model.TransactionValidation) {
				tv.RequestID = uuid.UUID{}
			},
			errSubstr: "transaction validation request ID is nil",
		},
		{
			name: "TransactionType is required and must be valid",
			modify: func(tv *model.TransactionValidation) {
				tv.TransactionType = ""
			},
			errSubstr: "invalid transaction type",
		},
		{
			name: "Amount must be positive",
			modify: func(tv *model.TransactionValidation) {
				tv.Amount = decimal.RequireFromString("0")
			},
			errSubstr: "invalid amount",
		},
		{
			name: "Currency is required",
			modify: func(tv *model.TransactionValidation) {
				tv.Currency = ""
			},
			errSubstr: "currency is empty",
		},
		{
			name: "TransactionTimestamp is required",
			modify: func(tv *model.TransactionValidation) {
				tv.TransactionTimestamp = time.Time{}
			},
			errSubstr: "transaction timestamp is zero",
		},
		{
			name: "Account.ID is required",
			modify: func(tv *model.TransactionValidation) {
				tv.Account.ID = uuid.UUID{}
			},
			errSubstr: "account ID is nil",
		},
	}

	for _, tc := range requiredFields {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tv := validTV()
			tc.modify(tv)

			err := validateTransactionValidation(tv)

			require.Error(t, err, "Expected validation error for: %s", tc.name)
			assert.Contains(t, err.Error(), tc.errSubstr)
		})
	}

	// Verify valid record passes all validations
	t.Run("all required fields present passes validation", func(t *testing.T) {
		t.Parallel()

		tv := validTV()
		err := validateTransactionValidation(tv)
		require.NoError(t, err)
	})
}

// TestValidateTransactionValidation_AllRequiredResponseFields verifies that all required response fields
// are validated (Decision must be valid).
func TestValidateTransactionValidation_AllRequiredResponseFields(t *testing.T) {
	t.Parallel()

	validID := testutil.MustDeterministicUUID(1)
	requestID := testutil.MustDeterministicUUID(2)
	accountID := testutil.MustDeterministicUUID(3)
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	// Base valid record
	validTV := func() *model.TransactionValidation {
		return &model.TransactionValidation{
			ID:                   validID,
			RequestID:            requestID,
			TransactionType:      model.TransactionTypeCard,
			Amount:               decimal.RequireFromString("100"),
			Currency:             "USD",
			TransactionTimestamp: fixedTime,
			Account:              model.AccountContext{ID: accountID, Type: "checking", Status: "active"},
			EvaluationResult:     model.EvaluationResult{Decision: model.DecisionAllow},
			CreatedAt:            fixedTime,
		}
	}

	// Test invalid decision values
	invalidDecisions := []struct {
		name     string
		decision model.Decision
	}{
		{name: "empty decision", decision: ""},
		{name: "invalid decision UNKNOWN", decision: "UNKNOWN"},
		{name: "invalid decision lowercase", decision: "allow"},
		{name: "invalid decision typo", decision: "ALOW"},
	}

	for _, tc := range invalidDecisions {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tv := validTV()
			tv.Decision = tc.decision

			err := validateTransactionValidation(tv)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid decision")
		})
	}
}

// TestValidateTransactionValidation_AllValidDecisions verifies all valid decision values pass validation.
func TestValidateTransactionValidation_AllValidDecisions(t *testing.T) {
	t.Parallel()

	validID := testutil.MustDeterministicUUID(1)
	requestID := testutil.MustDeterministicUUID(2)
	accountID := testutil.MustDeterministicUUID(3)
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	validDecisions := []model.Decision{model.DecisionAllow, model.DecisionDeny, model.DecisionReview}

	for _, decision := range validDecisions {
		t.Run(string(decision), func(t *testing.T) {
			t.Parallel()

			audit := &model.TransactionValidation{
				ID:                   validID,
				RequestID:            requestID,
				TransactionType:      model.TransactionTypeCard,
				Amount:               decimal.RequireFromString("100"),
				Currency:             "USD",
				TransactionTimestamp: fixedTime,
				Account:              model.AccountContext{ID: accountID, Type: "checking", Status: "active"},
				EvaluationResult:     model.EvaluationResult{Decision: decision},
				CreatedAt:            fixedTime,
			}

			err := validateTransactionValidation(audit)

			require.NoError(t, err)
		})
	}
}

// TestValidate_TransactionValidationPersistenceSuccess verifies that when validation
// succeeds, the transaction validation record is persisted correctly via the repository.
func TestValidate_TransactionValidationPersistenceSuccess(t *testing.T) {
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	ruleID := testutil.MustDeterministicUUID(10)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

	// FindByRequestID returns nil (no existing record) - new request
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)

	// AuditWriter mock - expects RecordValidationEventWithTx call (transactional for ALLOW)
	auditWriter := mocks.NewMockAuditWriter(ctrl)
	auditWriter.EXPECT().RecordValidationEventWithTx(gomock.Any(), mockTx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// Rule evaluation returns ALLOW
	evalResult, err := model.NewEvaluationResult(
		model.DecisionAllow,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"Transaction allowed",
	)
	require.NoError(t, err)
	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil)

	// BeginTx is called
	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil).Times(1)

	// Limit check passes via CheckLimits
	limitOutput := &model.CheckLimitsOutput{
		Allowed:           true,
		LimitUsageDetails: []model.LimitUsageDetail{},
		ExceededLimitIDs:  []uuid.UUID{},
	}
	limitCheck.EXPECT().
		CheckLimits(gomock.Any(), mockTx, gomock.Any()).
		Return(limitOutput, nil)

	// InsertWithTx is called after validation passes
	transactionValidationRepo.EXPECT().InsertWithTx(gomock.Any(), mockTx, gomock.Any()).Times(1).DoAndReturn(
		func(_ context.Context, _ pgdb.DB, _ *model.TransactionValidation) error {
			return nil
		})

	// Commit is called after successful ALLOW
	mockTx.EXPECT().Commit().Return(nil).Times(1)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Act
	result, err := service.Validate(context.Background(), request)

	// Assert: Validation succeeds - the client gets the result regardless of internal logging
	require.NoError(t, err, "Validation should succeed")
	require.NotNil(t, result)
	assert.Equal(t, model.DecisionAllow, result.Response.Decision)

	// Note: We can't easily verify the log output in this test without injecting a mock logger.
	// The important behavior is that the validation result is returned correctly.
	// In a production system, we would verify logs through observability tooling.
}

// TestValidate_AuditPersistFailure_LogsError verifies that when the audit repository
// returns an error for non-transactional persistence (DENY-by-rule path),
// the error is logged but does not affect the validation result.
// Note: For ALLOW path, InsertWithTx failure now fails the entire validation.
func TestValidate_AuditPersistFailure_LogsError(t *testing.T) {
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	ruleID := testutil.MustDeterministicUUID(10)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)
	errorLogged := make(chan struct{})

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

	// FindByRequestID returns nil (no existing record) - new request
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)

	// AuditWriter mock - expects RecordValidationEvent call (non-transactional for DENY-by-rule)
	auditWriter := mocks.NewMockAuditWriter(ctrl)
	auditWriter.EXPECT().RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// Rule evaluation returns DENY (triggers non-transactional path)
	evalResult, err := model.NewEvaluationResult(
		model.DecisionDeny,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"Transaction blocked",
	)
	require.NoError(t, err)
	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil)

	// No BeginTx for DENY-by-rule
	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)

	// Audit repo returns error (simulating database failure) - non-transactional Insert
	transactionValidationRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *model.TransactionValidation) error {
			close(errorLogged)
			return errors.New("database connection failed")
		})

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Act
	result, err := service.Validate(context.Background(), request)

	// Assert: Validation succeeds despite audit failure (non-transactional persistence is best-effort)
	require.NoError(t, err, "Validation should succeed even if audit fails for DENY-by-rule")
	require.NotNil(t, result)
	assert.Equal(t, model.DecisionDeny, result.Response.Decision)

	// Wait for audit error to be processed
	select {
	case <-errorLogged:
		// Audit error was processed
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for audit error to be processed")
	}
}

// TestValidate_WithSegmentAndPortfolio verifies that segment and portfolio context
// are correctly included in the audit event for SOX/GLBA compliance.
func TestValidate_WithSegmentAndPortfolio(t *testing.T) {
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	segmentID := testutil.MustDeterministicUUID(3)
	portfolioID := testutil.MustDeterministicUUID(4)
	ruleID := testutil.MustDeterministicUUID(10)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account: model.AccountContext{
			ID:     accountID,
			Type:   "checking",
			Status: "active",
		},
		Segment: &model.SegmentContext{
			ID:       segmentID,
			Name:     "Premium",
			Metadata: map[string]any{"tier": "gold"},
		},
		Portfolio: &model.PortfolioContext{
			ID:       portfolioID,
			Name:     "Investment Portfolio",
			Metadata: map[string]any{"type": "investment"},
		},
	}

	ctrl := gomock.NewController(t)
	persistDone := make(chan struct{})

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

	// FindByRequestID returns nil (no existing record) - new request
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)

	// AuditWriter mock - expects RecordValidationEventWithTx call with segment and portfolio
	auditWriter := mocks.NewMockAuditWriter(ctrl)
	auditWriter.EXPECT().RecordValidationEventWithTx(
		gomock.Any(),
		mockTx,
		gomock.Any(),
		gomock.Any(), // Request snapshot should contain segment and portfolio
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ pgdb.DB, _ uuid.UUID, snapshot map[string]any, _ model.EvaluationResult, _ model.ValidationResponseContext) error {
		// Verify segment is present
		segmentData, ok := snapshot["segment"].(map[string]any)
		assert.True(t, ok, "Segment should be in snapshot")
		assert.Equal(t, segmentID.String(), segmentData["segmentId"])
		assert.Equal(t, "Premium", segmentData["name"])

		// Verify portfolio is present
		portfolioData, ok := snapshot["portfolio"].(map[string]any)
		assert.True(t, ok, "Portfolio should be in snapshot")
		assert.Equal(t, portfolioID.String(), portfolioData["portfolioId"])
		assert.Equal(t, "Investment Portfolio", portfolioData["name"])

		// Verify account contains segmentId and portfolioId
		accountData, ok := snapshot["account"].(map[string]any)
		assert.True(t, ok, "Account should be in snapshot")
		assert.Equal(t, segmentID.String(), accountData["segmentId"])
		assert.Equal(t, portfolioID.String(), accountData["portfolioId"])

		return nil
	}).Times(1)

	// Rule evaluation returns ALLOW
	evalResult, err := model.NewEvaluationResult(
		model.DecisionAllow,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"Transaction allowed",
	)
	require.NoError(t, err)
	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil)

	// BeginTx is called
	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil).Times(1)

	// Limit check passes via CheckLimits
	limitOutput := &model.CheckLimitsOutput{
		Allowed:           true,
		LimitUsageDetails: []model.LimitUsageDetail{},
		ExceededLimitIDs:  []uuid.UUID{},
	}
	limitCheck.EXPECT().
		CheckLimits(gomock.Any(), mockTx, gomock.Any()).
		Return(limitOutput, nil)

	// Transaction validation persisted successfully via InsertWithTx
	transactionValidationRepo.EXPECT().InsertWithTx(gomock.Any(), mockTx, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ pgdb.DB, tv *model.TransactionValidation) error {
			// Verify segment and portfolio are persisted
			assert.NotNil(t, tv.Segment)
			assert.Equal(t, segmentID, tv.Segment.ID)
			assert.NotNil(t, tv.Portfolio)
			assert.Equal(t, portfolioID, tv.Portfolio.ID)
			close(persistDone)
			return nil
		})

	// Commit is called after successful ALLOW
	mockTx.EXPECT().Commit().Return(nil).Times(1)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Act
	result, err := service.Validate(context.Background(), request)

	// Wait for persistence to complete
	select {
	case <-persistDone:
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for persistence to complete")
	}

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.DecisionAllow, result.Response.Decision)
}

// TestValidate_NilRequest verifies that Validate returns an error when called with nil request.
func TestValidate_NilRequest(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)

	// No mock expectations - function should return early (nil check before FindByRequestID)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Act
	result, err := service.Validate(context.Background(), nil)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation request cannot be nil")
	assert.Nil(t, result)
}

// TestValidate_RuleEvaluatorReturnsNil verifies that Validate handles nil evaluation result.
func TestValidate_RuleEvaluatorReturnsNil(t *testing.T) {
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

	// FindByRequestID returns nil (no existing record) - new request
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)
	auditWriter := mocks.NewMockAuditWriter(ctrl)

	// Rule evaluation returns nil result (but no error)
	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(nil, nil)

	// No limit check or audit expected - should fail early

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Act
	result, err := service.Validate(context.Background(), request)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rule evaluation returned nil result")
	assert.Nil(t, result)
}

// TestValidate_LimitCheckerReturnsNil verifies that Validate handles nil limit check result.
func TestValidate_LimitCheckerReturnsNil(t *testing.T) {
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	ruleID := testutil.MustDeterministicUUID(10)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

	// FindByRequestID returns nil (no existing record) - new request
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)
	auditWriter := mocks.NewMockAuditWriter(ctrl)

	// Rule evaluation returns ALLOW
	evalResult, err := model.NewEvaluationResult(
		model.DecisionAllow,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"Transaction allowed",
	)
	require.NoError(t, err)
	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil)

	// BeginTx is called
	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil).Times(1)

	// Limit check returns nil result (but no error) via CheckLimits
	limitCheck.EXPECT().
		CheckLimits(gomock.Any(), mockTx, gomock.Any()).
		Return(nil, nil)

	// Rollback is called by defer when nil result is returned
	mockTx.EXPECT().Rollback().Return(nil).AnyTimes()

	// No audit expected - should fail early

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Act
	result, err := service.Validate(context.Background(), request)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "limit check returned nil result")
	assert.Nil(t, result)
}

// TestValidate_AuditEventWriterFailure verifies audit writer errors are logged but don't fail validation
// for non-transactional paths (DENY-by-rule).
// Note: For ALLOW path, RecordValidationEventWithTx failure now fails the entire validation.
func TestValidate_AuditEventWriterFailure(t *testing.T) {
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	ruleID := testutil.MustDeterministicUUID(10)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)
	persistDone := make(chan struct{})

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

	// FindByRequestID returns nil (no existing record) - new request
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)
	auditWriter := mocks.NewMockAuditWriter(ctrl)

	// AuditWriter returns error (non-transactional for DENY-by-rule)
	auditWriter.EXPECT().RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("audit writer failure")).Times(1)

	// Rule evaluation returns DENY (triggers non-transactional path)
	evalResult, err := model.NewEvaluationResult(
		model.DecisionDeny,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"Transaction blocked",
	)
	require.NoError(t, err)
	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil)

	// No BeginTx for DENY-by-rule
	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)

	// Transaction validation persisted successfully (non-transactional)
	transactionValidationRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *model.TransactionValidation) error {
			close(persistDone)
			return nil
		})

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Act
	result, err := service.Validate(context.Background(), request)

	// Wait for persistence to complete
	select {
	case <-persistDone:
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for persistence to complete")
	}

	// Assert: Validation succeeds despite audit writer failure (best-effort audit for DENY-by-rule)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.DecisionDeny, result.Response.Decision)
}

// TestValidationService_Validate_NilMetricsFactory_DoesNotPanic verifies that
// persistTransactionValidation completes without panic when metricsFactory
// is nil in the context (no metrics configured). The nil guard in
// persistTransactionValidation must prevent a nil-pointer dereference when
// the Insert call fails and the code attempts to emit MetricAuditPersistFailures.
func TestValidationService_Validate_NilMetricsFactory_DoesNotPanic(t *testing.T) {
	t.Parallel()

	fixedTime := testutil.FixedTime()
	requestID := testutil.MustDeterministicUUID(200)
	accountID := testutil.MustDeterministicUUID(201)
	ruleID := testutil.MustDeterministicUUID(210)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)

	// FindByRequestID returns nil (no existing record) - new request
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)

	// AuditWriter mock - expects RecordValidationEvent call (non-transactional for DENY-by-rule)
	auditWriter.EXPECT().RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// Rule evaluation returns DENY to trigger the non-transactional persist path
	evalResult, err := model.NewEvaluationResult(
		model.DecisionDeny,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"Transaction blocked",
	)
	require.NoError(t, err)
	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil)

	// No transaction started for DENY-by-rule
	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	limitCheck.EXPECT().CheckLimits(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	// Insert FAILS — this triggers the metricsFactory nil-guard path.
	// Persist is synchronous in the DENY-by-rule path, so gomock verifies the call.
	transactionValidationRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).
		Return(errors.New("database connection refused"))

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Use context.Background() which has NO metricsFactory injected.
	// This means libObservability.NewTrackingFromContext(ctx) returns nil for metricsFactory.
	// The nil guard in persistTransactionValidation must prevent a panic.
	var result *ValidateResult
	var validateErr error

	assert.NotPanics(t, func() {
		result, validateErr = service.Validate(context.Background(), request)
	})

	// Validation itself should succeed (persist failure is non-fatal)
	require.NoError(t, validateErr)
	require.NotNil(t, result)
	assert.Equal(t, model.DecisionDeny, result.Response.Decision)
	assert.False(t, result.IsDuplicate)
}

// TestPersistAuditEvent_UsesDetachedContext verifies that persistAuditEvent creates a
// detached context (derived from context.Background()) with a timeout, rather than
// passing the original request context to RecordValidationEvent.
// This ensures audit persistence completes even if the request context is cancelled.
func TestPersistAuditEvent_UsesDetachedContext(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	fixedTime := testutil.FixedTime()
	requestID := testutil.MustDeterministicUUID(400)
	accountID := testutil.MustDeterministicUUID(401)
	ruleID := testutil.MustDeterministicUUID(410)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)

	// FindByRequestID returns nil (no existing record) - new request
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)

	// Rule evaluation returns DENY to trigger non-transactional persistAuditEvent
	evalResult, err := model.NewEvaluationResult(
		model.DecisionDeny,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"Transaction blocked",
	)
	require.NoError(t, err)
	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil)

	// No transaction for DENY-by-rule
	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	limitCheck.EXPECT().CheckLimits(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	// persistTransactionValidation succeeds
	transactionValidationRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).Return(nil)

	// Capture the context passed to RecordValidationEvent
	var capturedCtx context.Context
	auditWriter.EXPECT().
		RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ uuid.UUID, _ map[string]any, _ model.EvaluationResult, _ model.ValidationResponseContext) error {
			capturedCtx = ctx
			require.NoError(t, ctx.Err(), "persist context should be detached from parent cancellation")
			return nil
		}).
		Times(1)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	result, err := service.Validate(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.DecisionDeny, result.Response.Decision)

	// The captured context should have a deadline (from context.WithTimeout on context.Background())
	// persistAuditEvent now uses a detached persistCtx, so capturedCtx must NOT be the original request ctx
	require.NotNil(t, capturedCtx, "RecordValidationEvent should have been called")
	_, hasDeadline := capturedCtx.Deadline()
	assert.True(t, hasDeadline, "persistAuditEvent should use a detached context with timeout (context.WithTimeout), not the original request context")
}

// TestPersistAuditEvent_NilMetricsFactory_DoesNotPanicOnFailure verifies that when
// RecordValidationEvent fails and metricsFactory is nil, the service does not
// panic. This is a nil-safety check.
func TestPersistAuditEvent_NilMetricsFactory_DoesNotPanicOnFailure(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	fixedTime := testutil.FixedTime()
	requestID := testutil.MustDeterministicUUID(500)
	accountID := testutil.MustDeterministicUUID(501)
	ruleID := testutil.MustDeterministicUUID(510)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)

	// FindByRequestID returns nil (no existing record) - new request
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)

	// Rule evaluation returns DENY to trigger non-transactional persistAuditEvent
	evalResult, err := model.NewEvaluationResult(
		model.DecisionDeny,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"Transaction blocked",
	)
	require.NoError(t, err)
	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil)

	// No transaction for DENY-by-rule
	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	limitCheck.EXPECT().CheckLimits(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	// persistTransactionValidation succeeds
	transactionValidationRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).Return(nil)

	// AuditWriter FAILS — this should trigger metric increment
	auditWriter.EXPECT().
		RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("audit write failed")).
		Times(1)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Use context.Background() which has NO metricsFactory.
	// persistAuditEvent should handle nil metricsFactory without panic.
	var result *ValidateResult
	var validateErr error

	assert.NotPanics(t, func() {
		result, validateErr = service.Validate(context.Background(), request)
	})

	// Validation itself should succeed (audit failure is best-effort)
	require.NoError(t, validateErr)
	require.NotNil(t, result)
	assert.Equal(t, model.DecisionDeny, result.Response.Decision)
}

// TestPersistAuditEvent_LogFieldIsErrorMessage verifies that when
// RecordValidationEvent fails, the error log uses field name "error.message"
// (not "error") for consistency with persistTransactionValidation.
func TestPersistAuditEvent_LogFieldIsErrorMessage(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	fixedTime := testutil.FixedTime()
	requestID := testutil.MustDeterministicUUID(600)
	accountID := testutil.MustDeterministicUUID(601)
	ruleID := testutil.MustDeterministicUUID(610)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	// Only need auditWriter mock and a minimal service for direct persistAuditEvent call
	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)

	// AuditWriter FAILS — to trigger the error logging path
	auditWriter.EXPECT().
		RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("audit write failed")).
		Times(1)

	// Use MockLogger to capture log output and verify field name
	mockLogger := testutil.NewMockLogger()

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Call persistAuditEvent directly to test the log field name.
	resp := model.NewValidationResponse(uuid.New(), requestID, model.DecisionDeny, fixedTime)
	evalResultForResp, err2 := model.NewEvaluationResult(
		model.DecisionDeny,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"Transaction blocked",
	)
	require.NoError(t, err2)
	resp.EvaluationResult = *evalResultForResp

	service.persistAuditEvent(context.Background(), request, resp, mockLogger)

	// Find the error log call
	require.NotEmpty(t, mockLogger.Calls, "Expected at least one log call from persistAuditEvent failure")

	var foundErrorLog bool
	for _, call := range mockLogger.Calls {
		if call.Message == "failed to persist audit event" {
			foundErrorLog = true
			fields := testutil.FieldsToMap(call.Fields)
			// The field MUST be "error.message", not "error"
			errMsgVal, hasErrorMessage := fields["error.message"]
			_, hasError := fields["error"]
			assert.True(t, hasErrorMessage, "Log field should be 'error.message', not 'error'")
			assert.False(t, hasError, "Log field 'error' should not exist; should be 'error.message'")
			assert.Equal(t, "audit write failed", errMsgVal, "error.message field value should match the error returned by the mock")
		}
	}
	assert.True(t, foundErrorLog, "Expected a log call with message 'failed to persist audit event'")
}

// TestPersistAuditEvent_SucceedsWithCancelledParentContext verifies that
// persistAuditEvent succeeds even when the parent context has been cancelled
// before calling persistAuditEvent. The detached context (context.Background()
// with timeout) should isolate audit persistence from parent cancellation.
func TestPersistAuditEvent_SucceedsWithCancelledParentContext(t *testing.T) {
	t.Parallel()
	testutil.SetupTestTracing(t)

	fixedTime := testutil.FixedTime()
	requestID := testutil.MustDeterministicUUID(700)
	accountID := testutil.MustDeterministicUUID(701)
	ruleID := testutil.MustDeterministicUUID(710)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)

	// AuditWriter succeeds — gomock Times(1) verifies it was called despite cancelled parent
	auditWriter.EXPECT().
		RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ uuid.UUID, _ map[string]any, _ model.EvaluationResult, _ model.ValidationResponseContext) error {
			require.NoError(t, ctx.Err(), "persist context should be detached from the canceled parent")
			_, hasDeadline := ctx.Deadline()
			require.True(t, hasDeadline, "persist context should keep the timeout budget")
			return nil
		}).
		Times(1)

	// Use MockLogger to capture log output
	mockLogger := testutil.NewMockLogger()

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Build a response for persistAuditEvent (same pattern as LogFieldIsErrorMessage test)
	resp := model.NewValidationResponse(uuid.New(), requestID, model.DecisionDeny, fixedTime)
	evalResult, err := model.NewEvaluationResult(
		model.DecisionDeny,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"Transaction blocked",
	)
	require.NoError(t, err)
	resp.EvaluationResult = *evalResult

	// Create a context and cancel it BEFORE calling persistAuditEvent
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Call persistAuditEvent directly with the cancelled context
	service.persistAuditEvent(ctx, request, resp, mockLogger)

	// gomock verifies RecordValidationEvent was called exactly once (Times(1)).
	// This proves persistAuditEvent completes the audit write despite the parent
	// context being cancelled — the core guarantee of the detached context pattern.
}
