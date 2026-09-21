// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	txMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	transactionPostgres "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func TestGetAtomicTransactionBatchProjections_ReadsBoundedPrimarySnapshotOnce(t *testing.T) {
	controller := gomock.NewController(t)
	transactionRepository := transactionPostgres.NewMockRepository(controller)
	metadataRepository := txMongo.NewMockRepository(controller)
	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000111")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000112")
	transactionIDs := []uuid.UUID{
		uuid.MustParse("01994f13-29b7-7000-8000-000000000113"),
		uuid.MustParse("01994f13-29b7-7000-8000-000000000114"),
	}
	firstOperationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000115").String()
	secondOperationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000116").String()
	thirdOperationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000117").String()
	transactions := []*transactionPostgres.Transaction{
		{
			ID:             transactionIDs[1].String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations: []*operation.Operation{{
				ID:           thirdOperationID,
				Type:         constant.CREDIT,
				AccountAlias: "@destination-1",
			}},
		},
		{
			ID:             transactionIDs[0].String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations: []*operation.Operation{
				{ID: secondOperationID, Type: constant.CREDIT, AccountAlias: "@destination-0"},
				{ID: firstOperationID, Type: constant.DEBIT, AccountAlias: "@source-0"},
			},
		},
	}

	transactionRepository.EXPECT().
		FindOrListAllWithOperations(
			gomock.Cond(func(ctx context.Context) bool { return readrouting.IsPrimaryRead(ctx) }),
			organizationID,
			ledgerID,
			transactionIDs,
			pkgHTTP.Pagination{Limit: len(transactionIDs), SortOrder: "ASC"},
		).
		Return(transactions, libHTTP.CursorPagination{}, nil).
		Times(1)
	metadataRepository.EXPECT().
		FindByEntityIDs(
			gomock.Any(),
			constant.EntityTransaction,
			[]string{transactionIDs[1].String(), transactionIDs[0].String()},
		).
		Return([]*txMongo.Metadata{{
			EntityID: transactionIDs[0].String(),
			Data:     txMongo.JSON{"transaction": "metadata"},
		}}, nil).
		Times(1)
	metadataRepository.EXPECT().
		FindByEntityIDs(
			gomock.Any(),
			constant.EntityOperation,
			[]string{thirdOperationID, firstOperationID, secondOperationID},
		).
		Return([]*txMongo.Metadata{{
			EntityID: firstOperationID,
			Data:     txMongo.JSON{"operation": "metadata"},
		}}, nil).
		Times(1)

	useCase := &UseCase{
		TransactionRepo:         transactionRepository,
		TransactionMetadataRepo: metadataRepository,
	}
	result, err := useCase.GetAtomicTransactionBatchProjections(
		context.Background(),
		organizationID,
		ledgerID,
		transactionIDs,
	)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, transactionIDs[1].String(), result[0].ID, "repository order is not treated as request order")
	assert.Equal(t, []string{firstOperationID, secondOperationID}, []string{
		result[1].Operations[0].ID,
		result[1].Operations[1].ID,
	})
	assert.Equal(t, []string{"@source-0"}, result[1].Source)
	assert.Equal(t, []string{"@destination-0"}, result[1].Destination)
	assert.Equal(t, map[string]any{"transaction": "metadata"}, result[1].Metadata)
	assert.Equal(t, map[string]any{"operation": "metadata"}, result[1].Operations[0].Metadata)
}

func TestGetAtomicTransactionBatchProjections_RejectsMoreThanFiftyIDsBeforeReads(t *testing.T) {
	controller := gomock.NewController(t)
	useCase := &UseCase{
		TransactionRepo:         transactionPostgres.NewMockRepository(controller),
		TransactionMetadataRepo: txMongo.NewMockRepository(controller),
	}
	transactionIDs := make([]uuid.UUID, atomicTransactionBatchProjectionLimit+1)
	for index := range transactionIDs {
		transactionIDs[index] = uuid.NewSHA1(uuid.Nil, []byte{byte(index + 1)})
	}

	result, err := useCase.GetAtomicTransactionBatchProjections(
		context.Background(),
		uuid.New(),
		uuid.New(),
		transactionIDs,
	)
	assert.Nil(t, result)
	require.ErrorContains(t, err, "identity is invalid")
}
