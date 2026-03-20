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

	mockOperationRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 0,
			Inserted:  0,
			Ignored:   0,
		}, nil)

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

	mockOperationRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 0,
			Inserted:  0,
			Ignored:   0,
		}, nil)

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

	mockOperationRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 0,
			Inserted:  0,
			Ignored:   0,
		}, nil)

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
	assert.Contains(t, messageResults[0].Error.Error(), "empty QueueData")
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
