// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/mock/gomock"
)

// TestGetAllMetadataTransactions is responsible to test GetAllMetadataTransactions with success and error
func TestGetAllMetadataTransactions(t *testing.T) {
	collection := constant.EntityTransaction
	filter := http.QueryHeader{
		Metadata: &bson.M{"metadata": 1},
		Limit:    10,
		Page:     1,
	}

	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(gomock.NewController(t))
	uc := UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
	}

	t.Run("Success", func(t *testing.T) {
		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), collection, filter).
			Return([]*mongodb.Metadata{{ID: bson.NewObjectID()}}, nil).
			Times(1)
		res, err := uc.TransactionMetadataRepo.FindList(context.TODO(), collection, filter)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMSG := "errDatabaseItemNotFound"
		mockMetadataRepo.
			EXPECT().
			FindList(gomock.Any(), collection, filter).
			Return(nil, errors.New(errMSG)).
			Times(1)
		res, err := uc.TransactionMetadataRepo.FindList(context.TODO(), collection, filter)

		assert.EqualError(t, err, errMSG)
		assert.Nil(t, res)
	})
}

// TestGetAllMetadataTransactionsWithOperations tests that operations are populated for transactions
// retrieved by metadata filtering in the GetAllMetadataTransactions method
func TestGetAllMetadataTransactionsWithOperations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	orgIDStr := "00000000-0000-0000-0000-000000000001"
	ledgerIDStr := "00000000-0000-0000-0000-000000000002"
	txID1Str := "00000000-0000-0000-0000-000000000003"
	txID2Str := "00000000-0000-0000-0000-000000000004"

	orgID, _ := uuid.Parse(orgIDStr)
	ledgerID, _ := uuid.Parse(ledgerIDStr)
	txID1, _ := uuid.Parse(txID1Str)
	txID2, _ := uuid.Parse(txID2Str)

	// Explicit non-zero dates so ApplyDefaultDateRange is a no-op and the
	// strict filter expectations below match unchanged.
	filter := http.QueryHeader{
		Metadata:  &bson.M{"key": "value"},
		Limit:     10,
		Page:      1,
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	}

	metadataList := []*mongodb.Metadata{
		{
			ID:       bson.NewObjectID(),
			EntityID: txID1Str,
			Data:     map[string]interface{}{"key": "value"},
		},
		{
			ID:       bson.NewObjectID(),
			EntityID: txID2Str,
			Data:     map[string]interface{}{"key": "value"},
		},
	}

	opMeta := []*mongodb.Metadata{
		{
			EntityID:   "op1-" + txID1Str,
			EntityName: "Operation",
			Data:       map[string]any{"op_key1": "op_value1"},
		},
		{
			EntityID:   "op2-" + txID2Str,
			EntityName: "Operation",
			Data:       map[string]any{"op_key2": "op_value2"},
		},
	}

	ops1 := []*operation.Operation{{
		ID:           "op1-" + txID1Str,
		Type:         constant.DEBIT,
		AccountAlias: "source1",
	}}
	ops2 := []*operation.Operation{{
		ID:           "op2-" + txID2Str,
		Type:         constant.CREDIT,
		AccountAlias: "destination2",
	}}

	transactions := []*transaction.Transaction{
		{
			ID:          txID1Str,
			Operations:  ops1,
			Source:      []string{"source1"},
			Destination: []string{},
		},
		{
			ID:          txID2Str,
			Operations:  ops2,
			Source:      []string{},
			Destination: []string{"destination2"},
		},
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), constant.EntityTransaction, filter).
		Return(metadataList, nil)

	mockTransactionRepo.EXPECT().
		FindOrListAllWithOperations(gomock.Any(), orgID, ledgerID, []uuid.UUID{txID1, txID2}, filter.ToCursorPagination()).
		Return(transactions, libHTTP.CursorPagination{}, nil)

	// Expect operation metadata lookup with both operation IDs
	mockMetadataRepo.EXPECT().
		FindByEntityIDs(
			gomock.Any(),
			constant.EntityOperation,
			[]string{"op1-" + txID1Str, "op2-" + txID2Str},
		).
		Return(opMeta, nil)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
		TransactionRepo:         mockTransactionRepo,
	}

	result, _, err := uc.GetAllMetadataTransactions(context.Background(), orgID, ledgerID, filter)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 2)

	for _, tx := range result {
		assert.NotEmpty(t, tx.Operations, "Transaction operations should be populated")
		assert.NotNil(t, tx.Metadata, "Transaction metadata should be populated")
		assert.Equal(t, "value", tx.Metadata["key"])
		assert.NotNil(t, tx.Operations[0].Metadata, "Operation metadata should be populated")

		switch tx.ID {
		case txID1Str:
			assert.Equal(t, "op1-"+txID1Str, tx.Operations[0].ID)
			assert.Contains(t, tx.Source, "source1")
			assert.Equal(t, "op_value1", tx.Operations[0].Metadata["op_key1"])
		case txID2Str:
			assert.Equal(t, "op2-"+txID2Str, tx.Operations[0].ID)
			assert.Contains(t, tx.Destination, "destination2")
			assert.Equal(t, "op_value2", tx.Operations[0].Metadata["op_key2"])
		}
	}
}

// TestGetAllMetadataTransactionsWithBlockUnblockOperations ensures BLOCK/UNBLOCK
// operations are derived into Source/Destination by their accounting Direction
// (debit -> Source, credit -> Destination), exactly as DEBIT/CREDIT are.
func TestGetAllMetadataTransactionsWithBlockUnblockOperations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	orgIDStr := "00000000-0000-0000-0000-000000000001"
	ledgerIDStr := "00000000-0000-0000-0000-000000000002"
	txIDStr := "00000000-0000-0000-0000-000000000003"

	orgID, _ := uuid.Parse(orgIDStr)
	ledgerID, _ := uuid.Parse(ledgerIDStr)
	txID, _ := uuid.Parse(txIDStr)

	filter := http.QueryHeader{
		Metadata: &bson.M{"key": "value"},
		Limit:    10,
		Page:     1,
	}

	metadataList := []*mongodb.Metadata{
		{
			ID:       bson.NewObjectID(),
			EntityID: txIDStr,
			Data:     map[string]any{"key": "value"},
		},
	}

	ops := []*operation.Operation{
		{
			ID:           "op-block-debit",
			Type:         constant.BLOCK,
			Direction:    constant.DirectionDebit,
			AccountAlias: "block-source",
		},
		{
			ID:           "op-unblock-credit",
			Type:         constant.UNBLOCK,
			Direction:    constant.DirectionCredit,
			AccountAlias: "unblock-destination",
		},
	}

	transactions := []*transaction.Transaction{
		{
			ID:         txIDStr,
			Operations: ops,
		},
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), constant.EntityTransaction, filter).
		Return(metadataList, nil)

	mockTransactionRepo.EXPECT().
		FindOrListAllWithOperations(gomock.Any(), orgID, ledgerID, []uuid.UUID{txID}, filter.ToCursorPagination()).
		Return(transactions, libHTTP.CursorPagination{}, nil)

	mockMetadataRepo.EXPECT().
		FindByEntityIDs(gomock.Any(), constant.EntityOperation, gomock.Any()).
		Return([]*mongodb.Metadata{}, nil)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
		TransactionRepo:         mockTransactionRepo,
	}

	result, _, err := uc.GetAllMetadataTransactions(context.Background(), orgID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Len(t, result, 1)

	// One debit-direction leg -> Source, one credit-direction leg -> Destination.
	// The length asserts make this non-vacuous: an over-derivation that appended
	// a leg to both arrays would push a length to 2 and fail here, where the
	// Contains-only checks below would still pass.
	assert.Len(t, result[0].Source, 1, "only the debit-direction BLOCK leg may be on Source")
	assert.Len(t, result[0].Destination, 1, "only the credit-direction UNBLOCK leg may be on Destination")

	assert.Contains(t, result[0].Source, "block-source", "BLOCK debit leg must be on Source")
	assert.Contains(t, result[0].Destination, "unblock-destination", "UNBLOCK credit leg must be on Destination")

	// Over-derivation guard: debit leg must not leak onto Destination, credit
	// leg must not leak onto Source.
	assert.NotContains(t, result[0].Destination, "block-source", "BLOCK debit leg must not leak onto Destination")
	assert.NotContains(t, result[0].Source, "unblock-destination", "UNBLOCK credit leg must not leak onto Source")
}

// TestGetAllMetadataTransactionsWithDirectionlessBlockUnblock pins the
// defensive silent-skip in the metadata read path: a BLOCK/UNBLOCK op with an
// empty Direction hits the inner Direction switch, which has no default branch,
// so it is dropped from BOTH Source and Destination. Not domain-reachable today
// (every persisted BLOCK/UNBLOCK carries a debit/credit Direction), but pinned
// so a future default branch cannot start routing a directionless op unnoticed.
func TestGetAllMetadataTransactionsWithDirectionlessBlockUnblock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	orgIDStr := "00000000-0000-0000-0000-000000000001"
	ledgerIDStr := "00000000-0000-0000-0000-000000000002"
	txIDStr := "00000000-0000-0000-0000-000000000003"

	orgID, _ := uuid.Parse(orgIDStr)
	ledgerID, _ := uuid.Parse(ledgerIDStr)
	txID, _ := uuid.Parse(txIDStr)

	filter := http.QueryHeader{
		Metadata: &bson.M{"key": "value"},
		Limit:    10,
		Page:     1,
	}

	metadataList := []*mongodb.Metadata{
		{
			ID:       bson.NewObjectID(),
			EntityID: txIDStr,
			Data:     map[string]any{"key": "value"},
		},
	}

	ops := []*operation.Operation{
		{
			ID:           "op-block-directionless",
			Type:         constant.BLOCK,
			Direction:    "",
			AccountAlias: "block-directionless",
		},
		{
			ID:           "op-unblock-directionless",
			Type:         constant.UNBLOCK,
			Direction:    "",
			AccountAlias: "unblock-directionless",
		},
	}

	transactions := []*transaction.Transaction{
		{
			ID:         txIDStr,
			Operations: ops,
		},
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), constant.EntityTransaction, filter).
		Return(metadataList, nil)

	mockTransactionRepo.EXPECT().
		FindOrListAllWithOperations(gomock.Any(), orgID, ledgerID, []uuid.UUID{txID}, filter.ToCursorPagination()).
		Return(transactions, libHTTP.CursorPagination{}, nil)

	mockMetadataRepo.EXPECT().
		FindByEntityIDs(gomock.Any(), constant.EntityOperation, gomock.Any()).
		Return([]*mongodb.Metadata{}, nil)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
		TransactionRepo:         mockTransactionRepo,
	}

	result, _, err := uc.GetAllMetadataTransactions(context.Background(), orgID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Empty(t, result[0].Source, "directionless BLOCK/UNBLOCK legs must not appear on Source")
	assert.Empty(t, result[0].Destination, "directionless BLOCK/UNBLOCK legs must not appear on Destination")
}

// TestGetAllMetadataTransactionsWithMixedOperations proves the BLOCK/UNBLOCK
// derivation coexists with the normal DEBIT/CREDIT derivation in the metadata
// read path: a transaction carrying both families classifies every op on the
// correct side and the Source/Destination lengths add up.
func TestGetAllMetadataTransactionsWithMixedOperations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	orgIDStr := "00000000-0000-0000-0000-000000000001"
	ledgerIDStr := "00000000-0000-0000-0000-000000000002"
	txIDStr := "00000000-0000-0000-0000-000000000003"

	orgID, _ := uuid.Parse(orgIDStr)
	ledgerID, _ := uuid.Parse(ledgerIDStr)
	txID, _ := uuid.Parse(txIDStr)

	filter := http.QueryHeader{
		Metadata: &bson.M{"key": "value"},
		Limit:    10,
		Page:     1,
	}

	metadataList := []*mongodb.Metadata{
		{
			ID:       bson.NewObjectID(),
			EntityID: txIDStr,
			Data:     map[string]any{"key": "value"},
		},
	}

	ops := []*operation.Operation{
		{
			ID:           "op-debit",
			Type:         constant.DEBIT,
			AccountAlias: "normal-source",
		},
		{
			ID:           "op-credit",
			Type:         constant.CREDIT,
			AccountAlias: "normal-destination",
		},
		{
			ID:           "op-block-debit",
			Type:         constant.BLOCK,
			Direction:    constant.DirectionDebit,
			AccountAlias: "block-source",
		},
		{
			ID:           "op-unblock-credit",
			Type:         constant.UNBLOCK,
			Direction:    constant.DirectionCredit,
			AccountAlias: "unblock-destination",
		},
	}

	transactions := []*transaction.Transaction{
		{
			ID:         txIDStr,
			Operations: ops,
		},
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), constant.EntityTransaction, filter).
		Return(metadataList, nil)

	mockTransactionRepo.EXPECT().
		FindOrListAllWithOperations(gomock.Any(), orgID, ledgerID, []uuid.UUID{txID}, filter.ToCursorPagination()).
		Return(transactions, libHTTP.CursorPagination{}, nil)

	mockMetadataRepo.EXPECT().
		FindByEntityIDs(gomock.Any(), constant.EntityOperation, gomock.Any()).
		Return([]*mongodb.Metadata{}, nil)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
		TransactionRepo:         mockTransactionRepo,
	}

	result, _, err := uc.GetAllMetadataTransactions(context.Background(), orgID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Len(t, result, 1)

	assert.Len(t, result[0].Source, 2, "normal DEBIT and BLOCK debit leg must both be on Source")
	assert.Len(t, result[0].Destination, 2, "normal CREDIT and UNBLOCK credit leg must both be on Destination")

	assert.Contains(t, result[0].Source, "normal-source")
	assert.Contains(t, result[0].Source, "block-source")
	assert.Contains(t, result[0].Destination, "normal-destination")
	assert.Contains(t, result[0].Destination, "unblock-destination")

	assert.NotContains(t, result[0].Destination, "normal-source")
	assert.NotContains(t, result[0].Destination, "block-source")
	assert.NotContains(t, result[0].Source, "normal-destination")
	assert.NotContains(t, result[0].Source, "unblock-destination")
}

// TestGetAllMetadataTransactions_NoMetadata ensures that when metadata lookup
// returns an empty (non-nil) slice, the use case returns no transactions and no error,
// and does not call the transaction repository.
func TestGetAllMetadataTransactions_NoMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	collection := constant.EntityTransaction
	filter := http.QueryHeader{
		Metadata:  &bson.M{"k": "v"},
		Limit:     10,
		Page:      1,
		StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	}

	// Return an empty, non-nil slice to hit the early-return branch.
	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), collection, filter).
		Return([]*mongodb.Metadata{}, nil)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
		TransactionRepo:         mockTransactionRepo, // must not be called
	}

	result, cur, err := uc.GetAllMetadataTransactions(context.Background(), uuid.UUID{}, uuid.UUID{}, filter)

	assert.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cur)
}

// TestGetAllMetadataTransactions_AppliesDefaultDateRange ensures that when the
// caller passes no date window, GetAllMetadataTransactions applies the default
// window before reaching the protected transaction repository, so the repo never
// receives a zero-date (unbounded) filter.
func TestGetAllMetadataTransactions_AppliesDefaultDateRange(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	// No StartDate/EndDate: the use case must inject the default window.
	filter := http.QueryHeader{
		Metadata: &bson.M{"key": "value"},
		Limit:    10,
		Page:     1,
	}

	metadataList := []*mongodb.Metadata{
		{
			ID:       bson.NewObjectID(),
			EntityID: txID.String(),
			Data:     map[string]any{"key": "value"},
		},
	}

	// FindList must receive a windowed QueryHeader (non-zero, ordered dates).
	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), constant.EntityTransaction, gomock.Cond(func(qh http.QueryHeader) bool {
			return isWindowed(qh.StartDate, qh.EndDate)
		})).
		Return(metadataList, nil)

	// FindOrListAllWithOperations must receive the same windowed pagination.
	mockTransactionRepo.EXPECT().
		FindOrListAllWithOperations(gomock.Any(), orgID, ledgerID, []uuid.UUID{txID}, gomock.Cond(func(p http.Pagination) bool {
			return isWindowed(p.StartDate, p.EndDate)
		})).
		Return([]*transaction.Transaction{{ID: txID.String()}}, libHTTP.CursorPagination{}, nil)

	mockMetadataRepo.EXPECT().
		FindByEntityIDs(gomock.Any(), constant.EntityOperation, gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
		TransactionRepo:         mockTransactionRepo,
	}

	result, _, err := uc.GetAllMetadataTransactions(context.Background(), orgID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Len(t, result, 1)
}

// isWindowed reports whether a date window is non-zero and correctly ordered.
// It avoids asserting against time.Now() directly.
func isWindowed(start, end time.Time) bool {
	return !start.IsZero() && !end.IsZero() && !start.After(end)
}
