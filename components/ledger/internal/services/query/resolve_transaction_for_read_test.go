// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	postgres "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// durableEngineEvidence re-encodes the fixture's index and envelope as
// acknowledged, as the acknowledgement script does to both at once: the
// write-behind completion reached PostgreSQL and Mongo. An entry keeps this
// state for the whole idempotency retention before cleanup removes it.
func durableEngineEvidence(t *testing.T, pendingIndex, pendingEnvelope []byte) ([]byte, []byte) {
	t.Helper()

	index, err := command.DecodeTransactionEvidenceIndex(pendingIndex)
	require.NoError(t, err)

	index.DurabilityState = command.TransactionDurabilityComplete

	encodedIndex, err := command.EncodeTransactionEvidenceIndex(*index)
	require.NoError(t, err)

	envelope, err := command.DecodeTransactionWriteBehindEnvelope(pendingEnvelope)
	require.NoError(t, err)

	envelope.DurabilityState = command.TransactionDurabilityComplete

	encodedEnvelope, err := command.EncodeTransactionWriteBehindEnvelope(*envelope)
	require.NoError(t, err)

	return encodedIndex, encodedEnvelope
}

func TestResolveTransactionForReadUsesPrimaryOnceEngineStateIsDurable(t *testing.T) {
	pendingIndex, envelope, receipt, engineView := queryEngineWriteBehindFixture(t)
	organizationID := uuid.MustParse(engineView.OrganizationID)
	ledgerID := uuid.MustParse(engineView.LedgerID)
	transactionID := uuid.MustParse(engineView.ID)

	ctrl := gomock.NewController(t)
	transactionRepo := postgres.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	// PATCH wrote these to the primary and Mongo after the engine state was frozen.
	edited := &postgres.Transaction{
		ID: engineView.ID, OrganizationID: engineView.OrganizationID, LedgerID: engineView.LedgerID,
		Description: "edited after persistence",
	}
	transactionRepo.EXPECT().
		FindWithOperations(gomock.Cond(func(ctx context.Context) bool { return readrouting.IsPrimaryRead(ctx) }), organizationID, ledgerID, transactionID).
		Return(edited, nil)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityTransaction, transactionID.String()).
		Return(&mongodb.Metadata{Data: map[string]any{"channel": "api", "edited": true}}, nil)

	durableIndex, durableEnvelope := durableEngineEvidence(t, pendingIndex, envelope)
	fake := &engineWriteBehindRepositoryFake{index: durableIndex, envelope: durableEnvelope, receipt: receipt, materializeResult: true}
	uc := &UseCase{
		EngineWriteBehindRepo: fake, EngineWriteBehindCodec: command.EngineWriteBehindEvidenceCodec{},
		TransactionRepo: transactionRepo, TransactionMetadataRepo: metadataRepo,
	}
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-query")

	resolved, err := uc.ResolveTransactionForRead(ctx, organizationID, ledgerID, transactionID)
	require.NoError(t, err)
	require.Equal(t, EngineTransactionResolutionPrimary, resolved.Source)
	require.Equal(t, "edited after persistence", resolved.Transaction.Description)
	require.Equal(t, map[string]any{"channel": "api", "edited": true}, resolved.Transaction.Metadata)
	require.Zero(t, fake.materializedCalls, "a durable read must not rebuild the engine view in Redis")
}

func TestResolveTransactionForReadServesEngineStateWhilePending(t *testing.T) {
	pendingIndex, envelope, receipt, engineView := queryEngineWriteBehindFixture(t)

	// No expectations: the primary holds nothing yet, so reading it is a defect.
	transactionRepo := postgres.NewMockRepository(gomock.NewController(t))

	fake := &engineWriteBehindRepositoryFake{
		index: pendingIndex, envelope: envelope, receipt: receipt,
		materializedErr: redis.ErrEngineWriteBehindNotFound, materializeResult: true,
	}
	uc := &UseCase{EngineWriteBehindRepo: fake, EngineWriteBehindCodec: command.EngineWriteBehindEvidenceCodec{}, TransactionRepo: transactionRepo}
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-query")

	resolved, err := uc.ResolveTransactionForRead(ctx, uuid.MustParse(engineView.OrganizationID), uuid.MustParse(engineView.LedgerID), uuid.MustParse(engineView.ID))
	require.NoError(t, err)
	require.Equal(t, EngineTransactionResolutionEvidence, resolved.Source)
	require.True(t, resolved.Pending)
	require.Equal(t, engineView.ID, resolved.Transaction.ID)
}

func TestResolveEngineWriteBehindTransactionKeepsDurableExecutionForLifecycleWrites(t *testing.T) {
	pendingIndex, envelope, receipt, engineView := queryEngineWriteBehindFixture(t)

	// Commit, cancel and revert name this execution as their predecessor, so the
	// lifecycle resolution must not trade it for the primary row.
	transactionRepo := postgres.NewMockRepository(gomock.NewController(t))

	durableIndex, durableEnvelope := durableEngineEvidence(t, pendingIndex, envelope)
	fake := &engineWriteBehindRepositoryFake{
		index: durableIndex, envelope: durableEnvelope, receipt: receipt,
		materializedErr: redis.ErrEngineWriteBehindNotFound, materializeResult: true,
	}
	uc := &UseCase{EngineWriteBehindRepo: fake, EngineWriteBehindCodec: command.EngineWriteBehindEvidenceCodec{}, TransactionRepo: transactionRepo}
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-query")

	resolved, err := uc.ResolveEngineWriteBehindTransaction(ctx, uuid.MustParse(engineView.OrganizationID), uuid.MustParse(engineView.LedgerID), uuid.MustParse(engineView.ID))
	require.NoError(t, err)
	require.Equal(t, EngineTransactionResolutionEvidence, resolved.Source)
	require.False(t, resolved.Pending)
	require.Equal(t, expectedExecutionID(t), resolved.ExecutionID)
}
