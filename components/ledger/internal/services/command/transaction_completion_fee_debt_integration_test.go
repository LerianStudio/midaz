//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
)

// interleavedMetadata runs between after a read returns, standing in for a write that lands
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

	t.Run("clearing update interleaved with the commit keeps its settlements", func(t *testing.T) {
		ctx, repo, complete := setup(t)
		commit, _, resolver := feeDebtLifecycleFixture(t)
		predecessor := commit.Dependencies[0]
		complete(resolver.records[transactionCompletionEvidenceIdentity(predecessor.TransactionID, predecessor.ExecutionID)])
		uc := &UseCase{TransactionMetadataRepo: interleavedMetadata{Repository: repo, between: func() { complete(&commit) }}}

		_, err := uc.UpdateTransactionMetadata(ctx, constant.EntityTransaction, feeDebtTransaction, nil)
		require.NoError(t, err)
		want := feeDebtCommittedMetadata()
		delete(want, "purpose")
		assert.Equal(t, want, stored(t, ctx, repo), "the commit's keys written after the read survive the clear")
	})
}
