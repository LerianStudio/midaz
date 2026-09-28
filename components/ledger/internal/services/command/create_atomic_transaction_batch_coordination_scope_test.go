// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAtomicTransactionBatchCoordinationScope_ClaimIgnoresFirstLedger(t *testing.T) {
	organizationID := uuid.MustParse("01995190-0000-7000-8000-000000000001")
	ledgerA := uuid.MustParse("01995190-0000-7000-8000-000000000002")
	ledgerB := uuid.MustParse("01995190-0000-7000-8000-000000000003")
	request := func(first, second uuid.UUID) CreateAtomicTransactionBatchV2Input {
		return CreateAtomicTransactionBatchV2Input{
			Transactions: []CreateAtomicTransactionBatchV2ItemInput{
				atomicTransactionBatchItemInput(organizationID, first, "@source-first", "@destination-first"),
				atomicTransactionBatchItemInput(organizationID, second, "@source-second", "@destination-second"),
			},
			IdempotencyKey: "same-key",
		}
	}
	claimScope := func(in CreateAtomicTransactionBatchV2Input) (uuid.UUID, uuid.UUID) {
		t.Helper()
		repository := &atomicTransactionBatchClaimRepositoryFake{}
		uc := &UseCase{UUIDv7Generator: func() (uuid.UUID, error) { return uuid.New(), nil }, AtomicTransactionBatchIdempotencyRepo: repository}
		run, err := uc.initializeAtomicTransactionBatchIdentity(context.Background(), in)
		require.NoError(t, err)
		_, err = uc.claimAtomicTransactionBatch(context.Background(), in, run)
		require.NoError(t, err)
		return repository.claimOrganizationID, repository.claimLedgerID
	}

	orgA, scopeA := claimScope(request(ledgerA, ledgerB))
	orgB, scopeB := claimScope(request(ledgerB, ledgerA))
	require.Equal(t, organizationID, orgA)
	require.Equal(t, orgA, orgB)
	require.Equal(t, ledgerA, scopeA)
	require.Equal(t, scopeA, scopeB, "the same participant set must share one claim regardless of item order")

	_, singleScope := claimScope(CreateAtomicTransactionBatchV2Input{
		Transactions: []CreateAtomicTransactionBatchV2ItemInput{
			atomicTransactionBatchItemInput(organizationID, ledgerB, "@single-source", "@single-destination"),
		},
	})
	require.Equal(t, ledgerB, singleScope, "single-ledger keys must keep their existing scope")
}
