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
	"go.uber.org/mock/gomock"

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	commandMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/mocks"
	queryMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// benchSink prevents compiler optimization of benchmark results.
var benchSink any

func BenchmarkValidationService_Validate(b *testing.B) {
	ctrl := gomock.NewController(b)

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	mockRuleEval := mocks.NewMockRuleEvaluator(ctrl)
	mockLimitCheck := mocks.NewMockLimitChecker(ctrl)
	mockAuditRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	mockAuditQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

	// Setup mock responses
	evalResult, err := model.NewEvaluationResult(
		model.DecisionAllow,
		[]uuid.UUID{},
		[]uuid.UUID{testutil.MustDeterministicUUID(1)},
		"No blocking rules",
	)
	if err != nil {
		b.Fatal(err)
	}

	limitOutput := &model.CheckLimitsOutput{
		Allowed:           true,
		LimitUsageDetails: []model.LimitUsageDetail{},
	}

	// FindByRequestID returns nil (no existing record) - new request
	mockAuditQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	mockRuleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil).
		AnyTimes()

	// BeginTx is called for ALLOW path
	mockTxBeginner.EXPECT().
		BeginTx(gomock.Any(), gomock.Any()).
		Return(mockTx, nil).
		AnyTimes()

	// CheckLimits is called for ALLOW path
	mockLimitCheck.EXPECT().
		CheckLimits(gomock.Any(), mockTx, gomock.Any()).
		Return(limitOutput, nil).
		AnyTimes()

	// InsertWithTx is called for ALLOW path
	mockAuditRepo.EXPECT().
		InsertWithTx(gomock.Any(), mockTx, gomock.Any()).
		Return(nil).
		AnyTimes()

	mockAuditWriter := mocks.NewMockAuditWriter(ctrl)
	mockAuditWriter.EXPECT().
		RecordValidationEventWithTx(gomock.Any(), mockTx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Commit is called for ALLOW path
	mockTx.EXPECT().
		Commit().
		Return(nil).
		AnyTimes()

	service, err := NewValidationService(mockTxBeginner, mockRuleEval, mockLimitCheck, mockAuditRepo, mockAuditQueryRepo, mockAuditWriter, nil)
	if err != nil {
		b.Fatal(err)
	}

	accountID := testutil.MustDeterministicUUID(100)
	request := &model.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1),
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: time.Now(),
		Account:              model.AccountContext{ID: accountID},
	}

	ctx := context.Background()

	b.ResetTimer()

	for b.Loop() {
		result, err := service.Validate(ctx, request)
		if err != nil {
			b.Fatal(err)
		}

		benchSink = result
	}
}

func BenchmarkValidationService_Validate_WithDenyRule(b *testing.B) {
	ctrl := gomock.NewController(b)

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockRuleEval := mocks.NewMockRuleEvaluator(ctrl)
	mockLimitCheck := mocks.NewMockLimitChecker(ctrl)
	mockAuditRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	mockAuditQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

	// Setup mock to return DENY (should skip limit check)
	evalResult, err := model.NewEvaluationResult(
		model.DecisionDeny,
		[]uuid.UUID{testutil.MustDeterministicUUID(10)},
		[]uuid.UUID{testutil.MustDeterministicUUID(10)},
		"Rule blocked transaction",
	)
	if err != nil {
		b.Fatal(err)
	}

	// FindByRequestID returns nil (no existing record) - new request
	mockAuditQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	mockRuleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil).
		AnyTimes()

	// No BeginTx for DENY-by-rule
	mockTxBeginner.EXPECT().
		BeginTx(gomock.Any(), gomock.Any()).
		Times(0)

	// LimitChecker should NOT be called when DENY by rule
	mockLimitCheck.EXPECT().
		CheckLimits(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	// Non-transactional Insert for DENY-by-rule
	mockAuditRepo.EXPECT().
		Insert(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockAuditWriter := mocks.NewMockAuditWriter(ctrl)
	mockAuditWriter.EXPECT().
		RecordValidationEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	service, err := NewValidationService(mockTxBeginner, mockRuleEval, mockLimitCheck, mockAuditRepo, mockAuditQueryRepo, mockAuditWriter, nil)
	if err != nil {
		b.Fatal(err)
	}

	accountID := testutil.MustDeterministicUUID(100)
	request := &model.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(1),
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("100"),
		Currency:             "USD",
		TransactionTimestamp: time.Now(),
		Account:              model.AccountContext{ID: accountID},
	}

	ctx := context.Background()

	b.ResetTimer()

	for b.Loop() {
		result, err := service.Validate(ctx, request)
		if err != nil {
			b.Fatal(err)
		}

		benchSink = result
	}
}

func BenchmarkValidationService_Validate_Parallel(b *testing.B) {
	ctrl := gomock.NewController(b)

	mockTxBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	mockTx := pgdbMocks.NewMockTx(ctrl)
	mockRuleEval := mocks.NewMockRuleEvaluator(ctrl)
	mockLimitCheck := mocks.NewMockLimitChecker(ctrl)
	mockAuditRepo := commandMocks.NewMockTransactionValidationRepository(ctrl)
	mockAuditQueryRepo := queryMocks.NewMockTransactionValidationRepository(ctrl)

	evalResult, err := model.NewEvaluationResult(
		model.DecisionAllow,
		[]uuid.UUID{},
		[]uuid.UUID{testutil.MustDeterministicUUID(1)},
		"No blocking rules",
	)
	if err != nil {
		b.Fatal(err)
	}

	limitOutput := &model.CheckLimitsOutput{
		Allowed:           true,
		LimitUsageDetails: []model.LimitUsageDetail{},
	}

	// FindByRequestID returns nil (no existing record) - new request
	mockAuditQueryRepo.EXPECT().
		FindByRequestID(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	mockRuleEval.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(evalResult, nil).
		AnyTimes()

	// BeginTx is called for ALLOW path
	mockTxBeginner.EXPECT().
		BeginTx(gomock.Any(), gomock.Any()).
		Return(mockTx, nil).
		AnyTimes()

	// CheckLimits is called for ALLOW path
	mockLimitCheck.EXPECT().
		CheckLimits(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(limitOutput, nil).
		AnyTimes()

	// InsertWithTx is called for ALLOW path
	mockAuditRepo.EXPECT().
		InsertWithTx(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockAuditWriter := mocks.NewMockAuditWriter(ctrl)
	mockAuditWriter.EXPECT().
		RecordValidationEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Commit is called for ALLOW path
	mockTx.EXPECT().
		Commit().
		Return(nil).
		AnyTimes()

	service, err := NewValidationService(mockTxBeginner, mockRuleEval, mockLimitCheck, mockAuditRepo, mockAuditQueryRepo, mockAuditWriter, nil)
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		accountID := uuid.New()
		request := &model.ValidationRequest{
			RequestID:            uuid.New(),
			TransactionType:      model.TransactionTypeCard,
			Amount:               decimal.RequireFromString("100"),
			Currency:             "USD",
			TransactionTimestamp: time.Now(),
			Account:              model.AccountContext{ID: accountID},
		}

		var localSink any
		for pb.Next() {
			result, err := service.Validate(ctx, request)
			if err != nil {
				b.Error(err)
				return
			}

			localSink = result
		}
		benchSink = localSink // Single write after loop
	})
}
