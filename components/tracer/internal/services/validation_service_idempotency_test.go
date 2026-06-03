// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "tracer/internal/adapters/postgres/db"
	pgdbMocks "tracer/internal/adapters/postgres/db/mocks"
	commandMocks "tracer/internal/services/command/mocks"
	"tracer/internal/services/mocks"
	queryMocks "tracer/internal/services/query/mocks"
	"tracer/internal/testutil"
	"tracer/pkg/model"
)

// =============================================================================
// Transactional Deduplication Tests
// =============================================================================
// These tests verify the deduplication behavior for idempotent requests.
// Duplicate detection uses FindByRequestID to check if a request was already processed.
// =============================================================================

// TestValidate_DuplicateRequestID_ReturnsOriginal tests that when the same request_id
// is submitted twice, the second call returns the original response (DD-3: Stripe model).
//
// Expected behavior:
// - First call: processes validation, persists result, returns ValidateResult{Response, IsDuplicate: false}
// - Second call: finds existing record, returns ValidateResult{Response, IsDuplicate: true}
func TestValidate_DuplicateRequestID_ReturnsOriginal(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(1001)
	accountID := testutil.MustDeterministicUUID(1002)
	ruleID := testutil.MustDeterministicUUID(1010)
	validationID := testutil.MustDeterministicUUID(1020)

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
	mockTx := pgdbMocks.NewMockTx(ctrl)
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)

	// First call expectations - FindByRequestID returns nil (no existing record)
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, nil).
		Times(1)

	evalResult, err := model.NewEvaluationResult(
		model.DecisionAllow,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"Transaction allowed",
	)
	require.NoError(t, err)

	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil).
		Times(1)

	// BeginTx is called for ALLOW path
	mockTxBeginner.EXPECT().
		BeginTx(gomock.Any(), gomock.Any()).
		Return(mockTx, nil).
		Times(1)

	// CheckLimits is called for ALLOW path
	limitCheck.EXPECT().
		CheckLimits(gomock.Any(), mockTx, gomock.Any()).
		Return(&model.CheckLimitsOutput{Allowed: true, LimitUsageDetails: []model.LimitUsageDetail{}}, nil).
		Times(1)

	// InsertWithTx is called for ALLOW path
	transactionValidationRepo.EXPECT().
		InsertWithTx(gomock.Any(), mockTx, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ pgdb.DB, _ *model.TransactionValidation) error {
			close(persistDone)
			return nil
		}).
		Times(1)

	// RecordValidationEventWithTx is called for ALLOW path
	auditWriter.EXPECT().
		RecordValidationEventWithTx(gomock.Any(), mockTx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	// Commit is called for ALLOW path
	mockTx.EXPECT().
		Commit().
		Return(nil).
		Times(1)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// First call
	result1, err := service.Validate(context.Background(), request)

	// Wait for persistence to complete
	select {
	case <-persistDone:
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for persistence to complete")
	}

	require.NoError(t, err)
	require.NotNil(t, result1)
	require.NotNil(t, result1.Response)
	assert.Equal(t, model.DecisionAllow, result1.Response.Decision)
	assert.False(t, result1.IsDuplicate, "First call should NOT be marked as duplicate")

	// Second call expectations - FindByRequestID returns the existing record
	existingTV, err := model.NewTransactionValidation(validationID, model.DecisionAllow, fixedTime)
	require.NoError(t, err)
	existingTV.RequestID = requestID
	existingTV.TransactionType = model.TransactionTypeCard
	existingTV.Amount = decimal.RequireFromString("100")
	existingTV.Currency = "USD"
	existingTV.TransactionTimestamp = fixedTime
	existingTV.Account = model.AccountContext{ID: accountID}
	existingTV.EvaluationResult = *evalResult
	existingTV.LimitUsageDetails = []model.LimitUsageDetail{}
	existingTV.ProcessingTimeMs = 15

	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(existingTV, nil).
		Times(1)

	// No other operations should be called for duplicates
	// (No ruleEval.Execute, limitCheck.CheckLimits, transactionValidationRepo.Insert, auditWriter.RecordValidationEvent)

	// Second call
	result2, err := service.Validate(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, result2)
	require.NotNil(t, result2.Response)
	assert.Equal(t, validationID, result2.Response.ValidationID, "Duplicate should return same validation ID")
	assert.True(t, result2.IsDuplicate, "Second call should be marked as duplicate")
}

// TestValidate_DuplicateRequestID_NoDoubleCount tests that usage counters are NOT
// incremented when a duplicate request is detected (DD-3).
//
// Expected behavior:
// - When a duplicate request_id is detected, CheckLimits should NOT be called
// - This prevents double-counting the transaction against limits
func TestValidate_DuplicateRequestID_NoDoubleCount(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(2001)
	accountID := testutil.MustDeterministicUUID(2002)
	ruleID := testutil.MustDeterministicUUID(2010)
	validationID := testutil.MustDeterministicUUID(2020)

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

	// Prepare existing validation record
	evalResult, err := model.NewEvaluationResult(
		model.DecisionAllow,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"Transaction allowed",
	)
	require.NoError(t, err)

	existingTV, err := model.NewTransactionValidation(validationID, model.DecisionAllow, fixedTime)
	require.NoError(t, err)
	existingTV.RequestID = requestID
	existingTV.TransactionType = model.TransactionTypeCard
	existingTV.Amount = decimal.RequireFromString("100")
	existingTV.Currency = "USD"
	existingTV.TransactionTimestamp = fixedTime
	existingTV.Account = model.AccountContext{ID: accountID}
	existingTV.EvaluationResult = *evalResult
	existingTV.LimitUsageDetails = []model.LimitUsageDetail{}
	existingTV.ProcessingTimeMs = 15

	// EXPECTED BEHAVIOR:
	// Service should first check if this request_id already exists in the database.
	// If it does, it should return the cached response WITHOUT calling any other methods.

	// FindByRequestID returns existing record - duplicate detected
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(existingTV, nil).
		Times(1)

	// CRITICAL: These should NOT be called for duplicates
	mockTxBeginner.EXPECT().
		BeginTx(gomock.Any(), gomock.Any()).
		Times(0)

	limitCheck.EXPECT().
		CheckLimits(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Times(0)

	auditWriter.EXPECT().
		RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	auditWriter.EXPECT().
		RecordValidationEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	transactionValidationRepo.EXPECT().
		Insert(gomock.Any(), gomock.Any()).
		Times(0)

	transactionValidationRepo.EXPECT().
		InsertWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Call with duplicate request
	result, err := service.Validate(context.Background(), request)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Response)
	assert.True(t, result.IsDuplicate, "Duplicate request should be flagged")
	assert.Equal(t, validationID, result.Response.ValidationID, "Should return original validation ID")
}

// TestValidate_DuplicateRequestID_NoAudit tests that audit events are NOT created
// when a duplicate request is detected (DD-3).
//
// Expected behavior:
// - When a duplicate request_id is detected, audit should NOT be recorded
// - The original audit from the first request is sufficient
func TestValidate_DuplicateRequestID_NoAudit(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(3001)
	accountID := testutil.MustDeterministicUUID(3002)
	validationID := testutil.MustDeterministicUUID(3020)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("50"),
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

	// Prepare existing validation record
	existingTV, err := model.NewTransactionValidation(validationID, model.DecisionAllow, fixedTime)
	require.NoError(t, err)
	existingTV.RequestID = requestID
	existingTV.TransactionType = model.TransactionTypeCard
	existingTV.Amount = decimal.RequireFromString("50")
	existingTV.Currency = "USD"
	existingTV.TransactionTimestamp = fixedTime
	existingTV.Account = model.AccountContext{ID: accountID}
	existingTV.MatchedRuleIDs = []uuid.UUID{}
	existingTV.EvaluatedRuleIDs = []uuid.UUID{}
	existingTV.LimitUsageDetails = []model.LimitUsageDetail{}
	existingTV.ProcessingTimeMs = 10

	// CRITICAL: For duplicate detection, FindByRequestID returns existing record
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(existingTV, nil).
		Times(1)

	// CRITICAL: These should NOT be called for duplicates
	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	auditWriter.EXPECT().RecordValidationEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	// No other operations should be called for duplicates
	ruleEval.EXPECT().Execute(gomock.Any(), gomock.Any()).Times(0)
	limitCheck.EXPECT().CheckLimits(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	transactionValidationRepo.EXPECT().Insert(gomock.Any(), gomock.Any()).Times(0)
	transactionValidationRepo.EXPECT().InsertWithTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	// Call with duplicate request
	result, err := service.Validate(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsDuplicate, "Duplicate request should be flagged")
}

// TestValidate_DenyByRule_RetryTolerant tests that DENY-by-rule works correctly
// and checks for duplicates before processing.
func TestValidate_DenyByRule_RetryTolerant(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(4001)
	accountID := testutil.MustDeterministicUUID(4002)
	ruleID := testutil.MustDeterministicUUID(4010)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("1000"),
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
	auditWriter := mocks.NewMockAuditWriter(ctrl)

	// FindByRequestID returns nil (no existing record) - new request
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, nil).
		Times(1)

	// Rule returns DENY
	evalResult, err := model.NewEvaluationResult(
		model.DecisionDeny,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"High-risk transaction blocked",
	)
	require.NoError(t, err)

	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil).
		Times(1)

	// No BeginTx for DENY by rule
	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Times(0)

	// Limit check NOT called for DENY by rule
	limitCheck.EXPECT().CheckLimits(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	// Non-transactional Insert for DENY-by-rule
	transactionValidationRepo.EXPECT().
		Insert(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *model.TransactionValidation) error {
			close(persistDone)
			return nil
		}).
		Times(1)

	// Non-transactional audit for DENY-by-rule
	auditWriter.EXPECT().
		RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	result, err := service.Validate(context.Background(), request)

	select {
	case <-persistDone:
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for persistence to complete")
	}

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Response)
	assert.Equal(t, model.DecisionDeny, result.Response.Decision)
	assert.False(t, result.IsDuplicate)
}

// TestValidate_DenyByLimit_RollbackAtomic tests that limits are handled correctly
// when DENY-by-limit occurs.
func TestValidate_DenyByLimit_RollbackAtomic(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(5001)
	accountID := testutil.MustDeterministicUUID(5002)
	ruleID := testutil.MustDeterministicUUID(5010)
	limitID := testutil.MustDeterministicUUID(5020)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("500"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)
	persistDone := make(chan struct{})

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)

	// FindByRequestID returns nil (no existing record) - new request
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, nil).
		Times(1)

	// Rule returns ALLOW
	evalResult, err := model.NewEvaluationResult(
		model.DecisionAllow,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"Transaction allowed",
	)
	require.NoError(t, err)

	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil).
		Times(1)

	// BeginTx is called
	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil).Times(1)

	// Limit check returns EXCEEDED via CheckLimits
	limitOutput := &model.CheckLimitsOutput{
		Allowed: false,
		LimitUsageDetails: []model.LimitUsageDetail{
			{
				LimitID:      limitID,
				LimitAmount:  decimal.RequireFromString("400"),
				CurrentUsage: decimal.RequireFromString("450"),
				Exceeded:     true,
				Period:       model.LimitTypeDaily,
			},
		},
		ExceededLimitIDs: []uuid.UUID{limitID},
	}

	limitCheck.EXPECT().
		CheckLimits(gomock.Any(), mockTx, gomock.Any()).
		Return(limitOutput, nil).
		Times(1)

	// Rollback is called to undo counter increments
	mockTx.EXPECT().Rollback().Return(nil).Times(1)

	// Non-transactional Insert for DENY-by-limit
	transactionValidationRepo.EXPECT().
		Insert(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *model.TransactionValidation) error {
			close(persistDone)
			return nil
		}).
		Times(1)

	// Non-transactional audit for DENY-by-limit
	auditWriter.EXPECT().
		RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	result, err := service.Validate(context.Background(), request)

	select {
	case <-persistDone:
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for persistence to complete")
	}

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Response)
	assert.Equal(t, model.DecisionDeny, result.Response.Decision)
	assert.Equal(t, "limit_exceeded", result.Response.Reason)
	assert.False(t, result.IsDuplicate)
}

// TestValidate_Review_RollbackAtomic tests that limits are rolled back
// when REVIEW decision is made.
func TestValidate_Review_RollbackAtomic(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	requestID := testutil.MustDeterministicUUID(6001)
	accountID := testutil.MustDeterministicUUID(6002)
	ruleID := testutil.MustDeterministicUUID(6010)
	limitID := testutil.MustDeterministicUUID(6020)

	request := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("200"),
		Currency:             "USD",
		TransactionTimestamp: fixedTime,
		Account:              model.AccountContext{ID: accountID},
	}

	ctrl := gomock.NewController(t)
	persistDone := make(chan struct{})

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	ruleEval := mocks.NewMockRuleEvaluator(ctrl)
	limitCheck := mocks.NewMockLimitChecker(ctrl)
	transactionValidationRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	transactionValidationQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)
	auditWriter := mocks.NewMockAuditWriter(ctrl)

	// FindByRequestID returns nil (no existing record) - new request
	transactionValidationQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), requestID).
		Return(nil, nil).
		Times(1)

	// Rule returns REVIEW
	evalResult, err := model.NewEvaluationResult(
		model.DecisionReview,
		[]uuid.UUID{ruleID},
		[]uuid.UUID{ruleID},
		"Manual review required",
	)
	require.NoError(t, err)

	ruleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil).
		Times(1)

	// BeginTx is called
	mockTxBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(mockTx, nil).Times(1)

	// Limit check passes via CheckLimits
	limitOutput := &model.CheckLimitsOutput{
		Allowed: true,
		LimitUsageDetails: []model.LimitUsageDetail{
			{
				LimitID:           limitID,
				LimitAmount:       decimal.RequireFromString("1000"),
				CurrentUsage:      decimal.RequireFromString("200"),
				Exceeded:          false,
				Period:            model.LimitTypeDaily,
				InternalLimitType: model.LimitTypeDaily,
				InternalPeriodKey: "2025-01-15",
			},
		},
		ExceededLimitIDs: []uuid.UUID{},
	}

	limitCheck.EXPECT().
		CheckLimits(gomock.Any(), mockTx, gomock.Any()).
		Return(limitOutput, nil).
		Times(1)

	// tx.Rollback is called for REVIEW decisions
	mockTx.EXPECT().Rollback().Return(nil).Times(1)

	// Non-transactional Insert for REVIEW
	transactionValidationRepo.EXPECT().
		Insert(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *model.TransactionValidation) error {
			close(persistDone)
			return nil
		}).
		Times(1)

	// Non-transactional audit for REVIEW
	auditWriter.EXPECT().
		RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	service, err := NewValidationService(mockTxBeginner, ruleEval, limitCheck, transactionValidationRepo, transactionValidationQueryRepo, auditWriter, nil)
	require.NoError(t, err)

	result, err := service.Validate(context.Background(), request)

	select {
	case <-persistDone:
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for persistence to complete")
	}

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Response)
	assert.Equal(t, model.DecisionReview, result.Response.Decision)
	assert.False(t, result.IsDuplicate)
}
