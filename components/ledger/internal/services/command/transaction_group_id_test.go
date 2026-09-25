// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestInitializeAtomicTransactionBatchIdentity_UsesGroupAsBatchID(t *testing.T) {
	t.Parallel()

	groupID := uuid.MustParse("01994f13-29b7-7000-8000-000000000211")
	uc := &UseCase{UUIDv7Generator: func() (uuid.UUID, error) {
		t.Fatal("a persisted group supplies the batch identity")
		return uuid.Nil, nil
	}}

	run, err := uc.initializeAtomicTransactionBatchIdentity(context.Background(), CreateAtomicTransactionBatchV2Input{
		GroupID: &groupID,
		Transactions: []CreateAtomicTransactionBatchV2ItemInput{{
			OrganizationID: uuid.MustParse("01994f13-29b7-7000-8000-000000000212"),
			LedgerID:       uuid.MustParse("01994f13-29b7-7000-8000-000000000213"),
			Action:         constant.ActionDirect,
			Order:          1,
		}},
	})

	require.NoError(t, err)
	assert.Equal(t, groupID, run.batchID)
	require.NotNil(t, run.groupID)
	assert.Equal(t, groupID, *run.groupID)
}

func TestBuildTransactionWriteSet_PreservesGroupID(t *testing.T) {
	t.Parallel()

	groupID := uuid.MustParse("01994f13-29b7-7000-8000-000000000201")
	payload, result := recoveryContractFixture(t)
	payload.GroupID = &groupID
	payload.IntentFingerprint, _ = ComputeEngineIntentFingerprint(recoveryContractIntent(payload))

	writeSet, err := BuildTransactionWriteSet(payload, result)

	require.NoError(t, err)
	require.NotNil(t, writeSet.Transaction.GroupID)
	assert.Equal(t, groupID.String(), *writeSet.Transaction.GroupID)
}
