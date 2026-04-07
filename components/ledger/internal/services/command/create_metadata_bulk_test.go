// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/repository"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestMetadataEntry_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		entry   MetadataEntry
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid entry",
			entry: MetadataEntry{
				EntityID:   uuid.New().String(),
				Collection: "Transaction",
				Data:       map[string]any{"key": "value"},
			},
			wantErr: false,
		},
		{
			name: "empty entity ID",
			entry: MetadataEntry{
				EntityID:   "",
				Collection: "Transaction",
				Data:       map[string]any{"key": "value"},
			},
			wantErr: true,
			errMsg:  "entity ID is required",
		},
		{
			name: "invalid UUID format",
			entry: MetadataEntry{
				EntityID:   "not-a-uuid",
				Collection: "Transaction",
				Data:       map[string]any{"key": "value"},
			},
			wantErr: true,
			errMsg:  "invalid entity ID format",
		},
		{
			name: "empty collection",
			entry: MetadataEntry{
				EntityID:   uuid.New().String(),
				Collection: "",
				Data:       map[string]any{"key": "value"},
			},
			wantErr: true,
			errMsg:  "collection is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.entry.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateMetadataBulk_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	// Create test metadata entries
	entries := []MetadataEntry{
		{
			EntityID:   uuid.New().String(),
			Collection: "Transaction",
			Data:       map[string]any{"key1": "value1"},
		},
		{
			EntityID:   uuid.New().String(),
			Collection: "Transaction",
			Data:       map[string]any{"key2": "value2"},
		},
		{
			EntityID:   uuid.New().String(),
			Collection: "Operation",
			Data:       map[string]any{"key3": "value3"},
		},
	}

	// Expect CreateBulk to be called once per collection
	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Transaction", gomock.Len(2)).
		Return(&repository.MongoDBBulkInsertResult{
			Attempted: 2,
			Inserted:  2,
			Matched:   0,
		}, nil).
		Times(1)

	// Operation has single entry, so Create is used instead of CreateBulk
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), "Operation", gomock.Any()).
		Return(nil).
		Times(1)

	err := uc.createMetadataBulk(ctx, entries)

	require.NoError(t, err)
}

func TestCreateMetadataBulk_EmptyEntries(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	ctx := context.Background()

	// Empty entries should return nil without calling repo
	err := uc.createMetadataBulk(ctx, []MetadataEntry{})

	require.NoError(t, err)
}

func TestCreateMetadataBulk_NilMetadataSkipped(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	validEntityID := uuid.New().String()

	// Create entries with some nil Data (should be skipped)
	entries := []MetadataEntry{
		{
			EntityID:   uuid.New().String(),
			Collection: "Transaction",
			Data:       nil, // Should be skipped
		},
		{
			EntityID:   validEntityID,
			Collection: "Transaction",
			Data:       map[string]any{"key1": "value1"},
		},
	}

	// Only 1 entry should be processed (the one with non-nil Data)
	// Single entry uses Create instead of CreateBulk
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), "Transaction", gomock.Any()).
		Return(nil).
		Times(1)

	err := uc.createMetadataBulk(ctx, entries)

	require.NoError(t, err)
}

func TestCreateMetadataBulk_AllNilData(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	ctx := context.Background()

	// All entries have nil Data - should return nil without repo calls
	entries := []MetadataEntry{
		{EntityID: uuid.New().String(), Collection: "Transaction", Data: nil},
		{EntityID: uuid.New().String(), Collection: "Operation", Data: nil},
	}

	err := uc.createMetadataBulk(ctx, entries)

	require.NoError(t, err)
}

func TestCreateMetadataBulk_InfrastructureError_SkipsFallback(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	entries := []MetadataEntry{
		{
			EntityID:   uuid.New().String(),
			Collection: "Transaction",
			Data:       map[string]any{"key1": "value1"},
		},
		{
			EntityID:   uuid.New().String(),
			Collection: "Transaction",
			Data:       map[string]any{"key2": "value2"},
		},
	}

	// CreateBulk fails with a context timeout (infrastructure error)
	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Transaction", gomock.Any()).
		Return(nil, context.DeadlineExceeded).
		Times(1)

	// Create should NOT be called — infrastructure errors skip fallback.
	// gomock strict controller will panic if Create is called unexpectedly.

	err := uc.createMetadataBulk(ctx, entries)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create 2 of 2 metadata entries")
}

func TestCreateMetadataBulk_FallbackOnBulkFailure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	entries := []MetadataEntry{
		{
			EntityID:   uuid.New().String(),
			Collection: "Transaction",
			Data:       map[string]any{"key1": "value1"},
		},
		{
			EntityID:   uuid.New().String(),
			Collection: "Transaction",
			Data:       map[string]any{"key2": "value2"},
		},
	}

	// CreateBulk fails
	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Transaction", gomock.Any()).
		Return(nil, errors.New("bulk insert failed")).
		Times(1)

	// Fallback: individual Create calls for each entry
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), "Transaction", gomock.Any()).
		Return(nil).
		Times(2)

	err := uc.createMetadataBulk(ctx, entries)

	require.NoError(t, err)
}

func TestCreateMetadataBulk_FallbackPartialFailure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	entries := []MetadataEntry{
		{
			EntityID:   uuid.New().String(),
			Collection: "Transaction",
			Data:       map[string]any{"key1": "value1"},
		},
		{
			EntityID:   uuid.New().String(),
			Collection: "Transaction",
			Data:       map[string]any{"key2": "value2"},
		},
	}

	// CreateBulk fails
	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Transaction", gomock.Any()).
		Return(nil, errors.New("bulk insert failed")).
		Times(1)

	// Fallback: first Create succeeds, second fails
	gomock.InOrder(
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), "Transaction", gomock.Any()).
			Return(nil),
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), "Transaction", gomock.Any()).
			Return(errors.New("individual create failed")),
	)

	err := uc.createMetadataBulk(ctx, entries)

	// Should return error for partial failure in fallback
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create 1 of 2 metadata entries")
}

func TestCreateMetadataBulk_SingleEntry_UsesCreate(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	// Single entry should use Create, not CreateBulk (optimization)
	entries := []MetadataEntry{
		{
			EntityID:   uuid.New().String(),
			Collection: "Transaction",
			Data:       map[string]any{"key1": "value1"},
		},
	}

	// Single entry uses Create directly (not CreateBulk)
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), "Transaction", gomock.Any()).
		Return(nil).
		Times(1)

	err := uc.createMetadataBulk(ctx, entries)

	require.NoError(t, err)
}

func TestCreateMetadataBulk_PartialSuccess_ReturnsInsertedCount(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	// Create 5 entries - simulate 3 inserted, 2 matched (duplicates)
	entries := make([]MetadataEntry, 5)
	for i := range entries {
		entries[i] = MetadataEntry{
			EntityID:   uuid.New().String(),
			Collection: "Transaction",
			Data:       map[string]any{fmt.Sprintf("key_%d", i): fmt.Sprintf("value_%d", i)},
		}
	}

	// CreateBulk returns partial success (3 inserted, 2 already existed)
	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Transaction", gomock.Len(5)).
		Return(&repository.MongoDBBulkInsertResult{
			Attempted: 5,
			Inserted:  3,
			Matched:   2,
		}, nil).
		Times(1)

	err := uc.createMetadataBulk(ctx, entries)

	// Partial success should NOT return error - duplicates are OK
	require.NoError(t, err)
}

func TestCreateMetadataBulk_MultipleCollections_ProcessesAll(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	// Create entries for 3 different collections
	entries := []MetadataEntry{
		{EntityID: uuid.New().String(), Collection: "Transaction", Data: map[string]any{"a": 1}},
		{EntityID: uuid.New().String(), Collection: "Transaction", Data: map[string]any{"b": 2}},
		{EntityID: uuid.New().String(), Collection: "Operation", Data: map[string]any{"c": 3}},
		{EntityID: uuid.New().String(), Collection: "Balance", Data: map[string]any{"d": 4}},
	}

	// Expect CreateBulk for Transaction (2 entries)
	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Transaction", gomock.Len(2)).
		Return(&repository.MongoDBBulkInsertResult{Attempted: 2, Inserted: 2}, nil).
		Times(1)

	// Expect Create for Operation (1 entry - uses single create)
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), "Operation", gomock.Any()).
		Return(nil).
		Times(1)

	// Expect Create for Balance (1 entry - uses single create)
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), "Balance", gomock.Any()).
		Return(nil).
		Times(1)

	err := uc.createMetadataBulk(ctx, entries)

	require.NoError(t, err)
}

func TestCreateMetadataBulk_InvalidEntityID_ReturnsError(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	ctx := context.Background()

	// Entry with empty EntityID should fail validation
	entries := []MetadataEntry{
		{
			EntityID:   "",
			Collection: "Transaction",
			Data:       map[string]any{"key": "value"},
		},
	}

	err := uc.createMetadataBulk(ctx, entries)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "entity ID is required")
}

func TestCreateMetadataBulk_InvalidUUIDFormat_ReturnsError(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	ctx := context.Background()

	// Entry with invalid UUID format should fail validation
	entries := []MetadataEntry{
		{
			EntityID:   "not-a-valid-uuid",
			Collection: "Transaction",
			Data:       map[string]any{"key": "value"},
		},
	}

	err := uc.createMetadataBulk(ctx, entries)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid entity ID format")
}

func TestCreateMetadataBulk_EmptyCollection_ReturnsError(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	ctx := context.Background()

	// Entry with empty collection should fail validation
	entries := []MetadataEntry{
		{
			EntityID:   uuid.New().String(),
			Collection: "",
			Data:       map[string]any{"key": "value"},
		},
	}

	err := uc.createMetadataBulk(ctx, entries)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "collection is required")
}

func TestCreateMetadataBulk_ExceedsMaxEntries_ChunksProcessing(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	// Create more entries than maxBulkMetadataEntries to verify chunking.
	// Use a small count above the limit to keep the test fast.
	entryCount := maxBulkMetadataEntries + 5
	entries := make([]MetadataEntry, entryCount)
	for i := range entries {
		entries[i] = MetadataEntry{
			EntityID:   uuid.New().String(),
			Collection: "Transaction",
			Data:       map[string]any{"key": "value"},
		}
	}

	// Expect two CreateBulk calls: one for the first chunk (maxBulkMetadataEntries)
	// and one for the remaining 5 entries.
	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Transaction", gomock.Len(maxBulkMetadataEntries)).
		Return(&repository.MongoDBBulkInsertResult{
			Attempted: int64(maxBulkMetadataEntries),
			Inserted:  int64(maxBulkMetadataEntries),
		}, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Transaction", gomock.Len(5)).
		Return(&repository.MongoDBBulkInsertResult{
			Attempted: 5,
			Inserted:  5,
		}, nil).
		Times(1)

	err := uc.createMetadataBulk(ctx, entries)

	require.NoError(t, err)
}

// TestCollectMetadataFromPayloads_Success tests that metadata entries are correctly
// collected from transaction payloads for bulk processing.
func TestCollectMetadataFromPayloads_Success(t *testing.T) {
	t.Parallel()

	tx1ID := uuid.New().String()
	tx2ID := uuid.New().String()
	op1ID := uuid.New().String()
	op2ID := uuid.New().String()
	op3ID := uuid.New().String()

	payloads := []transaction.TransactionProcessingPayload{
		{
			Transaction: &transaction.Transaction{
				ID:       tx1ID,
				Metadata: map[string]any{"tx1_key": "tx1_value"},
				Operations: []*operation.Operation{
					{
						ID:       op1ID,
						Metadata: map[string]any{"op1_key": "op1_value"},
					},
					{
						ID:       op2ID,
						Metadata: map[string]any{"op2_key": "op2_value"},
					},
				},
			},
		},
		{
			Transaction: &transaction.Transaction{
				ID:       tx2ID,
				Metadata: map[string]any{"tx2_key": "tx2_value"},
				Operations: []*operation.Operation{
					{
						ID:       op3ID,
						Metadata: map[string]any{"op3_key": "op3_value"},
					},
				},
			},
		},
	}

	// All transactions were inserted
	insertedTxIDs := map[string]struct{}{
		tx1ID: {},
		tx2ID: {},
	}

	entries := collectMetadataFromPayloads(payloads, insertedTxIDs)

	// Should have 2 transaction entries + 3 operation entries = 5 total
	require.Len(t, entries, 5)

	// Verify transaction entries
	txEntries := filterEntriesByCollection(entries, reflect.TypeOf(transaction.Transaction{}).Name())
	require.Len(t, txEntries, 2)

	// Verify operation entries
	opEntries := filterEntriesByCollection(entries, reflect.TypeOf(operation.Operation{}).Name())
	require.Len(t, opEntries, 3)
}

// TestCollectMetadataFromPayloads_SkipsDuplicateTxMetadata tests that transaction-level
// metadata for transactions not in insertedTxIDs is skipped, while their operation
// metadata is still collected.
func TestCollectMetadataFromPayloads_SkipsDuplicateTxMetadata(t *testing.T) {
	t.Parallel()

	tx1ID := uuid.New().String()
	tx2ID := uuid.New().String()
	op1ID := uuid.New().String()
	op2ID := uuid.New().String()

	payloads := []transaction.TransactionProcessingPayload{
		{
			Transaction: &transaction.Transaction{
				ID:       tx1ID,
				Metadata: map[string]any{"tx1_key": "tx1_value"},
				Operations: []*operation.Operation{
					{
						ID:       op1ID,
						Metadata: map[string]any{"op1_key": "op1_value"},
					},
				},
			},
		},
		{
			Transaction: &transaction.Transaction{
				ID:       tx2ID, // Not in insertedTxIDs (duplicate/status-transition)
				Metadata: map[string]any{"tx2_key": "tx2_value"},
				Operations: []*operation.Operation{
					{
						ID:       op2ID,
						Metadata: map[string]any{"op2_key": "op2_value"},
					},
				},
			},
		},
	}

	// Only tx1 was inserted, tx2 was a duplicate
	insertedTxIDs := map[string]struct{}{
		tx1ID: {},
	}

	entries := collectMetadataFromPayloads(payloads, insertedTxIDs)

	transactionTypeName := reflect.TypeOf(transaction.Transaction{}).Name()
	operationTypeName := reflect.TypeOf(operation.Operation{}).Name()

	// tx-level metadata: only tx1 (1 entry). tx2's metadata is skipped.
	txEntries := filterEntriesByCollection(entries, transactionTypeName)
	require.Len(t, txEntries, 1)
	assert.Equal(t, tx1ID, txEntries[0].EntityID)

	// op-level metadata: both op1 and op2 (2 entries). Operations are always collected.
	opEntries := filterEntriesByCollection(entries, operationTypeName)
	require.Len(t, opEntries, 2)
}

// TestCollectMetadataFromPayloads_MixedInsertAndStatusTransition tests that in a batch
// containing both newly inserted transactions and status-transitioned (updated) ones,
// operation metadata is preserved for all payloads while transaction-level metadata is
// only collected for newly inserted transactions.
func TestCollectMetadataFromPayloads_MixedInsertAndStatusTransition(t *testing.T) {
	t.Parallel()

	// tx1 is newly inserted (present in insertedTxIDs)
	tx1ID := uuid.New().String()
	op1ID := uuid.New().String()
	op2ID := uuid.New().String()

	// tx2 is a status-transition (NOT in insertedTxIDs)
	tx2ID := uuid.New().String()
	op3ID := uuid.New().String()

	payloads := []transaction.TransactionProcessingPayload{
		{
			Transaction: &transaction.Transaction{
				ID:       tx1ID,
				Metadata: map[string]any{"tx1_key": "tx1_value"},
				Operations: []*operation.Operation{
					{ID: op1ID, Metadata: map[string]any{"op1_key": "op1_value"}},
					{ID: op2ID, Metadata: map[string]any{"op2_key": "op2_value"}},
				},
			},
		},
		{
			Transaction: &transaction.Transaction{
				ID:       tx2ID,
				Metadata: map[string]any{"tx2_key": "tx2_value"},
				Operations: []*operation.Operation{
					{ID: op3ID, Metadata: map[string]any{"op3_key": "op3_value"}},
				},
			},
		},
	}

	// Only tx1 was newly inserted; tx2 is a status-transition (update)
	insertedTxIDs := map[string]struct{}{
		tx1ID: {},
	}

	entries := collectMetadataFromPayloads(payloads, insertedTxIDs)

	transactionTypeName := reflect.TypeOf(transaction.Transaction{}).Name()
	operationTypeName := reflect.TypeOf(operation.Operation{}).Name()

	// Transaction metadata: only tx1 (newly inserted), NOT tx2 (status-transition)
	txEntries := filterEntriesByCollection(entries, transactionTypeName)
	require.Len(t, txEntries, 1, "only the newly inserted transaction should have tx-level metadata")
	assert.Equal(t, tx1ID, txEntries[0].EntityID)

	// Operation metadata: all 3 operations from BOTH transactions
	opEntries := filterEntriesByCollection(entries, operationTypeName)
	require.Len(t, opEntries, 3, "operations from both inserted and status-transitioned transactions must be collected")

	opIDs := make(map[string]bool, len(opEntries))
	for _, e := range opEntries {
		opIDs[e.EntityID] = true
	}

	assert.True(t, opIDs[op1ID], "op1 from inserted tx1 should be present")
	assert.True(t, opIDs[op2ID], "op2 from inserted tx1 should be present")
	assert.True(t, opIDs[op3ID], "op3 from status-transitioned tx2 should be present")
}

// TestCollectMetadataFromPayloads_SkipsNilMetadata tests that entries with nil
// metadata are not collected.
func TestCollectMetadataFromPayloads_SkipsNilMetadata(t *testing.T) {
	t.Parallel()

	tx1ID := uuid.New().String()
	op1ID := uuid.New().String()

	payloads := []transaction.TransactionProcessingPayload{
		{
			Transaction: &transaction.Transaction{
				ID:       tx1ID,
				Metadata: nil, // No transaction metadata
				Operations: []*operation.Operation{
					{
						ID:       op1ID,
						Metadata: map[string]any{"op1_key": "op1_value"},
					},
				},
			},
		},
	}

	insertedTxIDs := map[string]struct{}{
		tx1ID: {},
	}

	entries := collectMetadataFromPayloads(payloads, insertedTxIDs)

	// Should have only 1 operation entry (transaction metadata was nil)
	require.Len(t, entries, 1)
	assert.Equal(t, op1ID, entries[0].EntityID)
}

// TestCollectMetadataFromPayloads_EmptyInsertedTxIDs tests that when insertedTxIDs
// is empty, all payloads are processed (fallback/status-update scenarios).
func TestCollectMetadataFromPayloads_EmptyInsertedTxIDs(t *testing.T) {
	t.Parallel()

	tx1ID := uuid.New().String()
	tx2ID := uuid.New().String()

	payloads := []transaction.TransactionProcessingPayload{
		{
			Transaction: &transaction.Transaction{
				ID:       tx1ID,
				Metadata: map[string]any{"tx1_key": "tx1_value"},
			},
		},
		{
			Transaction: &transaction.Transaction{
				ID:       tx2ID,
				Metadata: map[string]any{"tx2_key": "tx2_value"},
			},
		},
	}

	// Empty insertedTxIDs means process all
	insertedTxIDs := map[string]struct{}{}

	entries := collectMetadataFromPayloads(payloads, insertedTxIDs)

	// Should have 2 transaction entries (all processed when insertedTxIDs is empty)
	require.Len(t, entries, 2)
}

// filterEntriesByCollection is a test helper to filter metadata entries by collection.
func filterEntriesByCollection(entries []MetadataEntry, collection string) []MetadataEntry {
	var result []MetadataEntry

	for _, e := range entries {
		if e.Collection == collection {
			result = append(result, e)
		}
	}

	return result
}

// TestProcessMetadataAndEventsBulk_UsesBulkOperations tests that processMetadataAndEventsBulk
// uses bulk operations to create metadata for multiple payloads in a single batch.
func TestProcessMetadataAndEventsBulk_UsesBulkOperations(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	// Create test payloads with multiple transactions and operations
	tx1ID := uuid.New().String()
	tx2ID := uuid.New().String()
	op1ID := uuid.New().String()
	op2ID := uuid.New().String()
	op3ID := uuid.New().String()

	payloads := []transaction.TransactionProcessingPayload{
		{
			Transaction: &transaction.Transaction{
				ID:       tx1ID,
				Metadata: map[string]any{"tx1_key": "tx1_value"},
				Operations: []*operation.Operation{
					{ID: op1ID, Metadata: map[string]any{"op1_key": "op1_value"}},
					{ID: op2ID, Metadata: map[string]any{"op2_key": "op2_value"}},
				},
			},
		},
		{
			Transaction: &transaction.Transaction{
				ID:       tx2ID,
				Metadata: map[string]any{"tx2_key": "tx2_value"},
				Operations: []*operation.Operation{
					{ID: op3ID, Metadata: map[string]any{"op3_key": "op3_value"}},
				},
			},
		},
	}

	insertedTxIDs := map[string]struct{}{
		tx1ID: {},
		tx2ID: {},
	}

	// Expect CreateBulk for Transaction collection (2 entries)
	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Transaction", gomock.Len(2)).
		Return(&repository.MongoDBBulkInsertResult{
			Attempted: 2,
			Inserted:  2,
		}, nil).
		Times(1)

	// Expect CreateBulk for Operation collection (3 entries)
	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Operation", gomock.Len(3)).
		Return(&repository.MongoDBBulkInsertResult{
			Attempted: 3,
			Inserted:  3,
		}, nil).
		Times(1)

	// Call the bulk processing method (no error return - logs warnings internally)
	uc.processMetadataAndEventsBulk(ctx, nil, payloads, insertedTxIDs)
}

// TestProcessMetadataAndEventsBulk_SkipsDuplicateTxMetadata tests that duplicate transaction
// metadata (those not in insertedTxIDs) is not processed, while their operation metadata is
// still created.
func TestProcessMetadataAndEventsBulk_SkipsDuplicateTxMetadata(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	tx1ID := uuid.New().String()
	tx2ID := uuid.New().String() // Not in insertedTxIDs (duplicate/status-transition)
	op1ID := uuid.New().String()
	op2ID := uuid.New().String()

	payloads := []transaction.TransactionProcessingPayload{
		{
			Transaction: &transaction.Transaction{
				ID:       tx1ID,
				Metadata: map[string]any{"tx1_key": "tx1_value"},
				Operations: []*operation.Operation{
					{ID: op1ID, Metadata: map[string]any{"op1_key": "op1_value"}},
				},
			},
		},
		{
			Transaction: &transaction.Transaction{
				ID:       tx2ID,
				Metadata: map[string]any{"tx2_key": "tx2_value"},
				Operations: []*operation.Operation{
					{ID: op2ID, Metadata: map[string]any{"op2_key": "op2_value"}},
				},
			},
		},
	}

	// Only tx1 was inserted
	insertedTxIDs := map[string]struct{}{
		tx1ID: {},
	}

	// Only 1 transaction metadata entry (tx1), single entry uses Create
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), "Transaction", gomock.Any()).
		Return(nil).
		Times(1)

	// 2 operation metadata entries (op1 + op2) — operations are always collected
	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Operation", gomock.Len(2)).
		Return(&repository.MongoDBBulkInsertResult{
			Attempted: 2,
			Inserted:  2,
		}, nil).
		Times(1)

	uc.processMetadataAndEventsBulk(ctx, nil, payloads, insertedTxIDs)
}

// TestProcessMetadataAndEventsBulk_EmptyPayloads tests that empty payloads
// are handled gracefully.
func TestProcessMetadataAndEventsBulk_EmptyPayloads(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	ctx := context.Background()

	// Should not panic or cause issues
	uc.processMetadataAndEventsBulk(ctx, nil, []transaction.TransactionProcessingPayload{}, nil)
}

// TestProcessMetadataAndEventsBulk_HandlesAllNilMetadata tests that payloads
// with all nil metadata are handled gracefully.
func TestProcessMetadataAndEventsBulk_HandlesAllNilMetadata(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	ctx := context.Background()

	tx1ID := uuid.New().String()

	payloads := []transaction.TransactionProcessingPayload{
		{
			Transaction: &transaction.Transaction{
				ID:       tx1ID,
				Metadata: nil, // No metadata
				Operations: []*operation.Operation{
					{ID: uuid.New().String(), Metadata: nil}, // No metadata
				},
			},
		},
	}

	insertedTxIDs := map[string]struct{}{
		tx1ID: {},
	}

	// Should not panic or cause issues
	uc.processMetadataAndEventsBulk(ctx, nil, payloads, insertedTxIDs)
}
