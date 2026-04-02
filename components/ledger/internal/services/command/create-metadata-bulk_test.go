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

func TestCreateMetadataBulk_ExceedsMaxEntries_ReturnsError(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	ctx := context.Background()

	// Create more entries than the maximum allowed
	entries := make([]MetadataEntry, maxBulkMetadataEntries+1)
	for i := range entries {
		entries[i] = MetadataEntry{
			EntityID:   uuid.New().String(),
			Collection: "Transaction",
			Data:       map[string]any{"key": "value"},
		}
	}

	err := uc.createMetadataBulk(ctx, entries)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "bulk metadata entries exceed limit")
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

// TestCollectMetadataFromPayloads_SkipsDuplicates tests that transactions not in
// insertedTxIDs are skipped during metadata collection.
func TestCollectMetadataFromPayloads_SkipsDuplicates(t *testing.T) {
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
				ID:       tx2ID, // This one was a duplicate, not in insertedTxIDs
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

	// Should have 1 transaction entry + 1 operation entry = 2 total
	// tx2 and its operation should be skipped
	require.Len(t, entries, 2)
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

// TestProcessMetadataAndEventsBulk_SkipsDuplicates tests that duplicate transactions
// (those not in insertedTxIDs) are not processed.
func TestProcessMetadataAndEventsBulk_SkipsDuplicates(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	tx1ID := uuid.New().String()
	tx2ID := uuid.New().String() // This will be a duplicate
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
				ID:       tx2ID, // Duplicate - not in insertedTxIDs
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

	// Only 1 transaction should be processed (single entry uses Create)
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), "Transaction", gomock.Any()).
		Return(nil).
		Times(1)

	// Only 1 operation should be processed (single entry uses Create)
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), "Operation", gomock.Any()).
		Return(nil).
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
