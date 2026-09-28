// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
)

func (uc *UseCase) finalizeAtomicTransactionBatch(
	ctx context.Context,
	run *atomicTransactionBatchRun,
	transactions []*transaction.Transaction,
) error {
	if run == nil || !run.idempotencyClaimed || uc.AtomicTransactionBatchIdempotencyRepo == nil {
		return nil
	}

	if !run.idempotencyHandedOff || run.executionID == uuid.Nil {
		return errors.New("atomic transaction batch cannot finalize before execution handoff")
	}

	if len(transactions) != len(run.items) {
		return errors.New("atomic transaction batch terminal response cardinality differs")
	}

	responses := make(map[uuid.UUID]json.RawMessage, len(transactions))
	for index, tran := range transactions {
		if tran == nil || tran.ID != run.items[index].transactionID.String() {
			return errors.New("atomic transaction batch terminal response identity differs")
		}

		payload, err := json.Marshal(tran)
		if err != nil {
			return fmt.Errorf("marshal atomic transaction batch terminal item %d: %w", index, err)
		}

		responses[run.items[index].transactionID] = payload
	}

	// Zero asks the Redis adapter to derive the terminal replay TTL from the
	// execution receipt's frozen protection window in the same-slot CAS.
	result, err := uc.AtomicTransactionBatchIdempotencyRepo.FinalizeAtomicTransactionBatch(
		ctx,
		run.coordinationOrganizationID,
		run.coordinationLedgerID,
		run.executionID,
		run.idempotencyOwnerToken,
		responses,
		0,
	)
	if err != nil {
		return fmt.Errorf("finalize atomic transaction batch response: %w", err)
	}

	if result == nil ||
		(result.Outcome != txRedis.AtomicTransactionBatchFinalized &&
			result.Outcome != txRedis.AtomicTransactionBatchAlreadyComplete) {
		return errors.New("finalize atomic transaction batch response: invalid outcome")
	}

	return nil
}
