// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
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
	queryMocks "github.com/LerianStudio/midaz/v3/components/tracer/internal/services/query/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// TestValidationService_Constructor_AcceptsTxBeginner verifies that NewValidationService
// accepts a TxBeginner parameter to enable transactional validation flows.

func TestValidationService_Constructor_AcceptsTxBeginner(t *testing.T) {
	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	// Create mock dependencies
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)

	// Create mock TxBeginner - this is now accepted by the constructor
	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	service, err := NewValidationService(
		mockTxBeginner,
		ruleEval,
		limitCheck,
		transactionValidationRepo,
		transactionValidationQueryRepo,
		auditWriter,
		nil, // clock
	)
	require.NoError(t, err)
	require.NotNil(t, service)
}

// TestValidationService_Validate_Allow_UsesTransaction verifies that on ALLOW path,
// the service begins a transaction, calls CheckLimits with the tx, calls
// InsertValidationWithTx, and commits.
func TestValidationService_Validate_Allow_UsesTransaction(t *testing.T) {
	testutil.SetupTestTracing(t)

	// Common test fixtures
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	ruleID := testutil.MustDeterministicUUID(10)
	limitID := testutil.MustDeterministicUUID(20)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	// Create mock dependencies
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)
	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	// Track if InsertWithTx was called
	insertWithTxCalled := false

	// Expected flow for ALLOW path with transactions:
	// 1. FindByRequestID (idempotency check) - no existing validation
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, nil).
		Times(1)

	// 2. Rule evaluation returns ALLOW
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

	// 3. BeginTx is called
	mockTxBeginner.EXPECT().
		BeginTx(gomock.Any(), gomock.Any()).
		Return(mockTx, nil).
		Times(1)

	// 4. CheckLimits is called with the transaction
	limitOutput := &model.CheckLimitsOutput{
		Allowed: true,
		LimitUsageDetails: []model.LimitUsageDetail{
			{
				LimitID:      limitID,
				LimitAmount:  decimal.RequireFromString("1000"),
				CurrentUsage: decimal.RequireFromString("100"),
				Exceeded:     false,
			},
		},
		ExceededLimitIDs: []uuid.UUID{},
	}
	limitCheck.EXPECT().
		CheckLimits(gomock.Any(), mockTx, gomock.Any()).
		Return(limitOutput, nil).
		Times(1)

	// 5. InsertWithTx is called with the transaction
	transactionValidationRepo.EXPECT().
		InsertWithTx(gomock.Any(), mockTx, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ pgdb.DB, _ *model.TransactionValidation) error {
			insertWithTxCalled = true
			return nil
		}).
		Times(1)

	// 6. RecordValidationEventWithTx is called with the transaction
	auditWriter.EXPECT().
		RecordValidationEventWithTx(gomock.Any(), mockTx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	// 7. Commit is called
	mockTx.EXPECT().
		Commit().
		Return(nil).
		Times(1)

	// Create service with TxBeginner
	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Act
	result, err := service.Validate(context.Background(), request)

	// Assert: Validation succeeds
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.DecisionAllow, result.Response.Decision)

	assert.True(t, insertWithTxCalled, "Validate should call InsertWithTx with transaction for atomic persistence")
}

// TestValidationService_Validate_DenyByLimit_RollsBackCounters verifies that when
// a limit is exceeded, tx.Rollback() is called to undo counter increments,
// and a separate non-transactional path is used for validation+audit persistence.
func TestValidationService_Validate_DenyByLimit_RollsBackCounters(t *testing.T) {
	testutil.SetupTestTracing(t)

	// Common test fixtures
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	ruleID := testutil.MustDeterministicUUID(10)
	limitID := testutil.MustDeterministicUUID(20)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("600"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	// Create mock dependencies
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)
	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	// Track if tx.Rollback was called
	rollbackCalled := false

	// Expected flow for DENY-by-limit with transactions:
	// 1. FindByRequestID (idempotency check) - no existing validation
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, nil).
		Times(1)

	// 2. Rule evaluation returns ALLOW (so we proceed to limit check)
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

	// 3. BeginTx is called
	mockTxBeginner.EXPECT().
		BeginTx(gomock.Any(), gomock.Any()).
		Return(mockTx, nil).
		Times(1)

	// 4. CheckLimits returns limit exceeded
	limitOutput := &model.CheckLimitsOutput{
		Allowed: false,
		LimitUsageDetails: []model.LimitUsageDetail{
			{
				LimitID:      limitID,
				LimitAmount:  decimal.RequireFromString("500"),
				CurrentUsage: decimal.RequireFromString("600"),
				Exceeded:     true,
			},
		},
		ExceededLimitIDs: []uuid.UUID{limitID},
	}
	limitCheck.EXPECT().
		CheckLimits(gomock.Any(), mockTx, gomock.Any()).
		Return(limitOutput, nil).
		Times(1)

	// 5. tx.Rollback is called to undo counter increments
	mockTx.EXPECT().
		Rollback().
		DoAndReturn(func() error {
			rollbackCalled = true
			return nil
		}).
		Times(1)

	// 6. Non-transactional Insert is called (outside tx)
	transactionValidationRepo.EXPECT().
		Insert(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	// 7. Non-transactional audit is written
	auditWriter.EXPECT().
		RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	// Create service with TxBeginner
	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Act
	result, err := service.Validate(context.Background(), request)

	// Assert: Validation returns DENY with limit_exceeded
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.DecisionDeny, result.Response.Decision)
	assert.Equal(t, "limit_exceeded", result.Response.Reason)

	assert.True(t, rollbackCalled, "DENY-by-limit path should use tx.Rollback() to undo counter increments")
}

// TestValidationService_Validate_Review_RollsBackCounters verifies that on REVIEW,
// counters are rolled back via tx.Rollback() (not RollbackUsage).
func TestValidationService_Validate_Review_RollsBackCounters(t *testing.T) {
	testutil.SetupTestTracing(t)

	// Common test fixtures
	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	ruleID := testutil.MustDeterministicUUID(10)
	limitID := testutil.MustDeterministicUUID(20)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	// Create mock dependencies
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)
	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	// Track if tx.Rollback was called
	rollbackCalled := false

	// Expected flow for REVIEW with transactions:
	// 1. FindByRequestID (idempotency check) - no existing validation
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, nil).
		Times(1)

	// 2. Rule evaluation returns REVIEW
	evalResult, err := model.NewEvaluationResult(
		model.DecisionReview,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"Transaction requires review",
	)
	require.NoError(t, err)
	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil)

	// 3. BeginTx is called
	mockTxBeginner.EXPECT().
		BeginTx(gomock.Any(), gomock.Any()).
		Return(mockTx, nil).
		Times(1)

	// 4. CheckLimits passes (limits not exceeded)
	limitOutput := &model.CheckLimitsOutput{
		Allowed: true,
		LimitUsageDetails: []model.LimitUsageDetail{
			{
				LimitID:           limitID,
				LimitAmount:       decimal.RequireFromString("1000"),
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
		Return(limitOutput, nil).
		Times(1)

	// 5. tx.Rollback is called because REVIEW decision
	mockTx.EXPECT().
		Rollback().
		DoAndReturn(func() error {
			rollbackCalled = true
			return nil
		}).
		Times(1)

	// 6. Non-transactional Insert is called (outside tx)
	transactionValidationRepo.EXPECT().
		Insert(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	// 7. Non-transactional audit written
	auditWriter.EXPECT().
		RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	// Create service with TxBeginner
	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Act
	result, err := service.Validate(context.Background(), request)

	// Assert: Validation returns REVIEW
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.DecisionReview, result.Response.Decision)
	assert.Equal(t, "Transaction requires review", result.Response.Reason)

	assert.True(t, rollbackCalled, "REVIEW path should use tx.Rollback() to undo counter increments")
}

// TestValidationService_Validate_ConcurrentDuplicate_ReturnsCachedResponse verifies that
// when two concurrent requests with the same request_id race past the idempotency check,
// the second request detects the unique constraint violation and returns the cached response.
func TestValidationService_Validate_ConcurrentDuplicate_ReturnsCachedResponse(t *testing.T) {
	testutil.SetupTestTracing(t)

	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	validationID := testutil.MustDeterministicUUID(3)
	ruleID := testutil.MustDeterministicUUID(10)
	limitID := testutil.MustDeterministicUUID(20)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)
	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	// 1. FindByRequestID returns nil (race: both requests pass idempotency check)
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, nil).
		Times(1)

	// 2. Rule evaluation returns ALLOW
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

	// 3. BeginTx
	mockTxBeginner.EXPECT().
		BeginTx(gomock.Any(), gomock.Any()).
		Return(mockTx, nil)

	// 4. CheckLimits passes
	limitOutput := &model.CheckLimitsOutput{
		Allowed: true,
		LimitUsageDetails: []model.LimitUsageDetail{
			{
				LimitID:      limitID,
				LimitAmount:  decimal.RequireFromString("1000"),
				CurrentUsage: decimal.RequireFromString("100"),
				Exceeded:     false,
			},
		},
		ExceededLimitIDs: []uuid.UUID{},
	}
	limitCheck.EXPECT().
		CheckLimits(gomock.Any(), mockTx, gomock.Any()).
		Return(limitOutput, nil)

	// 5. InsertWithTx returns ErrDuplicateValidation (concurrent request already inserted)
	transactionValidationRepo.EXPECT().
		InsertWithTx(gomock.Any(), mockTx, gomock.Any()).
		Return(fmt.Errorf("%w: request_id %s", command.ErrDuplicateValidation, requestID))

	// 6. tx.Rollback() called by defer (tx is poisoned after unique violation)
	mockTx.EXPECT().
		Rollback().
		Return(nil)

	// 7. FindByRequestID retried - returns the existing record from the other request
	existingValidation, err := model.NewTransactionValidation(validationID, model.DecisionAllow, fixedTime)
	require.NoError(t, err)
	existingValidation.RequestID = requestID
	existingValidation.Amount = decimal.RequireFromString("100")
	existingValidation.Currency = "USD"
	existingValidation.Account = model.AccountContext{ID: accountID}
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(existingValidation, nil).
		Times(1)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	result, err := service.Validate(context.Background(), request)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsDuplicate, "Concurrent duplicate should return IsDuplicate=true")
	assert.Equal(t, validationID, result.Response.ValidationID)
}

// TestValidationService_Validate_ConcurrentDuplicate_FindByRequestIDFails verifies that
// when InsertWithTx returns ErrDuplicateValidation but the retry FindByRequestID also fails,
// the original error is propagated to the caller.
func TestValidationService_Validate_ConcurrentDuplicate_FindByRequestIDFails(t *testing.T) {
	testutil.SetupTestTracing(t)

	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	ruleID := testutil.MustDeterministicUUID(10)
	limitID := testutil.MustDeterministicUUID(20)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)
	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	// 1. Initial FindByRequestID - no duplicate (race window)
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, nil).
		Times(1)

	// 2. Rule evaluation returns ALLOW
	evalResult, err := model.NewEvaluationResult(model.DecisionAllow, []uuid.UUID{ruleID}, []uuid.UUID{ruleID}, "Transaction allowed")
	require.NoError(t, err)
	ruleEval.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(evalResult, nil)

	// 3. BeginTx
	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil)

	// 4. CheckLimits passes
	limitOutput := &model.CheckLimitsOutput{
		Allowed:           true,
		LimitUsageDetails: []model.LimitUsageDetail{{LimitID: limitID, LimitAmount: decimal.RequireFromString("1000"), CurrentUsage: decimal.RequireFromString("100"), Exceeded: false}},
		ExceededLimitIDs:  []uuid.UUID{},
	}
	limitCheck.EXPECT().CheckLimits(gomock.Any(), mockTx, gomock.Any()).Return(limitOutput, nil)

	// 5. InsertWithTx returns ErrDuplicateValidation
	transactionValidationRepo.EXPECT().
		InsertWithTx(gomock.Any(), mockTx, gomock.Any()).
		Return(fmt.Errorf("%w: request_id %s", command.ErrDuplicateValidation, requestID))

	// 6. Retry FindByRequestID FAILS
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, errors.New("database unavailable")).
		Times(1)

	// 7. Defer calls Rollback
	mockTx.EXPECT().Rollback().Return(nil)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	result, err := service.Validate(context.Background(), request)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to persist transaction validation")
}

// TestValidationService_Validate_BeginTxFailure verifies that when BeginTx fails,
// the error is properly propagated and no further processing occurs.
func TestValidationService_Validate_BeginTxFailure(t *testing.T) {
	testutil.SetupTestTracing(t)

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

	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)
	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)

	// 1. FindByRequestID - no duplicate
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, nil)

	// 2. Rule evaluation returns ALLOW (proceeds to tx path)
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

	// 3. BeginTx fails
	mockTxBeginner.EXPECT().
		BeginTx(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("connection refused"))

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	result, err := service.Validate(context.Background(), request)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to begin transaction")
	assert.Contains(t, err.Error(), "connection refused")
}

// TestValidationService_Validate_CommitFailure verifies that when tx.Commit() fails
// on the ALLOW path, the error is propagated and defer handles rollback.
func TestValidationService_Validate_CommitFailure(t *testing.T) {
	testutil.SetupTestTracing(t)

	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	ruleID := testutil.MustDeterministicUUID(10)
	limitID := testutil.MustDeterministicUUID(20)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)
	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	// 1. FindByRequestID - no duplicate
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, nil)

	// 2. Rule evaluation returns ALLOW
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

	// 3. BeginTx succeeds
	mockTxBeginner.EXPECT().
		BeginTx(gomock.Any(), gomock.Any()).
		Return(mockTx, nil)

	// 4. CheckLimits passes
	limitOutput := &model.CheckLimitsOutput{
		Allowed: true,
		LimitUsageDetails: []model.LimitUsageDetail{
			{
				LimitID:      limitID,
				LimitAmount:  decimal.RequireFromString("1000"),
				CurrentUsage: decimal.RequireFromString("100"),
				Exceeded:     false,
			},
		},
		ExceededLimitIDs: []uuid.UUID{},
	}
	limitCheck.EXPECT().
		CheckLimits(gomock.Any(), mockTx, gomock.Any()).
		Return(limitOutput, nil)

	// 5. InsertWithTx succeeds
	transactionValidationRepo.EXPECT().
		InsertWithTx(gomock.Any(), mockTx, gomock.Any()).
		Return(nil)

	// 6. RecordValidationEventWithTx succeeds
	auditWriter.EXPECT().
		RecordValidationEventWithTx(gomock.Any(), mockTx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	// 7. Commit FAILS
	mockTx.EXPECT().
		Commit().
		Return(errors.New("commit failed: serialization failure"))

	// 8. Defer calls Rollback (tx still non-nil after failed commit)
	mockTx.EXPECT().
		Rollback().
		Return(nil)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	result, err := service.Validate(context.Background(), request)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to commit transaction")
	assert.Contains(t, err.Error(), "serialization failure")
}

// TestValidationService_Validate_Allow_InsertWithTxFailure verifies that when
// InsertWithTx fails with a non-duplicate error on the ALLOW path,
// the error is propagated and defer handles rollback.
func TestValidationService_Validate_Allow_InsertWithTxFailure(t *testing.T) {
	testutil.SetupTestTracing(t)

	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	ruleID := testutil.MustDeterministicUUID(10)
	limitID := testutil.MustDeterministicUUID(20)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)
	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, nil)

	evalResult, err := model.NewEvaluationResult(model.DecisionAllow, []uuid.UUID{ruleID}, []uuid.UUID{ruleID}, "Transaction allowed")
	require.NoError(t, err)
	ruleEval.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(evalResult, nil)

	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil)

	limitOutput := &model.CheckLimitsOutput{
		Allowed:           true,
		LimitUsageDetails: []model.LimitUsageDetail{{LimitID: limitID, LimitAmount: decimal.RequireFromString("1000"), CurrentUsage: decimal.RequireFromString("100"), Exceeded: false}},
		ExceededLimitIDs:  []uuid.UUID{},
	}
	limitCheck.EXPECT().CheckLimits(gomock.Any(), mockTx, gomock.Any()).Return(limitOutput, nil)

	// InsertWithTx fails with a non-duplicate DB error
	transactionValidationRepo.EXPECT().
		InsertWithTx(gomock.Any(), mockTx, gomock.Any()).
		Return(errors.New("disk full"))

	// Defer calls Rollback
	mockTx.EXPECT().Rollback().Return(nil)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	result, err := service.Validate(context.Background(), request)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to persist transaction validation")
	assert.Contains(t, err.Error(), "disk full")
}

// TestValidationService_Validate_Allow_AuditWriteFailure verifies that when
// persistAuditEventWithTx fails on the ALLOW path, the error is propagated
// and defer handles rollback (counters + validation record are rolled back atomically).
func TestValidationService_Validate_Allow_AuditWriteFailure(t *testing.T) {
	testutil.SetupTestTracing(t)

	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	ruleID := testutil.MustDeterministicUUID(10)
	limitID := testutil.MustDeterministicUUID(20)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)
	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, nil)

	evalResult, err := model.NewEvaluationResult(model.DecisionAllow, []uuid.UUID{ruleID}, []uuid.UUID{ruleID}, "Transaction allowed")
	require.NoError(t, err)
	ruleEval.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(evalResult, nil)

	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil)

	limitOutput := &model.CheckLimitsOutput{
		Allowed:           true,
		LimitUsageDetails: []model.LimitUsageDetail{{LimitID: limitID, LimitAmount: decimal.RequireFromString("1000"), CurrentUsage: decimal.RequireFromString("100"), Exceeded: false}},
		ExceededLimitIDs:  []uuid.UUID{},
	}
	limitCheck.EXPECT().CheckLimits(gomock.Any(), mockTx, gomock.Any()).Return(limitOutput, nil)

	// InsertWithTx succeeds
	transactionValidationRepo.EXPECT().
		InsertWithTx(gomock.Any(), mockTx, gomock.Any()).
		Return(nil)

	// Audit write fails
	auditWriter.EXPECT().
		RecordValidationEventWithTx(gomock.Any(), mockTx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("audit service unavailable"))

	// Defer calls Rollback (validation record + counters rolled back atomically)
	mockTx.EXPECT().Rollback().Return(nil)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	result, err := service.Validate(context.Background(), request)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to persist audit event")
	assert.Contains(t, err.Error(), "audit service unavailable")
}

// TestValidationService_Validate_TxContextTimeout verifies that when the transaction
// context times out during CheckLimits, the error propagates and defer rolls back.
func TestValidationService_Validate_TxContextTimeout(t *testing.T) {
	testutil.SetupTestTracing(t)

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

	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)
	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, nil)

	evalResult, err := model.NewEvaluationResult(model.DecisionAllow, []uuid.UUID{ruleID}, []uuid.UUID{ruleID}, "Transaction allowed")
	require.NoError(t, err)
	ruleEval.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(evalResult, nil)

	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil)

	// CheckLimits returns DeadlineExceeded (txCtx timed out)
	limitCheck.EXPECT().
		CheckLimits(gomock.Any(), mockTx, gomock.Any()).
		Return(nil, context.DeadlineExceeded)

	// Defer rolls back
	mockTx.EXPECT().Rollback().Return(nil)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	result, err := service.Validate(context.Background(), request)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "limit check failed")
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestValidationService_Validate_DenyByLimit_ConcurrentDuplicate covers the H2
// regression: when two requests with the same RequestID race past the
// idempotency check, both reach the DENY-by-limit branch, both call
// rollbackAndPersist, and the loser's Insert hits the unique-constraint on
// request_id. The previous implementation swallowed that error and returned
// a fresh validationID with no DB row — a 404 on subsequent
// GET /v1/validations/{id} and a broken idempotency contract.
//
// Expected behavior: rollbackAndPersist surfaces the duplicate up the stack,
// the service refetches the canonical existing record by RequestID, and the
// loser returns IsDuplicate=true with the existing validation's ID.
func TestValidationService_Validate_DenyByLimit_ConcurrentDuplicate(t *testing.T) {
	testutil.SetupTestTracing(t)

	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	existingValidationID := testutil.MustDeterministicUUID(3)
	ruleID := testutil.MustDeterministicUUID(10)
	limitID := testutil.MustDeterministicUUID(20)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("600"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)
	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	// 1. Initial idempotency check: race window — no record yet.
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, nil).
		Times(1)

	// 2. Rule evaluation returns ALLOW so we reach the limit-check branch.
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

	// 3. BeginTx for limit-check phase.
	mockTxBeginner.EXPECT().
		BeginTx(gomock.Any(), gomock.Any()).
		Return(mockTx, nil)

	// 4. CheckLimits exceeds → triggers rollbackAndPersist on DENY-by-limit.
	limitOutput := &model.CheckLimitsOutput{
		Allowed:           false,
		LimitUsageDetails: []model.LimitUsageDetail{{LimitID: limitID, LimitAmount: decimal.RequireFromString("500"), CurrentUsage: decimal.RequireFromString("600"), Exceeded: true}},
		ExceededLimitIDs:  []uuid.UUID{limitID},
	}
	limitCheck.EXPECT().
		CheckLimits(gomock.Any(), mockTx, gomock.Any()).
		Return(limitOutput, nil)

	// 5. tx.Rollback inside rollbackAndPersist.
	mockTx.EXPECT().Rollback().Return(nil)

	// 6. Non-transactional Insert returns ErrDuplicateValidation — concurrent
	// request won the persist race.
	transactionValidationRepo.EXPECT().
		Insert(gomock.Any(), gomock.Any()).
		Return(fmt.Errorf("%w: request_id %s", command.ErrDuplicateValidation, requestID))

	// 7. handleConcurrentDuplicate retries FindByRequestID and gets the existing record.
	existing, err := model.NewTransactionValidation(existingValidationID, model.DecisionDeny, fixedTime)
	require.NoError(t, err)
	existing.RequestID = requestID
	existing.Amount = decimal.RequireFromString("600")
	existing.Currency = "USD"
	existing.Account = model.AccountContext{ID: accountID}
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(existing, nil).
		Times(1)

	// Note: the audit-event write is NOT expected here. Once
	// rollbackAndPersist returns the cached duplicate, the orchestrator
	// short-circuits before persistAuditEvent — duplicate detection means
	// the audit was already written by the request that won the race.

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	result, err := service.Validate(context.Background(), request)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsDuplicate,
		"DENY-by-limit concurrent duplicate must surface IsDuplicate=true (H2)")
	assert.Equal(t, existingValidationID, result.Response.ValidationID,
		"duplicate response must echo the existing validation ID, not a fresh one")
}

// TestValidationService_Validate_Review_ConcurrentDuplicate is the REVIEW-path
// twin of the DENY-by-limit test above. Same H2 race, same expected
// idempotency outcome — exercises the second rollbackAndPersist call site so
// a regression in either branch is caught independently.
func TestValidationService_Validate_Review_ConcurrentDuplicate(t *testing.T) {
	testutil.SetupTestTracing(t)

	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1)
	accountID := testutil.MustDeterministicUUID(2)
	existingValidationID := testutil.MustDeterministicUUID(3)
	ruleID := testutil.MustDeterministicUUID(10)
	limitID := testutil.MustDeterministicUUID(20)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)

	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)
	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)

	// 1. Idempotency check: race window — no record yet.
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, nil).
		Times(1)

	// 2. Rule evaluation returns REVIEW.
	evalResult, err := model.NewEvaluationResult(
		model.DecisionReview,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"Transaction requires review",
	)
	require.NoError(t, err)
	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil)

	// 3. BeginTx for limit-check phase.
	mockTxBeginner.EXPECT().
		BeginTx(gomock.Any(), gomock.Any()).
		Return(mockTx, nil)

	// 4. CheckLimits passes — REVIEW reaches rollbackAndPersist via the
	// "rules returned REVIEW" branch.
	limitOutput := &model.CheckLimitsOutput{
		Allowed:           true,
		LimitUsageDetails: []model.LimitUsageDetail{{LimitID: limitID, LimitAmount: decimal.RequireFromString("1000"), CurrentUsage: decimal.RequireFromString("100"), Exceeded: false}},
		ExceededLimitIDs:  []uuid.UUID{},
	}
	limitCheck.EXPECT().
		CheckLimits(gomock.Any(), mockTx, gomock.Any()).
		Return(limitOutput, nil)

	// 5. tx.Rollback inside rollbackAndPersist.
	mockTx.EXPECT().Rollback().Return(nil)

	// 6. Non-transactional Insert returns ErrDuplicateValidation.
	transactionValidationRepo.EXPECT().
		Insert(gomock.Any(), gomock.Any()).
		Return(fmt.Errorf("%w: request_id %s", command.ErrDuplicateValidation, requestID))

	// 7. handleConcurrentDuplicate refetches and returns the existing record.
	existing, err := model.NewTransactionValidation(existingValidationID, model.DecisionReview, fixedTime)
	require.NoError(t, err)
	existing.RequestID = requestID
	existing.Amount = decimal.RequireFromString("100")
	existing.Currency = "USD"
	existing.Account = model.AccountContext{ID: accountID}
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(existing, nil).
		Times(1)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	result, err := service.Validate(context.Background(), request)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsDuplicate,
		"REVIEW concurrent duplicate must surface IsDuplicate=true (H2)")
	assert.Equal(t, existingValidationID, result.Response.ValidationID,
		"duplicate response must echo the existing validation ID, not a fresh one")
}
