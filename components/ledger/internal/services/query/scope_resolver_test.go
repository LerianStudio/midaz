// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"
	"testing"

	libPointers "github.com/LerianStudio/lib-commons/v7/commons/pointers"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

var _ ScopeResolver = (*UseCase)(nil)

func scopeAccount(alias string) (*mmodel.Account, uuid.UUID) {
	id := uuid.New()

	return &mmodel.Account{ID: id.String(), Alias: libPointers.String(alias)}, id
}

func notFoundTransaction() error {
	return pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityTransaction)
}

func legsBody(from, to []string) mtransaction.Transaction {
	body := mtransaction.Transaction{}

	for _, alias := range from {
		body.Send.Source.From = append(body.Send.Source.From, mtransaction.FromTo{AccountAlias: alias})
	}

	for _, alias := range to {
		body.Send.Distribute.To = append(body.Send.Distribute.To, mtransaction.FromTo{AccountAlias: alias})
	}

	return body
}

func TestAccountIDsByAlias(t *testing.T) {
	organizationID, ledgerID := uuid.New(), uuid.New()

	t.Run("every alias found resolves in one batched read", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := account.NewMockRepository(ctrl)
		uc := &UseCase{AccountRepo: repo}

		alice, aliceID := scopeAccount("@alice")
		bob, bobID := scopeAccount("@bob")

		repo.EXPECT().
			ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{"@alice", "@bob"}).
			Return([]*mmodel.Account{bob, alice}, nil).
			Times(1)

		got, err := uc.AccountIDsByAlias(context.Background(), organizationID, ledgerID, []string{"@alice", "@bob"})
		require.NoError(t, err)
		assert.Equal(t, map[string]uuid.UUID{"@alice": aliceID, "@bob": bobID}, got.AccountIDs)
		assert.Empty(t, got.NotFound)
	})

	t.Run("an alias with no live account is reported as not found, never dropped", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := account.NewMockRepository(ctrl)
		uc := &UseCase{AccountRepo: repo}

		alice, aliceID := scopeAccount("@alice")

		repo.EXPECT().
			ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{"@ghost", "@alice", "@deleted"}).
			Return([]*mmodel.Account{alice}, nil)

		got, err := uc.AccountIDsByAlias(context.Background(), organizationID, ledgerID, []string{"@ghost", "@alice", "@deleted"})
		require.NoError(t, err)
		assert.Equal(t, map[string]uuid.UUID{"@alice": aliceID}, got.AccountIDs)
		assert.Equal(t, []string{"@ghost", "@deleted"}, got.NotFound)
	})

	t.Run("the external account alias resolves like any other alias", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := account.NewMockRepository(ctrl)
		uc := &UseCase{AccountRepo: repo}

		external, externalID := scopeAccount(constant.DefaultExternalAccountAliasPrefix + "BRL")

		repo.EXPECT().
			ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{"@external/BRL"}).
			Return([]*mmodel.Account{external}, nil)

		got, err := uc.AccountIDsByAlias(context.Background(), organizationID, ledgerID, []string{"@external/BRL"})
		require.NoError(t, err)
		assert.Equal(t, map[string]uuid.UUID{"@external/BRL": externalID}, got.AccountIDs)
	})

	t.Run("duplicates are asked once and answered once", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := account.NewMockRepository(ctrl)
		uc := &UseCase{AccountRepo: repo}

		alice, aliceID := scopeAccount("@alice")

		repo.EXPECT().
			ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{"@alice", "@ghost"}).
			Return([]*mmodel.Account{alice}, nil)

		got, err := uc.AccountIDsByAlias(context.Background(), organizationID, ledgerID, []string{"@alice", "@ghost", "@alice", "@ghost"})
		require.NoError(t, err)
		assert.Equal(t, map[string]uuid.UUID{"@alice": aliceID}, got.AccountIDs)
		assert.Equal(t, []string{"@ghost"}, got.NotFound)
	})

	t.Run("an empty alias is reported as not found without being asked", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		uc := &UseCase{AccountRepo: account.NewMockRepository(ctrl)}

		got, err := uc.AccountIDsByAlias(context.Background(), organizationID, ledgerID, []string{""})
		require.NoError(t, err)
		assert.Empty(t, got.AccountIDs)
		assert.Equal(t, []string{""}, got.NotFound)
	})

	t.Run("an empty question reads nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		uc := &UseCase{AccountRepo: account.NewMockRepository(ctrl)}

		got, err := uc.AccountIDsByAlias(context.Background(), organizationID, ledgerID, nil)
		require.NoError(t, err)
		assert.Empty(t, got.AccountIDs)
		assert.Empty(t, got.NotFound)
	})

	t.Run("one hundred distinct aliases is the limit", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := account.NewMockRepository(ctrl)
		uc := &UseCase{AccountRepo: repo}

		aliases := make([]string, 0, MaxScopeAliases+1)
		for i := range MaxScopeAliases {
			aliases = append(aliases, fmt.Sprintf("@a%d", i))
		}

		repo.EXPECT().ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, aliases).Return(nil, nil)

		got, err := uc.AccountIDsByAlias(context.Background(), organizationID, ledgerID, append(aliases, aliases[0]))
		require.NoError(t, err)
		assert.Len(t, got.NotFound, MaxScopeAliases)
	})

	t.Run("more than one hundred distinct aliases is refused before any read", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		uc := &UseCase{AccountRepo: account.NewMockRepository(ctrl)}

		aliases := make([]string, 0, MaxScopeAliases+1)
		for i := range MaxScopeAliases + 1 {
			aliases = append(aliases, fmt.Sprintf("@a%d", i))
		}

		_, err := uc.AccountIDsByAlias(context.Background(), organizationID, ledgerID, aliases)

		var tooMany ScopeAliasBatchTooLargeError
		require.ErrorAs(t, err, &tooMany)
		assert.Equal(t, MaxScopeAliases+1, tooMany.Count)
		assert.Equal(t, MaxScopeAliases, tooMany.Max)
	})

	t.Run("two live accounts under one alias are refused, not guessed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := account.NewMockRepository(ctrl)
		uc := &UseCase{AccountRepo: repo}

		first, firstID := scopeAccount("@twin")
		second, secondID := scopeAccount("@twin")

		repo.EXPECT().ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{"@twin"}).
			Return([]*mmodel.Account{first, second}, nil)

		_, err := uc.AccountIDsByAlias(context.Background(), organizationID, ledgerID, []string{"@twin"})

		var ambiguous ScopeAliasAmbiguousError
		require.ErrorAs(t, err, &ambiguous)
		assert.Equal(t, "@twin", ambiguous.Alias)
		assert.ElementsMatch(t, []uuid.UUID{firstID, secondID}, ambiguous.AccountIDs)
	})

	t.Run("a repository failure is an error, not a not-found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := account.NewMockRepository(ctrl)
		uc := &UseCase{AccountRepo: repo}

		boom := errors.New("replica down")
		repo.EXPECT().ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{"@alice"}).Return(nil, boom)

		got, err := uc.AccountIDsByAlias(context.Background(), organizationID, ledgerID, []string{"@alice"})
		require.ErrorIs(t, err, boom)
		assert.Nil(t, got)
	})
}

func TestAccountIDsOfTransaction(t *testing.T) {
	organizationID, ledgerID, transactionID := uuid.New(), uuid.New(), uuid.New()

	t.Run("two legs answer both accounts", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		txRepo := transaction.NewMockRepository(ctrl)
		accRepo := account.NewMockRepository(ctrl)
		uc := &UseCase{TransactionRepo: txRepo, AccountRepo: accRepo}

		source, sourceID := scopeAccount("@source")
		destination, destinationID := scopeAccount("@destination")

		txRepo.EXPECT().ListAccountRefsByTransaction(gomock.Any(), organizationID, ledgerID, transactionID).
			Return(&transaction.AccountRefs{
				AccountIDs: []uuid.UUID{sourceID, destinationID},
				Body:       legsBody([]string{"@source"}, []string{"@destination"}),
			}, nil)
		accRepo.EXPECT().ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{"@source", "@destination"}).
			Return([]*mmodel.Account{source, destination}, nil)

		ids, found, err := uc.AccountIDsOfTransaction(context.Background(), organizationID, ledgerID, transactionID)
		require.NoError(t, err)
		assert.True(t, found)
		assert.ElementsMatch(t, []uuid.UUID{sourceID, destinationID}, ids)
	})

	t.Run("three legs answer three distinct accounts", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		txRepo := transaction.NewMockRepository(ctrl)
		accRepo := account.NewMockRepository(ctrl)
		uc := &UseCase{TransactionRepo: txRepo, AccountRepo: accRepo}

		a, aID := scopeAccount("@a")
		b, bID := scopeAccount("@b")
		c, cID := scopeAccount("@c")

		txRepo.EXPECT().ListAccountRefsByTransaction(gomock.Any(), organizationID, ledgerID, transactionID).
			Return(&transaction.AccountRefs{
				AccountIDs: []uuid.UUID{aID, bID, cID},
				Body:       legsBody([]string{"@a"}, []string{"@b#default", "0#@c#savings"}),
			}, nil)
		accRepo.EXPECT().ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{"@a", "@b", "@c"}).
			Return([]*mmodel.Account{a, b, c}, nil)

		ids, found, err := uc.AccountIDsOfTransaction(context.Background(), organizationID, ledgerID, transactionID)
		require.NoError(t, err)
		assert.True(t, found)
		assert.ElementsMatch(t, []uuid.UUID{aID, bID, cID}, ids)
	})

	t.Run("a pending transaction names its destination from the body, which the hold rows omit", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		txRepo := transaction.NewMockRepository(ctrl)
		accRepo := account.NewMockRepository(ctrl)
		uc := &UseCase{TransactionRepo: txRepo, AccountRepo: accRepo}

		source, sourceID := scopeAccount("@source")
		destination, destinationID := scopeAccount("@destination")

		txRepo.EXPECT().ListAccountRefsByTransaction(gomock.Any(), organizationID, ledgerID, transactionID).
			Return(&transaction.AccountRefs{
				AccountIDs: []uuid.UUID{sourceID},
				Body:       legsBody([]string{"@source"}, []string{"@destination"}),
			}, nil)
		accRepo.EXPECT().ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{"@source", "@destination"}).
			Return([]*mmodel.Account{source, destination}, nil)

		ids, found, err := uc.AccountIDsOfTransaction(context.Background(), organizationID, ledgerID, transactionID)
		require.NoError(t, err)
		assert.True(t, found)
		assert.ElementsMatch(t, []uuid.UUID{sourceID, destinationID}, ids)
	})

	t.Run("a transaction without a body answers from its operation rows alone", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		txRepo := transaction.NewMockRepository(ctrl)
		uc := &UseCase{TransactionRepo: txRepo, AccountRepo: account.NewMockRepository(ctrl)}

		aID, bID := uuid.New(), uuid.New()

		txRepo.EXPECT().ListAccountRefsByTransaction(gomock.Any(), organizationID, ledgerID, transactionID).
			Return(&transaction.AccountRefs{AccountIDs: []uuid.UUID{aID, bID}}, nil)

		ids, found, err := uc.AccountIDsOfTransaction(context.Background(), organizationID, ledgerID, transactionID)
		require.NoError(t, err)
		assert.True(t, found)
		assert.ElementsMatch(t, []uuid.UUID{aID, bID}, ids)
	})

	t.Run("a persisted transaction with neither rows nor legs fails closed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		txRepo := transaction.NewMockRepository(ctrl)
		uc := &UseCase{TransactionRepo: txRepo, AccountRepo: account.NewMockRepository(ctrl)}

		txRepo.EXPECT().ListAccountRefsByTransaction(gomock.Any(), organizationID, ledgerID, transactionID).
			Return(&transaction.AccountRefs{}, nil)

		ids, found, err := uc.AccountIDsOfTransaction(context.Background(), organizationID, ledgerID, transactionID)
		require.ErrorIs(t, err, ErrScopeTransactionAccountsUnavailable)
		assert.False(t, found)
		assert.Nil(t, ids)
	})

	t.Run("an unknown transaction is not found, not an error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		txRepo := transaction.NewMockRepository(ctrl)
		redisRepo := redis.NewMockRedisRepository(ctrl)
		uc := &UseCase{TransactionRepo: txRepo, TransactionRedisRepo: redisRepo}

		txRepo.EXPECT().ListAccountRefsByTransaction(gomock.Any(), organizationID, ledgerID, transactionID).
			Return(nil, notFoundTransaction())
		redisRepo.EXPECT().GetBytes(gomock.Any(), utils.WriteBehindTransactionKey(organizationID, ledgerID, transactionID.String())).
			Return(nil, goredis.Nil)

		ids, found, err := uc.AccountIDsOfTransaction(context.Background(), organizationID, ledgerID, transactionID)
		require.NoError(t, err)
		assert.False(t, found)
		assert.Nil(t, ids)
	})

	t.Run("a transaction still only in the write-behind cache resolves from there", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		txRepo := transaction.NewMockRepository(ctrl)
		accRepo := account.NewMockRepository(ctrl)
		redisRepo := redis.NewMockRedisRepository(ctrl)
		uc := &UseCase{TransactionRepo: txRepo, AccountRepo: accRepo, TransactionRedisRepo: redisRepo}

		source, sourceID := scopeAccount("@source")
		destination, destinationID := scopeAccount("@destination")

		cached := transaction.Transaction{
			ID:         transactionID.String(),
			Body:       legsBody([]string{"@source"}, []string{"@destination"}),
			Operations: []*operation.Operation{{AccountID: sourceID.String()}},
		}
		payload, err := msgpack.Marshal(cached)
		require.NoError(t, err)

		txRepo.EXPECT().ListAccountRefsByTransaction(gomock.Any(), organizationID, ledgerID, transactionID).
			Return(nil, notFoundTransaction())
		redisRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Return(payload, nil)
		accRepo.EXPECT().ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, []string{"@source", "@destination"}).
			Return([]*mmodel.Account{source, destination}, nil)

		ids, found, err := uc.AccountIDsOfTransaction(context.Background(), organizationID, ledgerID, transactionID)
		require.NoError(t, err)
		assert.True(t, found)
		assert.ElementsMatch(t, []uuid.UUID{sourceID, destinationID}, ids)
	})

	t.Run("a cache transport failure is an error, never a not-found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		txRepo := transaction.NewMockRepository(ctrl)
		redisRepo := redis.NewMockRedisRepository(ctrl)
		uc := &UseCase{TransactionRepo: txRepo, TransactionRedisRepo: redisRepo}

		boom := errors.New("redis unreachable")
		txRepo.EXPECT().ListAccountRefsByTransaction(gomock.Any(), organizationID, ledgerID, transactionID).
			Return(nil, notFoundTransaction())
		redisRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Return(nil, boom)

		_, found, err := uc.AccountIDsOfTransaction(context.Background(), organizationID, ledgerID, transactionID)
		require.ErrorIs(t, err, boom)
		assert.False(t, found)
	})

	t.Run("a repository failure is an error, not a not-found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		txRepo := transaction.NewMockRepository(ctrl)
		uc := &UseCase{TransactionRepo: txRepo}

		boom := errors.New("primary down")
		txRepo.EXPECT().ListAccountRefsByTransaction(gomock.Any(), organizationID, ledgerID, transactionID).Return(nil, boom)

		_, found, err := uc.AccountIDsOfTransaction(context.Background(), organizationID, ledgerID, transactionID)
		require.ErrorIs(t, err, boom)
		assert.False(t, found)
	})

	t.Run("an execution the engine indexed but did not project resolves from its evidence", func(t *testing.T) {
		index, envelope, receipt, expected := queryEngineWriteBehindFixture(t)
		fake := &engineWriteBehindRepositoryFake{
			index: index, envelope: envelope, receipt: receipt, materializeResult: true,
			materializedErr: redis.ErrEngineWriteBehindNotFound,
		}

		ctrl := gomock.NewController(t)
		txRepo := transaction.NewMockRepository(ctrl)
		uc := &UseCase{
			TransactionRepo: txRepo, AccountRepo: account.NewMockRepository(ctrl),
			EngineWriteBehindRepo: fake, EngineWriteBehindCodec: command.EngineWriteBehindEvidenceCodec{},
		}

		org, ledger, id := uuid.MustParse(expected.OrganizationID), uuid.MustParse(expected.LedgerID), uuid.MustParse(expected.ID)
		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-query")

		txRepo.EXPECT().ListAccountRefsByTransaction(gomock.Any(), org, ledger, id).Return(nil, notFoundTransaction())

		ids, found, err := uc.AccountIDsOfTransaction(ctx, org, ledger, id)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, []uuid.UUID{uuid.MustParse("66666666-6666-4666-8666-666666666666")}, ids)
	})
}

func TestAccountIDOfBalance(t *testing.T) {
	organizationID, ledgerID, balanceID := uuid.New(), uuid.New(), uuid.New()

	t.Run("a live balance answers its account", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := balance.NewMockRepository(ctrl)
		uc := &UseCase{BalanceRepo: repo}

		accountID := uuid.New()
		repo.EXPECT().Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(&mmodel.Balance{ID: balanceID.String(), AccountID: accountID.String()}, nil)

		got, found, err := uc.AccountIDOfBalance(context.Background(), organizationID, ledgerID, balanceID)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, accountID, got)
	})

	t.Run("an unknown or deleted balance is not found, not an error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := balance.NewMockRepository(ctrl)
		uc := &UseCase{BalanceRepo: repo}

		repo.EXPECT().Find(gomock.Any(), organizationID, ledgerID, balanceID).
			Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance))

		got, found, err := uc.AccountIDOfBalance(context.Background(), organizationID, ledgerID, balanceID)
		require.NoError(t, err)
		assert.False(t, found)
		assert.Equal(t, uuid.Nil, got)
	})

	t.Run("a repository failure is an error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := balance.NewMockRepository(ctrl)
		uc := &UseCase{BalanceRepo: repo}

		boom := errors.New("replica down")
		repo.EXPECT().Find(gomock.Any(), organizationID, ledgerID, balanceID).Return(nil, boom)

		_, found, err := uc.AccountIDOfBalance(context.Background(), organizationID, ledgerID, balanceID)
		require.ErrorIs(t, err, boom)
		assert.False(t, found)
	})
}
