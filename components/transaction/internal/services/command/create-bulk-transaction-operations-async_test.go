// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/repository"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"
)

// =============================================================================
// UNIT TESTS - BulkResult Struct
// =============================================================================

func TestBulkResult_ZeroValues(t *testing.T) {
	t.Parallel()

	result := &BulkResult{}

	assert.Equal(t, int64(0), result.TransactionsAttempted)
	assert.Equal(t, int64(0), result.TransactionsInserted)
	assert.Equal(t, int64(0), result.TransactionsIgnored)
	assert.Equal(t, int64(0), result.OperationsAttempted)
	assert.Equal(t, int64(0), result.OperationsInserted)
	assert.Equal(t, int64(0), result.OperationsIgnored)
	assert.False(t, result.FallbackUsed)
	assert.Equal(t, 0, result.FallbackCount)
}

// =============================================================================
// UNIT TESTS - BulkMessageResult Struct
// =============================================================================

func TestBulkMessageResult_Success(t *testing.T) {
	t.Parallel()

	result := BulkMessageResult{
		Index:   5,
		Success: true,
		Error:   nil,
	}

	assert.Equal(t, 5, result.Index)
	assert.True(t, result.Success)
	assert.Nil(t, result.Error)
}

func TestBulkMessageResult_Failure(t *testing.T) {
	t.Parallel()

	testErr := errors.New("test error")
	result := BulkMessageResult{
		Index:   3,
		Success: false,
		Error:   testErr,
	}

	assert.Equal(t, 3, result.Index)
	assert.False(t, result.Success)
	assert.Equal(t, testErr, result.Error)
}

// =============================================================================
// UNIT TESTS - sortTransactionsByID
// =============================================================================

func TestSortTransactionsByID(t *testing.T) {
	t.Parallel()

	transactions := []*transaction.Transaction{
		{ID: "c-transaction"},
		{ID: "a-transaction"},
		{ID: "b-transaction"},
	}

	sortTransactionsByID(transactions)

	assert.Equal(t, "a-transaction", transactions[0].ID)
	assert.Equal(t, "b-transaction", transactions[1].ID)
	assert.Equal(t, "c-transaction", transactions[2].ID)
}

func TestSortTransactionsByID_Empty(t *testing.T) {
	t.Parallel()

	transactions := []*transaction.Transaction{}

	// Should not panic
	sortTransactionsByID(transactions)

	assert.Empty(t, transactions)
}

func TestSortTransactionsByID_Single(t *testing.T) {
	t.Parallel()

	transactions := []*transaction.Transaction{
		{ID: "only-transaction"},
	}

	sortTransactionsByID(transactions)

	assert.Len(t, transactions, 1)
	assert.Equal(t, "only-transaction", transactions[0].ID)
}

// =============================================================================
// UNIT TESTS - sortOperationsByID
// =============================================================================

func TestSortOperationsByID(t *testing.T) {
	t.Parallel()

	operations := []*operation.Operation{
		{ID: "z-operation"},
		{ID: "a-operation"},
		{ID: "m-operation"},
	}

	sortOperationsByID(operations)

	assert.Equal(t, "a-operation", operations[0].ID)
	assert.Equal(t, "m-operation", operations[1].ID)
	assert.Equal(t, "z-operation", operations[2].ID)
}

func TestSortOperationsByID_Empty(t *testing.T) {
	t.Parallel()

	operations := []*operation.Operation{}

	// Should not panic
	sortOperationsByID(operations)

	assert.Empty(t, operations)
}

// =============================================================================
// UNIT TESTS - CreateBulkTransactionOperationsAsync - Empty Messages
// =============================================================================

func TestCreateBulkTransactionOperationsAsync_EmptyMessages(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := &UseCase{}
	ctx := context.Background()

	result, messageResults, err := uc.CreateBulkTransactionOperationsAsync(ctx, []mmodel.Queue{}, true)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, messageResults)
	assert.Equal(t, int64(0), result.TransactionsAttempted)
}

// =============================================================================
// UNIT TESTS - CreateBulkTransactionOperationsAsync - Success Path
// =============================================================================

func TestCreateBulkTransactionOperationsAsync_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		OperationRepo:   mockOperationRepo,
		MetadataRepo:    mockMetadataRepo,
		BalanceRepo:     mockBalanceRepo,
		RabbitMQRepo:    mockRabbitMQRepo,
		RedisRepo:       mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()

	// Create a NOTED transaction (skips balance update)
	notedStatus := constant.NOTED
	tran := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code:        notedStatus,
			Description: &notedStatus,
		},
		Operations: []*operation.Operation{
			{ID: uuid.New().String(), TransactionID: transactionID},
		},
	}

	payload := transaction.TransactionProcessingPayload{
		Transaction: tran,
		Validate:    &pkgTransaction.Responses{},
	}

	payloadBytes, err := msgpack.Marshal(payload)
	require.NoError(t, err)

	messages := []mmodel.Queue{
		{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData: []mmodel.QueueData{
				{ID: uuid.New(), Value: payloadBytes},
			},
		},
	}

	// Mock expectations for bulk insert
	mockTransactionRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 1,
			Inserted:  1,
			Ignored:   0,
		}, nil)

	mockOperationRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 1,
			Inserted:  1,
			Ignored:   0,
		}, nil)

	// Mock metadata creation (best effort, can return error)
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock Redis cleanup (best effort) - multiple methods called in goroutines
	mockRedisRepo.EXPECT().
		RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock RabbitMQ for events (best effort)
	mockRabbitMQRepo.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	result, messageResults, err := uc.CreateBulkTransactionOperationsAsync(ctx, messages, true)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(1), result.TransactionsInserted)
	assert.Equal(t, int64(1), result.OperationsInserted)
	assert.False(t, result.FallbackUsed)
	assert.Len(t, messageResults, 1)
	assert.True(t, messageResults[0].Success)

	// Wait for goroutines to complete
	// Note: In production, these run async but for tests we need to wait
	// to avoid race conditions with mock controller cleanup
	<-time.After(100 * time.Millisecond)
}

// =============================================================================
// UNIT TESTS - CreateBulkTransactionOperationsAsync - Fallback on Failure
// =============================================================================

func TestCreateBulkTransactionOperationsAsync_FallbackOnBulkFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		OperationRepo:   mockOperationRepo,
		MetadataRepo:    mockMetadataRepo,
		BalanceRepo:     mockBalanceRepo,
		RabbitMQRepo:    mockRabbitMQRepo,
		RedisRepo:       mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()

	// Create a NOTED transaction (skips balance update)
	notedStatus := constant.NOTED
	tran := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code:        notedStatus,
			Description: &notedStatus,
		},
		Operations: []*operation.Operation{},
	}

	payload := transaction.TransactionProcessingPayload{
		Transaction: tran,
		Validate:    &pkgTransaction.Responses{},
	}

	payloadBytes, err := msgpack.Marshal(payload)
	require.NoError(t, err)

	messages := []mmodel.Queue{
		{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData: []mmodel.QueueData{
				{ID: uuid.New(), Value: payloadBytes},
			},
		},
	}

	// Mock bulk insert failure
	mockTransactionRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("bulk insert failed"))

	// Mock individual transaction creation (fallback)
	mockTransactionRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(tran, nil)

	// Mock metadata creation (best effort)
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock Redis cleanup (best effort) - multiple methods called in goroutines
	mockRedisRepo.EXPECT().
		RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock RabbitMQ for events (best effort)
	mockRabbitMQRepo.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	result, messageResults, err := uc.CreateBulkTransactionOperationsAsync(ctx, messages, true)

	require.NoError(t, err) // Fallback should succeed
	assert.NotNil(t, result)
	assert.True(t, result.FallbackUsed)
	assert.Equal(t, 1, result.FallbackCount)
	assert.Len(t, messageResults, 1)
	assert.True(t, messageResults[0].Success)

	// Wait for goroutines to complete
	<-time.After(100 * time.Millisecond)
}

// =============================================================================
// UNIT TESTS - CreateBulkTransactionOperationsAsync - No Fallback
// =============================================================================

func TestCreateBulkTransactionOperationsAsync_NoFallbackOnFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		OperationRepo:   mockOperationRepo,
		MetadataRepo:    mockMetadataRepo,
		BalanceRepo:     mockBalanceRepo,
		RabbitMQRepo:    mockRabbitMQRepo,
		RedisRepo:       mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()

	// Create a NOTED transaction (skips balance update)
	notedStatus := constant.NOTED
	tran := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code:        notedStatus,
			Description: &notedStatus,
		},
		Operations: []*operation.Operation{},
	}

	payload := transaction.TransactionProcessingPayload{
		Transaction: tran,
		Validate:    &pkgTransaction.Responses{},
	}

	payloadBytes, err := msgpack.Marshal(payload)
	require.NoError(t, err)

	messages := []mmodel.Queue{
		{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData: []mmodel.QueueData{
				{ID: uuid.New(), Value: payloadBytes},
			},
		},
	}

	// Mock bulk insert failure
	mockTransactionRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("bulk insert failed"))

	// Fallback disabled - should return error
	result, messageResults, err := uc.CreateBulkTransactionOperationsAsync(ctx, messages, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "bulk insert failed")
	assert.NotNil(t, result)
	assert.False(t, result.FallbackUsed)
	assert.Len(t, messageResults, 1)
	assert.False(t, messageResults[0].Success)
}

// =============================================================================
// UNIT TESTS - CreateBulkTransactionOperationsAsync - Duplicates Ignored
// =============================================================================

func TestCreateBulkTransactionOperationsAsync_DuplicatesIgnored(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		OperationRepo:   mockOperationRepo,
		MetadataRepo:    mockMetadataRepo,
		BalanceRepo:     mockBalanceRepo,
		RabbitMQRepo:    mockRabbitMQRepo,
		RedisRepo:       mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Create 2 NOTED transactions (one will be duplicate)
	notedStatus := constant.NOTED

	createPayload := func(t *testing.T, id string) []byte {
		t.Helper()

		tran := &transaction.Transaction{
			ID:             id,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Status: transaction.Status{
				Code:        notedStatus,
				Description: &notedStatus,
			},
			Operations: []*operation.Operation{},
		}
		payload := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    &pkgTransaction.Responses{},
		}
		bytes, err := msgpack.Marshal(payload)
		require.NoError(t, err)

		return bytes
	}

	messages := []mmodel.Queue{
		{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData:      []mmodel.QueueData{{ID: uuid.New(), Value: createPayload(t, "tx-1")}},
		},
		{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData:      []mmodel.QueueData{{ID: uuid.New(), Value: createPayload(t, "tx-2")}},
		},
	}

	// Mock bulk insert with 1 duplicate
	mockTransactionRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 2,
			Inserted:  1,
			Ignored:   1, // One duplicate
		}, nil)

	// Operations CreateBulk may not be called if there are no operations
	mockOperationRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 0,
			Inserted:  0,
			Ignored:   0,
		}, nil).
		AnyTimes()

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockRedisRepo.EXPECT().
		RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock RabbitMQ for events (best effort)
	mockRabbitMQRepo.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	result, messageResults, err := uc.CreateBulkTransactionOperationsAsync(ctx, messages, true)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(2), result.TransactionsAttempted)
	assert.Equal(t, int64(1), result.TransactionsInserted)
	assert.Equal(t, int64(1), result.TransactionsIgnored) // Duplicate detected
	assert.False(t, result.FallbackUsed)
	assert.Len(t, messageResults, 2)

	// Wait for goroutines to complete
	<-time.After(100 * time.Millisecond)
}

// =============================================================================
// UNIT TESTS - CreateBulkTransactionOperationsAsync - Balance Processing
// =============================================================================

func TestCreateBulkTransactionOperationsAsync_WithBalanceProcessing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		OperationRepo:   mockOperationRepo,
		MetadataRepo:    mockMetadataRepo,
		BalanceRepo:     mockBalanceRepo,
		RabbitMQRepo:    mockRabbitMQRepo,
		RedisRepo:       mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()

	// Create a CREATED transaction (requires balance update)
	createdStatus := constant.CREATED
	approvedStatus := constant.APPROVED

	balances := []*mmodel.Balance{
		{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Alias:          "alias1",
			Available:      decimal.NewFromInt(100),
			OnHold:         decimal.NewFromInt(0),
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			AssetCode:      "USD",
		},
	}

	tran := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code:        createdStatus,
			Description: &approvedStatus,
		},
		Operations: []*operation.Operation{},
	}

	validate := &pkgTransaction.Responses{
		Aliases: []string{"alias1"},
		From: map[string]pkgTransaction.Amount{
			"alias1": {Asset: "USD", Value: decimal.NewFromInt(50)},
		},
	}

	payload := transaction.TransactionProcessingPayload{
		Transaction:   tran,
		Validate:      validate,
		Balances:      balances,
		BalancesAfter: balances, // Same for simplicity
	}

	payloadBytes, err := msgpack.Marshal(payload)
	require.NoError(t, err)

	messages := []mmodel.Queue{
		{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData: []mmodel.QueueData{
				{ID: uuid.New(), Value: payloadBytes},
			},
		},
	}

	// Mock balance sync batch for balance update
	mockBalanceRepo.EXPECT().
		SyncBatch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(int64(1), nil).
		AnyTimes()

	// Mock BalancesUpdate for individual balance updates
	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock bulk insert
	mockTransactionRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 1,
			Inserted:  1,
			Ignored:   0,
		}, nil)

	// Operations CreateBulk may not be called if there are no operations
	mockOperationRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 0,
			Inserted:  0,
			Ignored:   0,
		}, nil).
		AnyTimes()

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockRedisRepo.EXPECT().
		RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock RabbitMQ for events (best effort)
	mockRabbitMQRepo.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	result, messageResults, err := uc.CreateBulkTransactionOperationsAsync(ctx, messages, true)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(1), result.TransactionsInserted)
	assert.Len(t, messageResults, 1)
	assert.True(t, messageResults[0].Success)

	// Wait for goroutines to complete
	<-time.After(100 * time.Millisecond)
}

// =============================================================================
// UNIT TESTS - CreateBulkTransactionOperationsAsync - Unmarshal Failure
// =============================================================================

func TestCreateBulkTransactionOperationsAsync_UnmarshalFailure(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Create message with invalid msgpack data
	messages := []mmodel.Queue{
		{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData: []mmodel.QueueData{
				{ID: uuid.New(), Value: []byte("invalid msgpack data that cannot be unmarshaled")},
			},
		},
	}

	result, messageResults, err := uc.CreateBulkTransactionOperationsAsync(ctx, messages, true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
	assert.NotNil(t, result)
	assert.Len(t, messageResults, 1)
	assert.False(t, messageResults[0].Success)
	assert.NotNil(t, messageResults[0].Error)
}

func TestCreateBulkTransactionOperationsAsync_PartialUnmarshalFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		OperationRepo:   mockOperationRepo,
		MetadataRepo:    mockMetadataRepo,
		BalanceRepo:     mockBalanceRepo,
		RabbitMQRepo:    mockRabbitMQRepo,
		RedisRepo:       mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()

	// Create one valid and one invalid message
	notedStatus := constant.NOTED
	tran := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code:        notedStatus,
			Description: &notedStatus,
		},
		Operations: []*operation.Operation{},
	}

	payload := transaction.TransactionProcessingPayload{
		Transaction: tran,
		Validate:    &pkgTransaction.Responses{},
	}

	validPayloadBytes, err := msgpack.Marshal(payload)
	require.NoError(t, err)

	messages := []mmodel.Queue{
		{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData: []mmodel.QueueData{
				{ID: uuid.New(), Value: []byte("invalid data")}, // Invalid
			},
		},
		{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData: []mmodel.QueueData{
				{ID: uuid.New(), Value: validPayloadBytes}, // Valid
			},
		},
	}

	// Mock bulk insert for the one valid message
	mockTransactionRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 1,
			Inserted:  1,
			Ignored:   0,
		}, nil)

	// Operations CreateBulk may not be called if there are no operations
	mockOperationRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 0,
			Inserted:  0,
			Ignored:   0,
		}, nil).
		AnyTimes()

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockRedisRepo.EXPECT().
		RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockRabbitMQRepo.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	result, messageResults, err := uc.CreateBulkTransactionOperationsAsync(ctx, messages, true)

	// Should succeed because at least one message was valid
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, messageResults, 2)

	// First message (invalid) should fail
	assert.False(t, messageResults[0].Success)
	assert.NotNil(t, messageResults[0].Error)

	// Second message (valid) should succeed
	assert.True(t, messageResults[1].Success)

	// Wait for goroutines to complete
	<-time.After(100 * time.Millisecond)
}

// =============================================================================
// UNIT TESTS - CreateBulkTransactionOperationsAsync - Empty QueueData
// =============================================================================

func TestCreateBulkTransactionOperationsAsync_EmptyQueueData(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Create message with empty QueueData
	messages := []mmodel.Queue{
		{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData:      []mmodel.QueueData{}, // Empty
		},
	}

	result, messageResults, err := uc.CreateBulkTransactionOperationsAsync(ctx, messages, true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
	assert.NotNil(t, result)
	assert.Len(t, messageResults, 1)
	assert.False(t, messageResults[0].Success)
	assert.NotNil(t, messageResults[0].Error)
	assert.Contains(t, messageResults[0].Error.Error(), "expected 1 queue item, got 0")
}

// =============================================================================
// UNIT TESTS - Sorting Functions with Nil Elements
// =============================================================================

func TestSortTransactionsByID_WithNilElements(t *testing.T) {
	t.Parallel()

	transactions := []*transaction.Transaction{
		{ID: "c-transaction"},
		nil, // Nil element
		{ID: "a-transaction"},
		nil, // Another nil element
		{ID: "b-transaction"},
	}

	// Should not panic
	sortTransactionsByID(transactions)

	// Verify nils are sorted to the beginning
	assert.Nil(t, transactions[0])
	assert.Nil(t, transactions[1])
	// Remaining elements should be sorted
	assert.Equal(t, "a-transaction", transactions[2].ID)
	assert.Equal(t, "b-transaction", transactions[3].ID)
	assert.Equal(t, "c-transaction", transactions[4].ID)
}

func TestSortTransactionsByID_AllNil(t *testing.T) {
	t.Parallel()

	transactions := []*transaction.Transaction{nil, nil, nil}

	// Should not panic
	sortTransactionsByID(transactions)

	// All should still be nil
	for _, tx := range transactions {
		assert.Nil(t, tx)
	}
}

func TestSortOperationsByID_WithNilElements(t *testing.T) {
	t.Parallel()

	operations := []*operation.Operation{
		{ID: "z-operation"},
		nil, // Nil element
		{ID: "a-operation"},
		nil, // Another nil element
		{ID: "m-operation"},
	}

	// Should not panic
	sortOperationsByID(operations)

	// Verify nils are sorted to the beginning
	assert.Nil(t, operations[0])
	assert.Nil(t, operations[1])
	// Remaining elements should be sorted
	assert.Equal(t, "a-operation", operations[2].ID)
	assert.Equal(t, "m-operation", operations[3].ID)
	assert.Equal(t, "z-operation", operations[4].ID)
}

func TestSortOperationsByID_AllNil(t *testing.T) {
	t.Parallel()

	operations := []*operation.Operation{nil, nil, nil}

	// Should not panic
	sortOperationsByID(operations)

	// All should still be nil
	for _, op := range operations {
		assert.Nil(t, op)
	}
}

// =============================================================================
// UNIT TESTS - classifyAndExtractEntities
// =============================================================================

func TestClassifyAndExtractEntities_AllNewTransactions(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Create transactions with CREATED status (should go to toInsert)
	createdStatus := constant.CREATED
	tran1 := &transaction.Transaction{
		ID:             uuid.New().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code:        createdStatus,
			Description: &createdStatus,
		},
		Operations: []*operation.Operation{
			{ID: uuid.New().String()},
		},
	}

	payloads := []*transaction.TransactionProcessingPayload{
		{
			Transaction: tran1,
			Validate:    &pkgTransaction.Responses{Pending: false}, // Not a pending transition
		},
	}

	uc := &UseCase{}
	toInsert, toUpdate, operations := uc.classifyAndExtractEntities(payloads, []int{0})

	assert.Len(t, toInsert, 1, "Should have 1 transaction to insert")
	assert.Len(t, toUpdate, 0, "Should have 0 transactions to update")
	assert.Len(t, operations, 1, "Should have 1 operation")
	assert.Equal(t, constant.APPROVED, toInsert[0].Status.Code, "CREATED should be converted to APPROVED")
}

func TestClassifyAndExtractEntities_AllPendingToApproved(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Create transaction with APPROVED status and Validate.Pending=true (should go to toUpdate)
	approvedStatus := constant.APPROVED
	tran1 := &transaction.Transaction{
		ID:             uuid.New().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code:        approvedStatus,
			Description: &approvedStatus,
		},
		Operations: []*operation.Operation{
			{ID: uuid.New().String()},
		},
	}

	payloads := []*transaction.TransactionProcessingPayload{
		{
			Transaction: tran1,
			Validate:    &pkgTransaction.Responses{Pending: true}, // This is a pending transition
		},
	}

	uc := &UseCase{}
	toInsert, toUpdate, operations := uc.classifyAndExtractEntities(payloads, []int{0})

	assert.Len(t, toInsert, 0, "Should have 0 transactions to insert")
	assert.Len(t, toUpdate, 1, "Should have 1 transaction to update")
	assert.Len(t, operations, 1, "Should have 1 operation")
	assert.Equal(t, constant.APPROVED, toUpdate[0].Status.Code)
}

func TestClassifyAndExtractEntities_AllPendingToCanceled(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Create transaction with CANCELED status and Validate.Pending=true (should go to toUpdate)
	canceledStatus := constant.CANCELED
	tran1 := &transaction.Transaction{
		ID:             uuid.New().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code:        canceledStatus,
			Description: &canceledStatus,
		},
		Operations: []*operation.Operation{},
	}

	payloads := []*transaction.TransactionProcessingPayload{
		{
			Transaction: tran1,
			Validate:    &pkgTransaction.Responses{Pending: true}, // This is a pending transition
		},
	}

	uc := &UseCase{}
	toInsert, toUpdate, operations := uc.classifyAndExtractEntities(payloads, []int{0})

	assert.Len(t, toInsert, 0, "Should have 0 transactions to insert")
	assert.Len(t, toUpdate, 1, "Should have 1 transaction to update")
	assert.Len(t, operations, 0, "Should have 0 operations")
	assert.Equal(t, constant.CANCELED, toUpdate[0].Status.Code)
}

func TestClassifyAndExtractEntities_MixedBatch(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Transaction 1: CREATED (goes to toInsert)
	createdStatus := constant.CREATED
	tran1 := &transaction.Transaction{
		ID:             uuid.New().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code:        createdStatus,
			Description: &createdStatus,
		},
		Operations: []*operation.Operation{{ID: uuid.New().String()}},
	}

	// Transaction 2: APPROVED with Pending=true (goes to toUpdate)
	approvedStatus := constant.APPROVED
	tran2 := &transaction.Transaction{
		ID:             uuid.New().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code:        approvedStatus,
			Description: &approvedStatus,
		},
		Operations: []*operation.Operation{{ID: uuid.New().String()}},
	}

	// Transaction 3: NOTED (goes to toInsert, no status change needed)
	notedStatus := constant.NOTED
	tran3 := &transaction.Transaction{
		ID:             uuid.New().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code:        notedStatus,
			Description: &notedStatus,
		},
		Operations: []*operation.Operation{{ID: uuid.New().String()}},
	}

	payloads := []*transaction.TransactionProcessingPayload{
		{Transaction: tran1, Validate: &pkgTransaction.Responses{Pending: false}},
		{Transaction: tran2, Validate: &pkgTransaction.Responses{Pending: true}},
		{Transaction: tran3, Validate: &pkgTransaction.Responses{Pending: false}},
	}

	uc := &UseCase{}
	toInsert, toUpdate, operations := uc.classifyAndExtractEntities(payloads, []int{0, 1, 2})

	assert.Len(t, toInsert, 2, "Should have 2 transactions to insert (CREATED and NOTED)")
	assert.Len(t, toUpdate, 1, "Should have 1 transaction to update (APPROVED with Pending)")
	assert.Len(t, operations, 3, "Should have 3 operations total")
}

func TestClassifyAndExtractEntities_NilValidate(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Create transaction with APPROVED status but nil Validate (should go to toInsert)
	approvedStatus := constant.APPROVED
	tran1 := &transaction.Transaction{
		ID:             uuid.New().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code:        approvedStatus,
			Description: &approvedStatus,
		},
		Operations: []*operation.Operation{},
	}

	payloads := []*transaction.TransactionProcessingPayload{
		{
			Transaction: tran1,
			Validate:    nil, // Nil Validate should not trigger update
		},
	}

	uc := &UseCase{}
	toInsert, toUpdate, _ := uc.classifyAndExtractEntities(payloads, []int{0})

	assert.Len(t, toInsert, 1, "Should have 1 transaction to insert (nil Validate)")
	assert.Len(t, toUpdate, 0, "Should have 0 transactions to update")
}

func TestClassifyAndExtractEntities_ApprovedWithPendingFalse(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Create transaction with APPROVED status but Pending=false (should go to toInsert)
	approvedStatus := constant.APPROVED
	tran1 := &transaction.Transaction{
		ID:             uuid.New().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code:        approvedStatus,
			Description: &approvedStatus,
		},
		Operations: []*operation.Operation{},
	}

	payloads := []*transaction.TransactionProcessingPayload{
		{
			Transaction: tran1,
			Validate:    &pkgTransaction.Responses{Pending: false}, // Pending=false should not trigger update
		},
	}

	uc := &UseCase{}
	toInsert, toUpdate, _ := uc.classifyAndExtractEntities(payloads, []int{0})

	assert.Len(t, toInsert, 1, "Should have 1 transaction to insert (Pending=false)")
	assert.Len(t, toUpdate, 0, "Should have 0 transactions to update")
}

// =============================================================================
// UNIT TESTS - BulkResult with Update Fields
// =============================================================================

func TestBulkResult_UpdateFields(t *testing.T) {
	t.Parallel()

	result := &BulkResult{
		TransactionsAttempted:       10,
		TransactionsInserted:        8,
		TransactionsIgnored:         2,
		TransactionsUpdateAttempted: 5,
		TransactionsUpdated:         5,
		OperationsAttempted:         30,
		OperationsInserted:          28,
		OperationsIgnored:           2,
	}

	assert.Equal(t, int64(10), result.TransactionsAttempted)
	assert.Equal(t, int64(8), result.TransactionsInserted)
	assert.Equal(t, int64(2), result.TransactionsIgnored)
	assert.Equal(t, int64(5), result.TransactionsUpdateAttempted)
	assert.Equal(t, int64(5), result.TransactionsUpdated)
}

// =============================================================================
// UNIT TESTS - Bulk Update Threshold
// =============================================================================

func TestBulkUpdateThreshold_Constant(t *testing.T) {
	t.Parallel()

	// Verify the threshold constant is set to expected value
	assert.Equal(t, 10, bulkUpdateThreshold, "Bulk update threshold should be 10")
}

// =============================================================================
// UNIT TESTS - CreateBulkTransactionOperationsAsync with Updates
// =============================================================================

func TestCreateBulkTransactionOperationsAsync_WithPendingToApproved(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		OperationRepo:   mockOperationRepo,
		MetadataRepo:    mockMetadataRepo,
		BalanceRepo:     mockBalanceRepo,
		RabbitMQRepo:    mockRabbitMQRepo,
		RedisRepo:       mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()

	// Create an APPROVED transaction with Validate.Pending=true (PENDING→APPROVED transition)
	approvedStatus := constant.APPROVED
	tran := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code:        approvedStatus,
			Description: &approvedStatus,
		},
		Operations: []*operation.Operation{
			{ID: uuid.New().String(), TransactionID: transactionID},
		},
	}

	payload := transaction.TransactionProcessingPayload{
		Transaction: tran,
		Validate:    &pkgTransaction.Responses{Pending: true}, // This triggers update instead of insert
	}

	payloadBytes, err := msgpack.Marshal(payload)
	require.NoError(t, err)

	messages := []mmodel.Queue{
		{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData: []mmodel.QueueData{
				{ID: uuid.New(), Value: payloadBytes},
			},
		},
	}

	// Mock balance update
	mockBalanceRepo.EXPECT().
		SyncBatch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(int64(1), nil).
		AnyTimes()

	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// No CreateBulk call expected (toInsert is empty)
	// Individual status update expected (1 transaction < threshold)
	mockTransactionRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(tran, nil)

	// Operations still need to be inserted
	mockOperationRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 1,
			Inserted:  1,
			Ignored:   0,
		}, nil)

	// Mock metadata creation
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock Redis cleanup
	mockRedisRepo.EXPECT().
		RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock RabbitMQ for events
	mockRabbitMQRepo.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	result, messageResults, err := uc.CreateBulkTransactionOperationsAsync(ctx, messages, true)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(0), result.TransactionsInserted, "No inserts expected")
	assert.Equal(t, int64(1), result.TransactionsUpdateAttempted, "1 update attempted")
	assert.Equal(t, int64(1), result.TransactionsUpdated, "1 update succeeded")
	assert.Equal(t, int64(1), result.OperationsInserted)
	assert.False(t, result.FallbackUsed)
	assert.Len(t, messageResults, 1)
	assert.True(t, messageResults[0].Success)

	// Wait for goroutines to complete
	<-time.After(100 * time.Millisecond)
}

func TestCreateBulkTransactionOperationsAsync_MixedInsertAndUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		OperationRepo:   mockOperationRepo,
		MetadataRepo:    mockMetadataRepo,
		BalanceRepo:     mockBalanceRepo,
		RabbitMQRepo:    mockRabbitMQRepo,
		RedisRepo:       mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	createPayload := func(t *testing.T, status string, pending bool) []byte {
		t.Helper()

		tran := &transaction.Transaction{
			ID:             uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Status: transaction.Status{
				Code:        status,
				Description: &status,
			},
			Operations: []*operation.Operation{},
		}
		payload := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    &pkgTransaction.Responses{Pending: pending},
		}
		bytes, err := msgpack.Marshal(payload)
		require.NoError(t, err)

		return bytes
	}

	messages := []mmodel.Queue{
		// Message 1: NOTED transaction (insert)
		{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData:      []mmodel.QueueData{{ID: uuid.New(), Value: createPayload(t, constant.NOTED, false)}},
		},
		// Message 2: APPROVED with Pending=true (update)
		{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData:      []mmodel.QueueData{{ID: uuid.New(), Value: createPayload(t, constant.APPROVED, true)}},
		},
	}

	// Mock for insert (1 NOTED transaction)
	mockTransactionRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 1,
			Inserted:  1,
			Ignored:   0,
		}, nil)

	// Mock for individual update (1 APPROVED transaction)
	mockTransactionRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&transaction.Transaction{}, nil)

	// No operations to insert
	mockOperationRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 0,
			Inserted:  0,
			Ignored:   0,
		}, nil).
		AnyTimes()

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockRedisRepo.EXPECT().
		RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockRabbitMQRepo.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	result, messageResults, err := uc.CreateBulkTransactionOperationsAsync(ctx, messages, true)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(1), result.TransactionsInserted, "1 insert expected")
	assert.Equal(t, int64(1), result.TransactionsUpdateAttempted, "1 update attempted")
	assert.Equal(t, int64(1), result.TransactionsUpdated, "1 update succeeded")
	assert.Len(t, messageResults, 2)

	// Wait for goroutines to complete
	<-time.After(100 * time.Millisecond)
}

func TestCreateBulkTransactionOperationsAsync_UpdateFailureReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		OperationRepo:   mockOperationRepo,
		MetadataRepo:    mockMetadataRepo,
		BalanceRepo:     mockBalanceRepo,
		RabbitMQRepo:    mockRabbitMQRepo,
		RedisRepo:       mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()

	// Create an APPROVED transaction with Validate.Pending=true
	approvedStatus := constant.APPROVED
	tran := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code:        approvedStatus,
			Description: &approvedStatus,
		},
		Operations: []*operation.Operation{},
	}

	payload := transaction.TransactionProcessingPayload{
		Transaction: tran,
		Validate:    &pkgTransaction.Responses{Pending: true},
	}

	payloadBytes, err := msgpack.Marshal(payload)
	require.NoError(t, err)

	messages := []mmodel.Queue{
		{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData: []mmodel.QueueData{
				{ID: uuid.New(), Value: payloadBytes},
			},
		},
	}

	// Mock balance update
	mockBalanceRepo.EXPECT().
		SyncBatch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(int64(1), nil).
		AnyTimes()

	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock update failure
	updateErr := errors.New("database connection lost")
	mockTransactionRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, updateErr)

	// Fallback disabled - should return error
	result, messageResults, err := uc.CreateBulkTransactionOperationsAsync(ctx, messages, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction status update failed")
	assert.NotNil(t, result)
	assert.Equal(t, int64(1), result.TransactionsUpdateAttempted)
	assert.Equal(t, int64(0), result.TransactionsUpdated)
	assert.Len(t, messageResults, 1)
	assert.False(t, messageResults[0].Success)
}
