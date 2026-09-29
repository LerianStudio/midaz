//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"sync"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
)

// interleavedMetadata runs between after each read returns, standing in for a write that lands
// between a metadata update's read and its write.
type interleavedMetadata struct {
	mongodb.Repository
	between func()
}

func (repo interleavedMetadata) FindByEntity(ctx context.Context, collection, id string) (*mongodb.Metadata, error) {
	found, err := repo.Repository.FindByEntity(ctx, collection, id)
	repo.between()

	return found, err
}

func TestIntegrationFeeDebtMetadataOnMongo(t *testing.T) {
	setup := func(t *testing.T) (context.Context, *mongodb.MetadataMongoDBRepository, func(*TransactionWriteBehindEnvelope)) {
		container := mongotestutil.SetupReusableContainer(t)
		repo := mongodb.NewMetadataMongoDBRepository(mongotestutil.CreateConnection(t, container.URI, container.DBName))
		calls := []string{}
		service := NewTransactionCompletionService(&finalizationStoreStub{calls: &calls}, repo)
		_, _, resolver := feeDebtLifecycleFixture(t)
		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

		return ctx, repo, func(envelope *TransactionWriteBehindEnvelope) {
			_, err := CompleteTransactionWriteBehind(ctx, envelope, resolver, service)
			require.NoError(t, err)
		}
	}
	stored := func(t *testing.T, ctx context.Context, repo mongodb.Repository) mongodb.JSON {
		document, err := repo.FindByEntity(ctx, constant.EntityTransaction, feeDebtTransaction)
		require.NoError(t, err)
		require.NotNil(t, document)

		return document.Data
	}

	t.Run("commit replay and revert confirm the pending", func(t *testing.T) {
		ctx, repo, complete := setup(t)
		commit, revert, _ := feeDebtLifecycleFixture(t)
		complete(&commit)
		complete(&commit)
		complete(&revert)
		assert.Equal(t, feeDebtCommittedMetadata(), stored(t, ctx, repo))
	})

	t.Run("commit onto a document a client update wrote first", func(t *testing.T) {
		ctx, repo, complete := setup(t)
		commit, _, _ := feeDebtLifecycleFixture(t)
		uc := &UseCase{TransactionMetadataRepo: repo}
		_, err := uc.UpdateTransactionMetadata(ctx, constant.EntityTransaction, feeDebtTransaction, map[string]any{"note": "client"})
		require.NoError(t, err)

		complete(&commit)
		documents, err := repo.FindByEntityIDs(ctx, constant.EntityTransaction, []string{feeDebtTransaction})
		require.NoError(t, err)
		require.Len(t, documents, 1)
		want := feeDebtCommittedMetadata()
		want["note"] = "client"
		assert.Equal(t, want, documents[0].Data, "the frozen and fee-debt keys land under the client's")
		assert.Equal(t, constant.EntityTransaction, documents[0].EntityName, "completion marks the document completed")
	})

	t.Run("a key deleted after completion stays deleted through a revert", func(t *testing.T) {
		ctx, repo, complete := setup(t)
		commit, revert, _ := feeDebtLifecycleFixture(t)
		uc := &UseCase{TransactionMetadataRepo: repo}
		_, err := uc.UpdateTransactionMetadata(ctx, constant.EntityTransaction, feeDebtTransaction, map[string]any{"note": "client"})
		require.NoError(t, err)
		complete(&commit)

		_, err = uc.UpdateTransactionMetadata(ctx, constant.EntityTransaction, feeDebtTransaction, map[string]any{"purpose": nil})
		require.NoError(t, err)
		complete(&revert)
		want := feeDebtCommittedMetadata()
		want["note"] = "client"
		delete(want, "purpose")
		assert.Equal(t, want, stored(t, ctx, repo), "re-completing the origin adds no key the client deleted")
	})

	t.Run("client update keeps the fee-debt keys", func(t *testing.T) {
		ctx, repo, complete := setup(t)
		commit, _, _ := feeDebtLifecycleFixture(t)
		complete(&commit)
		uc := &UseCase{TransactionMetadataRepo: repo}

		_, err := uc.UpdateTransactionMetadata(ctx, constant.EntityTransaction, feeDebtTransaction, map[string]any{"purpose": "edited", "note": "client"})
		require.NoError(t, err)
		want := feeDebtCommittedMetadata()
		want["purpose"], want["note"] = "edited", "client"
		assert.Equal(t, want, stored(t, ctx, repo))

		_, err = uc.UpdateTransactionMetadata(ctx, constant.EntityTransaction, feeDebtTransaction, nil)
		require.NoError(t, err)
		delete(want, "purpose")
		delete(want, "note")
		assert.Equal(t, want, stored(t, ctx, repo), "clearing drops only client keys")
	})

	for _, sent := range []map[string]any{nil, {"note": "client"}} {
		t.Run("update interleaved with the commit keeps its fee-debt keys", func(t *testing.T) {
			ctx, repo, complete := setup(t)
			commit, _, resolver := feeDebtLifecycleFixture(t)
			predecessor := commit.Dependencies[0]
			complete(resolver.records[transactionCompletionEvidenceIdentity(predecessor.TransactionID, predecessor.ExecutionID)])
			var once sync.Once
			uc := &UseCase{TransactionMetadataRepo: interleavedMetadata{Repository: repo, between: func() { once.Do(func() { complete(&commit) }) }}}

			_, err := uc.UpdateTransactionMetadata(ctx, constant.EntityTransaction, feeDebtTransaction, sent)
			require.NoError(t, err)
			want := feeDebtCommittedMetadata()
			if sent == nil {
				delete(want, "purpose")
			} else {
				want["note"] = "client"
			}
			assert.Equal(t, want, stored(t, ctx, repo), "the commit's keys written after the read survive the update")
		})
	}
}
