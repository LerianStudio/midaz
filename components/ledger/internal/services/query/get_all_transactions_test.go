// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetAllTransactions(t *testing.T) {
	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	filter := http.QueryHeader{
		Limit:        10,
		Page:         1,
		SortOrder:    "asc",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}
	mockCur := libHTTP.CursorPagination{
		Next: "next",
		Prev: "prev",
	}

	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := UseCase{
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
	}

	t.Run("Success", func(t *testing.T) {
		transactionID := uuid.New()

		operations := []*operation.Operation{
			{
				ID:             uuid.New().String(),
				TransactionID:  transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Type:           constant.DEBIT,
				AccountAlias:   "source",
			},
			{
				ID:             uuid.New().String(),
				TransactionID:  transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Type:           constant.CREDIT,
				AccountAlias:   "destination",
			},
		}

		trans := []*transaction.Transaction{
			{
				ID:             transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Operations:     operations,
				Source:         []string{"source"},
				Destination:    []string{"destination"},
			},
		}

		metadata := []*mongodb.Metadata{
			{
				EntityID:   transactionID.String(),
				EntityName: "Transaction",
				Data:       map[string]any{"key": "value"},
			},
		}

		operationMetadata := []*mongodb.Metadata{
			{
				EntityID:   operations[0].ID,
				EntityName: "Operation",
				Data:       map[string]any{"op_key1": "op_value1"},
			},
			{
				EntityID:   operations[1].ID,
				EntityName: "Operation",
				Data:       map[string]any{"op_key2": "op_value2"},
			},
		}

		operationIDs := []string{operations[0].ID, operations[1].ID}

		mockTransactionRepo.
			EXPECT().
			FindOrListAllWithOperations(gomock.Any(), organizationID, ledgerID, []uuid.UUID{}, filter.ToCursorPagination()).
			Return(trans, mockCur, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindByEntityIDs(gomock.Any(), "Transaction", []string{transactionID.String()}).
			Return(metadata, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindByEntityIDs(gomock.Any(), "Operation", operationIDs).
			Return(operationMetadata, nil).
			Times(1)

		result, cur, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, mockCur, cur)
		assert.Equal(t, map[string]any{"key": "value"}, result[0].Metadata)
		assert.Len(t, result[0].Operations, 2)
		assert.Contains(t, result[0].Source, "source")
		assert.Contains(t, result[0].Destination, "destination")

		assert.Equal(t, map[string]any{"op_key1": "op_value1"}, result[0].Operations[0].Metadata)
		assert.Equal(t, map[string]any{"op_key2": "op_value2"}, result[0].Operations[1].Metadata)
	})

	t.Run("BlockUnblockDerivedSourceDestination", func(t *testing.T) {
		transactionID := uuid.New()

		// BLOCK/UNBLOCK operations carry a normal Direction (debit for source
		// legs, credit for destination legs) but a semantic Type label. They
		// must be classified by Direction exactly as DEBIT/CREDIT are.
		operations := []*operation.Operation{
			{
				ID:             uuid.New().String(),
				TransactionID:  transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Type:           constant.BLOCK,
				Direction:      constant.DirectionDebit,
				AccountAlias:   "block-source",
			},
			{
				ID:             uuid.New().String(),
				TransactionID:  transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Type:           constant.BLOCK,
				Direction:      constant.DirectionCredit,
				AccountAlias:   "block-destination",
			},
			{
				ID:             uuid.New().String(),
				TransactionID:  transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Type:           constant.UNBLOCK,
				Direction:      constant.DirectionDebit,
				AccountAlias:   "unblock-source",
			},
			{
				ID:             uuid.New().String(),
				TransactionID:  transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Type:           constant.UNBLOCK,
				Direction:      constant.DirectionCredit,
				AccountAlias:   "unblock-destination",
			},
		}

		trans := []*transaction.Transaction{
			{
				ID:             transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Operations:     operations,
			},
		}

		mockTransactionRepo.
			EXPECT().
			FindOrListAllWithOperations(gomock.Any(), organizationID, ledgerID, []uuid.UUID{}, filter.ToCursorPagination()).
			Return(trans, mockCur, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindByEntityIDs(gomock.Any(), "Transaction", []string{transactionID.String()}).
			Return([]*mongodb.Metadata{}, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindByEntityIDs(gomock.Any(), "Operation", gomock.Any()).
			Return([]*mongodb.Metadata{}, nil).
			Times(1)

		result, _, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, result, 1)

		// Exactly the two debit legs land on Source and the two credit legs land
		// on Destination — no leg is duplicated across both arrays. The length
		// assertions make this non-vacuous: if a future change appended a leg to
		// both arrays (over-derivation), Len would be 3 and the test would fail
		// even though every Contains assertion below would still pass.
		assert.Len(t, result[0].Source, 2, "only the two debit-direction legs may be on Source")
		assert.Len(t, result[0].Destination, 2, "only the two credit-direction legs may be on Destination")

		assert.Contains(t, result[0].Source, "block-source", "BLOCK debit leg must be on Source")
		assert.Contains(t, result[0].Source, "unblock-source", "UNBLOCK debit leg must be on Source")
		assert.Contains(t, result[0].Destination, "block-destination", "BLOCK credit leg must be on Destination")
		assert.Contains(t, result[0].Destination, "unblock-destination", "UNBLOCK credit leg must be on Destination")

		// Over-derivation guard: a debit-direction leg must NOT leak onto
		// Destination, and a credit-direction leg must NOT leak onto Source.
		assert.NotContains(t, result[0].Destination, "block-source", "BLOCK debit leg must not leak onto Destination")
		assert.NotContains(t, result[0].Destination, "unblock-source", "UNBLOCK debit leg must not leak onto Destination")
		assert.NotContains(t, result[0].Source, "block-destination", "BLOCK credit leg must not leak onto Source")
		assert.NotContains(t, result[0].Source, "unblock-destination", "UNBLOCK credit leg must not leak onto Source")
	})

	t.Run("DirectionlessBlockUnblockDropped", func(t *testing.T) {
		// Defensive pin: the inner Direction switch in the read-path derivation
		// has NO default branch, so a BLOCK/UNBLOCK op with an empty Direction is
		// intentionally dropped from BOTH Source and Destination. This case is not
		// domain-reachable today (every persisted BLOCK/UNBLOCK carries a
		// debit/credit Direction), but we pin the silent-skip so a future change
		// that adds a default branch — accidentally routing a directionless op —
		// cannot land without failing this test.
		transactionID := uuid.New()

		operations := []*operation.Operation{
			{
				ID:             uuid.New().String(),
				TransactionID:  transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Type:           constant.BLOCK,
				Direction:      "", // directionless — must be dropped
				AccountAlias:   "block-directionless",
			},
			{
				ID:             uuid.New().String(),
				TransactionID:  transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Type:           constant.UNBLOCK,
				Direction:      "", // directionless — must be dropped
				AccountAlias:   "unblock-directionless",
			},
		}

		trans := []*transaction.Transaction{
			{
				ID:             transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Operations:     operations,
			},
		}

		mockTransactionRepo.
			EXPECT().
			FindOrListAllWithOperations(gomock.Any(), organizationID, ledgerID, []uuid.UUID{}, filter.ToCursorPagination()).
			Return(trans, mockCur, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindByEntityIDs(gomock.Any(), "Transaction", []string{transactionID.String()}).
			Return([]*mongodb.Metadata{}, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindByEntityIDs(gomock.Any(), "Operation", gomock.Any()).
			Return([]*mongodb.Metadata{}, nil).
			Times(1)

		result, _, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Empty(t, result[0].Source, "directionless BLOCK/UNBLOCK legs must not appear on Source")
		assert.Empty(t, result[0].Destination, "directionless BLOCK/UNBLOCK legs must not appear on Destination")
		assert.NotContains(t, result[0].Source, "block-directionless")
		assert.NotContains(t, result[0].Source, "unblock-directionless")
		assert.NotContains(t, result[0].Destination, "block-directionless")
		assert.NotContains(t, result[0].Destination, "unblock-directionless")
	})

	t.Run("MixedDebitCreditAndBlockUnblock", func(t *testing.T) {
		// A single transaction carrying BOTH a normal DEBIT/CREDIT pair AND a
		// BLOCK/UNBLOCK pair must classify every op on the correct side, and the
		// Source/Destination lengths must add up across the two op families —
		// proving the BLOCK/UNBLOCK cases coexist with the existing DEBIT/CREDIT
		// cases rather than replacing or shadowing them.
		transactionID := uuid.New()

		operations := []*operation.Operation{
			{
				ID:             uuid.New().String(),
				TransactionID:  transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Type:           constant.DEBIT,
				AccountAlias:   "normal-source",
			},
			{
				ID:             uuid.New().String(),
				TransactionID:  transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Type:           constant.CREDIT,
				AccountAlias:   "normal-destination",
			},
			{
				ID:             uuid.New().String(),
				TransactionID:  transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Type:           constant.BLOCK,
				Direction:      constant.DirectionDebit,
				AccountAlias:   "block-source",
			},
			{
				ID:             uuid.New().String(),
				TransactionID:  transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Type:           constant.UNBLOCK,
				Direction:      constant.DirectionCredit,
				AccountAlias:   "unblock-destination",
			},
		}

		trans := []*transaction.Transaction{
			{
				ID:             transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Operations:     operations,
			},
		}

		mockTransactionRepo.
			EXPECT().
			FindOrListAllWithOperations(gomock.Any(), organizationID, ledgerID, []uuid.UUID{}, filter.ToCursorPagination()).
			Return(trans, mockCur, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindByEntityIDs(gomock.Any(), "Transaction", []string{transactionID.String()}).
			Return([]*mongodb.Metadata{}, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindByEntityIDs(gomock.Any(), "Operation", gomock.Any()).
			Return([]*mongodb.Metadata{}, nil).
			Times(1)

		result, _, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		assert.NoError(t, err)
		assert.Len(t, result, 1)

		// One DEBIT + one debit-direction BLOCK -> Source; one CREDIT + one
		// credit-direction UNBLOCK -> Destination. Lengths add up across families.
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
	})

	t.Run("Error_FindAll", func(t *testing.T) {
		mockTransactionRepo.
			EXPECT().
			FindOrListAllWithOperations(gomock.Any(), organizationID, ledgerID, []uuid.UUID{}, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, errors.New("database error")).
			Times(1)

		result, cur, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
		assert.Contains(t, err.Error(), "database error")
	})

	t.Run("Error_ItemNotFound", func(t *testing.T) {
		mockTransactionRepo.
			EXPECT().
			FindOrListAllWithOperations(gomock.Any(), organizationID, ledgerID, []uuid.UUID{}, filter.ToCursorPagination()).
			Return(nil, libHTTP.CursorPagination{}, services.ErrDatabaseItemNotFound).
			Times(1)

		result, cur, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
		assert.Contains(t, err.Error(), "No transactions were found")
	})

	t.Run("Error_Metadata", func(t *testing.T) {
		trans := []*transaction.Transaction{
			{
				ID:             uuid.New().String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Operations:     []*operation.Operation{},
			},
		}

		mockTransactionRepo.
			EXPECT().
			FindOrListAllWithOperations(gomock.Any(), organizationID, ledgerID, []uuid.UUID{}, filter.ToCursorPagination()).
			Return(trans, mockCur, nil).
			Times(1)

		mockMetadataRepo.
			EXPECT().
			FindByEntityIDs(gomock.Any(), "Transaction", []string{trans[0].ID}).
			Return(nil, errors.New("metadata error")).
			Times(1)

		result, cur, err := uc.GetAllTransactions(context.TODO(), organizationID, ledgerID, filter)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, libHTTP.CursorPagination{}, cur)
		assert.Contains(t, err.Error(), "No transactions were found")
	})
}

func TestGetOperationsByTransaction(t *testing.T) {
	t.Parallel()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	transactionID := uuid.New()
	filter := http.QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "asc",
	}

	tests := []struct {
		name              string
		setupMocks        func(mockOpRepo *operation.MockRepository, mockMetaRepo *mongodb.MockRepository)
		expectedErr       error
		expectedSourceLen int
		expectedDestLen   int
		expectedOpLen     int
		// extraAsserts runs alias-level cross-checks (inclusion + over-derivation
		// guards) on the derived Source/Destination arrays. Optional.
		extraAsserts func(t *testing.T, result *transaction.Transaction)
	}{
		{
			name: "success with debit and credit operations",
			setupMocks: func(mockOpRepo *operation.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				ops := []*operation.Operation{
					{
						ID:             uuid.New().String(),
						TransactionID:  transactionID.String(),
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Type:           constant.DEBIT,
						AccountAlias:   "source1",
					},
					{
						ID:             uuid.New().String(),
						TransactionID:  transactionID.String(),
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Type:           constant.CREDIT,
						AccountAlias:   "dest1",
					},
					{
						ID:             uuid.New().String(),
						TransactionID:  transactionID.String(),
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Type:           constant.DEBIT,
						AccountAlias:   "source2",
					},
				}

				mockOpRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, transactionID, filter.ToCursorPagination()).
					Return(ops, libHTTP.CursorPagination{}, nil).
					Times(1)

				mockMetaRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Operation", gomock.Any()).
					Return(nil, nil).
					Times(1)
			},
			expectedErr:       nil,
			expectedSourceLen: 2,
			expectedDestLen:   1,
			expectedOpLen:     3,
		},
		{
			name: "success with block and unblock operations classified by direction",
			setupMocks: func(mockOpRepo *operation.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				ops := []*operation.Operation{
					{
						ID:             uuid.New().String(),
						TransactionID:  transactionID.String(),
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Type:           constant.BLOCK,
						Direction:      constant.DirectionDebit,
						AccountAlias:   "block-source",
					},
					{
						ID:             uuid.New().String(),
						TransactionID:  transactionID.String(),
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Type:           constant.UNBLOCK,
						Direction:      constant.DirectionCredit,
						AccountAlias:   "unblock-dest",
					},
				}

				mockOpRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, transactionID, filter.ToCursorPagination()).
					Return(ops, libHTTP.CursorPagination{}, nil).
					Times(1)

				mockMetaRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Operation", gomock.Any()).
					Return(nil, nil).
					Times(1)
			},
			expectedErr:       nil,
			expectedSourceLen: 1,
			expectedDestLen:   1,
			expectedOpLen:     2,
			extraAsserts: func(t *testing.T, result *transaction.Transaction) {
				// debit-direction BLOCK -> Source; credit-direction UNBLOCK ->
				// Destination. Over-derivation guards prove neither leg leaks onto
				// the opposite array (the length asserts above would pass even if a
				// leg were wrongly appended to both).
				assert.Contains(t, result.Source, "block-source")
				assert.Contains(t, result.Destination, "unblock-dest")
				assert.NotContains(t, result.Destination, "block-source", "BLOCK debit leg must not leak onto Destination")
				assert.NotContains(t, result.Source, "unblock-dest", "UNBLOCK credit leg must not leak onto Source")
			},
		},
		{
			name: "directionless block and unblock operations are dropped",
			setupMocks: func(mockOpRepo *operation.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				// Defensive pin: directionless BLOCK/UNBLOCK ops hit the inner
				// Direction switch which has no default branch, so they are silently
				// dropped from both Source and Destination. Pinned so a future
				// default branch cannot start routing them unnoticed.
				ops := []*operation.Operation{
					{
						ID:             uuid.New().String(),
						TransactionID:  transactionID.String(),
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Type:           constant.BLOCK,
						Direction:      "",
						AccountAlias:   "block-directionless",
					},
					{
						ID:             uuid.New().String(),
						TransactionID:  transactionID.String(),
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Type:           constant.UNBLOCK,
						Direction:      "",
						AccountAlias:   "unblock-directionless",
					},
				}

				mockOpRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, transactionID, filter.ToCursorPagination()).
					Return(ops, libHTTP.CursorPagination{}, nil).
					Times(1)

				mockMetaRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Operation", gomock.Any()).
					Return(nil, nil).
					Times(1)
			},
			expectedErr:       nil,
			expectedSourceLen: 0,
			expectedDestLen:   0,
			expectedOpLen:     2,
			extraAsserts: func(t *testing.T, result *transaction.Transaction) {
				assert.NotContains(t, result.Source, "block-directionless")
				assert.NotContains(t, result.Source, "unblock-directionless")
				assert.NotContains(t, result.Destination, "block-directionless")
				assert.NotContains(t, result.Destination, "unblock-directionless")
			},
		},
		{
			name: "mixed debit/credit and block/unblock operations coexist",
			setupMocks: func(mockOpRepo *operation.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				ops := []*operation.Operation{
					{
						ID:             uuid.New().String(),
						TransactionID:  transactionID.String(),
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Type:           constant.DEBIT,
						AccountAlias:   "normal-source",
					},
					{
						ID:             uuid.New().String(),
						TransactionID:  transactionID.String(),
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Type:           constant.CREDIT,
						AccountAlias:   "normal-dest",
					},
					{
						ID:             uuid.New().String(),
						TransactionID:  transactionID.String(),
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Type:           constant.BLOCK,
						Direction:      constant.DirectionDebit,
						AccountAlias:   "block-source",
					},
					{
						ID:             uuid.New().String(),
						TransactionID:  transactionID.String(),
						OrganizationID: organizationID.String(),
						LedgerID:       ledgerID.String(),
						Type:           constant.UNBLOCK,
						Direction:      constant.DirectionCredit,
						AccountAlias:   "unblock-dest",
					},
				}

				mockOpRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, transactionID, filter.ToCursorPagination()).
					Return(ops, libHTTP.CursorPagination{}, nil).
					Times(1)

				mockMetaRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Operation", gomock.Any()).
					Return(nil, nil).
					Times(1)
			},
			expectedErr:       nil,
			expectedSourceLen: 2,
			expectedDestLen:   2,
			expectedOpLen:     4,
			extraAsserts: func(t *testing.T, result *transaction.Transaction) {
				assert.Contains(t, result.Source, "normal-source")
				assert.Contains(t, result.Source, "block-source")
				assert.Contains(t, result.Destination, "normal-dest")
				assert.Contains(t, result.Destination, "unblock-dest")
				assert.NotContains(t, result.Destination, "normal-source")
				assert.NotContains(t, result.Destination, "block-source")
				assert.NotContains(t, result.Source, "normal-dest")
				assert.NotContains(t, result.Source, "unblock-dest")
			},
		},
		{
			name: "success with no operations",
			setupMocks: func(mockOpRepo *operation.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				// When there are no operations, GetAllOperations returns early without calling metadata
				mockOpRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, transactionID, filter.ToCursorPagination()).
					Return([]*operation.Operation{}, libHTTP.CursorPagination{}, nil).
					Times(1)
			},
			expectedErr:       nil,
			expectedSourceLen: 0,
			expectedDestLen:   0,
			expectedOpLen:     0,
		},
		{
			name: "error from GetAllOperations",
			setupMocks: func(mockOpRepo *operation.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockOpRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, transactionID, filter.ToCursorPagination()).
					Return(nil, libHTTP.CursorPagination{}, errors.New("database error")).
					Times(1)
			},
			expectedErr: errors.New("database error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockOpRepo := operation.NewMockRepository(ctrl)
			mockMetaRepo := mongodb.NewMockRepository(ctrl)

			tt.setupMocks(mockOpRepo, mockMetaRepo)

			uc := UseCase{
				OperationRepo:           mockOpRepo,
				TransactionMetadataRepo: mockMetaRepo,
			}

			tran := &transaction.Transaction{
				ID:             transactionID.String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
			}

			result, err := uc.GetOperationsByTransaction(context.Background(), organizationID, ledgerID, tran, filter)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr.Error())
				assert.Nil(t, result)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Len(t, result.Source, tt.expectedSourceLen)
			assert.Len(t, result.Destination, tt.expectedDestLen)
			assert.Len(t, result.Operations, tt.expectedOpLen)

			if tt.extraAsserts != nil {
				tt.extraAsserts(t, result)
			}
		})
	}
}
