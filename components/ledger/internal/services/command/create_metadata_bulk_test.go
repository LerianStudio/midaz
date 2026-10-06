// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
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

	failed, err := uc.createMetadataBulk(ctx, entries)

	require.NoError(t, err)
	assert.Empty(t, failed)
}

func TestCreateMetadataBulk_EmptyEntries(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	ctx := context.Background()

	// Empty entries should return nil without calling repo
	failed, err := uc.createMetadataBulk(ctx, []MetadataEntry{})

	require.NoError(t, err)
	assert.Empty(t, failed)
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

	failed, err := uc.createMetadataBulk(ctx, entries)

	require.NoError(t, err)
	assert.Empty(t, failed)
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

	failed, err := uc.createMetadataBulk(ctx, entries)

	require.NoError(t, err)
	assert.Empty(t, failed)
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

	failed, err := uc.createMetadataBulk(ctx, entries)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create 2 of 2 metadata entries")
	assert.ElementsMatch(t, entries, failed, "an infrastructure error leaves every entry of the collection unconfirmed")
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

	failed, err := uc.createMetadataBulk(ctx, entries)

	require.NoError(t, err)
	assert.Empty(t, failed)
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

	failed, err := uc.createMetadataBulk(ctx, entries)

	// Should return error for partial failure in fallback
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create 1 of 2 metadata entries")
	assert.Equal(t, []MetadataEntry{entries[1]}, failed, "only the entry whose individual create failed is unconfirmed")
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

	failed, err := uc.createMetadataBulk(ctx, entries)

	require.NoError(t, err)
	assert.Empty(t, failed)
}

func TestCreateMetadataBulk_SingleEntryFailure_ReturnsEntry(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	txID := uuid.New().String()

	entries := []MetadataEntry{
		{
			EntityID:      txID,
			Collection:    "Transaction",
			Data:          map[string]any{"key1": "value1"},
			TransactionID: txID,
		},
	}

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), "Transaction", gomock.Any()).
		Return(errors.New("create failed")).
		Times(1)

	failed, err := uc.createMetadataBulk(ctx, entries)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create 1 of 1 metadata entries")
	assert.Equal(t, entries, failed)
}

func TestCreateMetadataBulk_FailureInOneCollection_ReturnsOnlyItsEntries(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	txID := uuid.New().String()

	txEntry := MetadataEntry{EntityID: txID, Collection: "Transaction", Data: map[string]any{"a": 1}, TransactionID: txID}
	opEntry1 := MetadataEntry{EntityID: uuid.New().String(), Collection: "Operation", Data: map[string]any{"b": 2}, TransactionID: txID}
	opEntry2 := MetadataEntry{EntityID: uuid.New().String(), Collection: "Operation", Data: map[string]any{"c": 3}, TransactionID: txID}

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), "Transaction", gomock.Any()).
		Return(nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Operation", gomock.Len(2)).
		Return(nil, context.DeadlineExceeded).
		Times(1)

	failed, err := uc.createMetadataBulk(ctx, []MetadataEntry{txEntry, opEntry1, opEntry2})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create 2 of 3 metadata entries")
	assert.ElementsMatch(t, []MetadataEntry{opEntry1, opEntry2}, failed)
}

func TestCreateMetadataBulk_InvalidEntryInMixedBatch_ReturnsEveryDataEntry(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	ctx := context.Background()

	validEntry := MetadataEntry{EntityID: uuid.New().String(), Collection: "Transaction", Data: map[string]any{"a": 1}}
	nilDataEntry := MetadataEntry{EntityID: uuid.New().String(), Collection: "Transaction", Data: nil}
	invalidEntry := MetadataEntry{EntityID: "not-a-valid-uuid", Collection: "Operation", Data: map[string]any{"b": 2}}

	// Validation aborts before any write, so the repository is never called.
	failed, err := uc.createMetadataBulk(ctx, []MetadataEntry{validEntry, nilDataEntry, invalidEntry})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid metadata entry at index 2")
	assert.Contains(t, err.Error(), "invalid entity ID format")
	assert.Equal(t, []MetadataEntry{validEntry, invalidEntry}, failed,
		"every entry carrying data is unconfirmed, not only the invalid one")
}

func TestCreateMetadataBulk_FallbackAllFail_ReturnsAllEntries(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	entries := []MetadataEntry{
		{EntityID: uuid.New().String(), Collection: "Transaction", Data: map[string]any{"a": 1}},
		{EntityID: uuid.New().String(), Collection: "Transaction", Data: map[string]any{"b": 2}},
	}

	// Document-level bulk error triggers the fallback; every individual create fails.
	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Transaction", gomock.Len(2)).
		Return(nil, errors.New("bulk insert failed")).
		Times(1)

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), "Transaction", gomock.Any()).
		Return(errors.New("individual create failed")).
		Times(4)

	failed, err := uc.createMetadataBulk(ctx, entries)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create 2 of 2 metadata entries")
	assert.Equal(t, entries, failed)

	fallbackFailed, fallbackErr := uc.fallbackToIndividualMetadataCreate(ctx, nil, "Transaction", entries)

	require.Error(t, fallbackErr)
	assert.Contains(t, fallbackErr.Error(), "failed to create 2 of 2 metadata entries in fallback")
	assert.Equal(t, entries, fallbackFailed)
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

	failed, err := uc.createMetadataBulk(ctx, entries)

	// Partial success should NOT return error - duplicates are OK
	require.NoError(t, err)
	assert.Empty(t, failed)
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

	failed, err := uc.createMetadataBulk(ctx, entries)

	require.NoError(t, err)
	assert.Empty(t, failed)
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

	failed, err := uc.createMetadataBulk(ctx, entries)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "entity ID is required")
	assert.Equal(t, entries, failed, "an invalid entry aborts the batch before any write")
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

	failed, err := uc.createMetadataBulk(ctx, entries)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid entity ID format")
	assert.Equal(t, entries, failed, "an invalid entry aborts the batch before any write")
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

	failed, err := uc.createMetadataBulk(ctx, entries)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "collection is required")
	assert.Equal(t, entries, failed, "an invalid entry aborts the batch before any write")
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

	failed, err := uc.createMetadataBulk(ctx, entries)

	require.NoError(t, err)
	assert.Empty(t, failed)
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

	entries := collectMetadataFromPayloads(payloads)

	// Should have 2 transaction entries + 3 operation entries = 5 total
	require.Len(t, entries, 5)

	// Verify transaction entries
	txEntries := filterEntriesByCollection(entries, constant.EntityTransaction)
	require.Len(t, txEntries, 2)

	// Verify operation entries
	opEntries := filterEntriesByCollection(entries, constant.EntityOperation)
	require.Len(t, opEntries, 3)
}

// TestCollectMetadataFromPayloads_AttributesEntriesToOwningTransaction tests that every
// entry carries the transaction that owns it: the transaction itself for transaction
// metadata and the parent transaction for operation metadata.
func TestCollectMetadataFromPayloads_AttributesEntriesToOwningTransaction(t *testing.T) {
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
					{ID: op1ID, Metadata: map[string]any{"op1_key": "op1_value"}},
					{ID: op2ID, Metadata: map[string]any{"op2_key": "op2_value"}},
				},
			},
		},
		{
			Transaction: &transaction.Transaction{
				ID: tx2ID,
				Operations: []*operation.Operation{
					{ID: op3ID, Metadata: map[string]any{"op3_key": "op3_value"}},
				},
			},
		},
	}

	entries := collectMetadataFromPayloads(payloads)

	owners := make(map[string]string, len(entries))
	for _, e := range entries {
		owners[e.EntityID] = e.TransactionID
	}

	assert.Equal(t, map[string]string{
		tx1ID: tx1ID,
		op1ID: tx1ID,
		op2ID: tx1ID,
		op3ID: tx2ID,
	}, owners)
}

// TestCollectMetadataFromPayloads_CollectsDuplicateTxMetadata tests that transaction-level
// metadata is collected for a transaction that is already persisted (redelivery), so a
// previously unconfirmed write can be repaired.
func TestCollectMetadataFromPayloads_CollectsDuplicateTxMetadata(t *testing.T) {
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
				ID:       tx2ID, // Already persisted (duplicate on insert)
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

	entries := collectMetadataFromPayloads(payloads)

	// tx-level metadata: both tx1 and the duplicate tx2.
	txEntries := filterEntriesByCollection(entries, constant.EntityTransaction)
	require.Len(t, txEntries, 2)

	txIDs := []string{txEntries[0].EntityID, txEntries[1].EntityID}
	assert.ElementsMatch(t, []string{tx1ID, tx2ID}, txIDs)

	// op-level metadata: both op1 and op2.
	opEntries := filterEntriesByCollection(entries, constant.EntityOperation)
	require.Len(t, opEntries, 2)
}

// TestCollectMetadataFromPayloads_MixedInsertAndStatusTransition tests that in a batch
// containing both newly inserted transactions and status-transitioned (updated) ones,
// transaction and operation metadata are collected for all payloads.
func TestCollectMetadataFromPayloads_MixedInsertAndStatusTransition(t *testing.T) {
	t.Parallel()

	// tx1 is newly inserted
	tx1ID := uuid.New().String()
	op1ID := uuid.New().String()
	op2ID := uuid.New().String()

	// tx2 is a status-transition
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

	entries := collectMetadataFromPayloads(payloads)

	// Transaction metadata: both tx1 (inserted) and tx2 (status-transition)
	txEntries := filterEntriesByCollection(entries, constant.EntityTransaction)
	require.Len(t, txEntries, 2, "inserted and status-transitioned transactions must both have tx-level metadata")

	txIDs := []string{txEntries[0].EntityID, txEntries[1].EntityID}
	assert.ElementsMatch(t, []string{tx1ID, tx2ID}, txIDs)

	// Operation metadata: all 3 operations from BOTH transactions
	opEntries := filterEntriesByCollection(entries, constant.EntityOperation)
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

	entries := collectMetadataFromPayloads(payloads)

	// Should have only 1 operation entry (transaction metadata was nil)
	require.Len(t, entries, 1)
	assert.Equal(t, op1ID, entries[0].EntityID)
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

	failedTxIDs := uc.processMetadataAndEventsBulk(ctx, nil, payloads)

	assert.Empty(t, failedTxIDs)
}

// TestProcessMetadataAndEventsBulk_CollectsDuplicateTxMetadata tests that transaction
// metadata of a transaction already persisted is written alongside the rest of the batch.
func TestProcessMetadataAndEventsBulk_CollectsDuplicateTxMetadata(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	tx1ID := uuid.New().String()
	tx2ID := uuid.New().String() // Already persisted (duplicate on insert)
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

	// 2 transaction metadata entries (tx1 + duplicate tx2)
	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Transaction", gomock.Len(2)).
		Return(&repository.MongoDBBulkInsertResult{
			Attempted: 2,
			Inserted:  1,
			Matched:   1,
		}, nil).
		Times(1)

	// 2 operation metadata entries (op1 + op2)
	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Operation", gomock.Len(2)).
		Return(&repository.MongoDBBulkInsertResult{
			Attempted: 2,
			Inserted:  2,
		}, nil).
		Times(1)

	failedTxIDs := uc.processMetadataAndEventsBulk(ctx, nil, payloads)

	assert.Empty(t, failedTxIDs)
}

// TestProcessMetadataAndEventsBulk_OperationFailureMarksParentTransaction tests that an
// unconfirmed operation metadata write reports the parent transaction, even when the
// transaction's own metadata was written.
func TestProcessMetadataAndEventsBulk_OperationFailureMarksParentTransaction(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	tx1ID := uuid.New().String()
	tx2ID := uuid.New().String()
	op1ID := uuid.New().String()

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
			},
		},
	}

	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Transaction", gomock.Len(2)).
		Return(&repository.MongoDBBulkInsertResult{Attempted: 2, Inserted: 2}, nil).
		Times(1)

	// Single operation entry uses Create; it fails.
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), "Operation", gomock.Any()).
		Return(errors.New("create failed")).
		Times(1)

	failedTxIDs := uc.processMetadataAndEventsBulk(ctx, nil, payloads)

	assert.Equal(t, map[string]struct{}{tx1ID: {}}, failedTxIDs)
}

// TestProcessMetadataAndEventsBulk_TransactionFailureMarksTransaction tests that an
// unconfirmed transaction metadata write reports that transaction only.
func TestProcessMetadataAndEventsBulk_TransactionFailureMarksTransaction(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	tx1ID := uuid.New().String()
	tx2ID := uuid.New().String()

	payloads := []transaction.TransactionProcessingPayload{
		{Transaction: &transaction.Transaction{ID: tx1ID, Metadata: map[string]any{"tx1_key": "tx1_value"}}},
		{Transaction: &transaction.Transaction{ID: tx2ID, Metadata: map[string]any{"tx2_key": "tx2_value"}}},
	}

	// Bulk insert hits a document-level error; the fallback confirms tx1 and fails tx2.
	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), "Transaction", gomock.Len(2)).
		Return(nil, errors.New("bulk insert failed")).
		Times(1)

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), "Transaction", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, meta *mongodb.Metadata) error {
			if meta.EntityID == tx2ID {
				return errors.New("individual create failed")
			}

			return nil
		}).
		Times(2)

	failedTxIDs := uc.processMetadataAndEventsBulk(ctx, nil, payloads)

	assert.Equal(t, map[string]struct{}{tx2ID: {}}, failedTxIDs)
}

// TestProcessMetadataAndEventsBulk_WarnsPerTransactionWithoutMetadataContent tests that
// each affected transaction gets one Warn carrying its ID, and that no metadata key or
// value reaches any log line.
func TestProcessMetadataAndEventsBulk_WarnsPerTransactionWithoutMetadataContent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	logger := &capturingLogger{}
	ctx := libObservability.ContextWithLogger(context.Background(), logger)

	tx1ID := uuid.New().String()
	tx2ID := uuid.New().String()

	payloads := []transaction.TransactionProcessingPayload{
		{
			Transaction: &transaction.Transaction{
				ID:       tx1ID,
				Metadata: map[string]any{"secret_tx_key": "secret_tx_value"},
				Operations: []*operation.Operation{
					{ID: uuid.New().String(), Metadata: map[string]any{"secret_op_key": "secret_op_value"}},
					{ID: uuid.New().String(), Metadata: map[string]any{"secret_op_key2": "secret_op_value2"}},
				},
			},
		},
		{
			Transaction: &transaction.Transaction{
				ID:       tx2ID,
				Metadata: map[string]any{"secret_tx2_key": "secret_tx2_value"},
			},
		},
	}

	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), gomock.Any(), gomock.Len(2)).
		Return(nil, context.DeadlineExceeded).
		Times(2)

	failedTxIDs := uc.processMetadataAndEventsBulk(ctx, logger, payloads)

	assert.Equal(t, map[string]struct{}{tx1ID: {}, tx2ID: {}}, failedTxIDs)

	var warnLines []capturedLogLine

	for _, line := range logger.snapshot() {
		if line.Level == libLog.LevelWarn && line.Msg == "Transaction metadata not confirmed" {
			warnLines = append(warnLines, line)
		}
	}

	require.Len(t, warnLines, 2, "one Warn per affected transaction")

	warns := rendered(warnLines)
	assert.Contains(t, warns, tx1ID)
	assert.Contains(t, warns, tx2ID)

	all := rendered(logger.snapshot())
	for _, secret := range []string{"secret_tx_key", "secret_tx_value", "secret_op_key", "secret_op_value", "secret_tx2_key", "secret_tx2_value"} {
		assert.NotContains(t, all, secret)
	}
}

// TestProcessMetadataAndEventsBulk_EmptyPayloads tests that empty payloads
// are handled gracefully.
func TestProcessMetadataAndEventsBulk_EmptyPayloads(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	ctx := context.Background()

	failedTxIDs := uc.processMetadataAndEventsBulk(ctx, nil, []transaction.TransactionProcessingPayload{})

	assert.Empty(t, failedTxIDs)
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

	failedTxIDs := uc.processMetadataAndEventsBulk(ctx, nil, payloads)

	assert.Empty(t, failedTxIDs)
}
