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

// captureAtomicTransactionBatchInitialResponse freezes the public creation
// representation while the completion result is still authoritative. It runs
// before the corresponding recovery ACK can remove its only source evidence.
func (uc *UseCase) captureAtomicTransactionBatchInitialResponse(
	ctx context.Context,
	run *atomicTransactionBatchRun,
	transactionID uuid.UUID,
	tran *transaction.Transaction,
) error {
	if run == nil || !run.idempotencyClaimed || uc.AtomicTransactionBatchIdempotencyRepo == nil {
		return nil
	}

	if !run.idempotencyHandedOff || run.executionID == uuid.Nil {
		return errors.New("atomic transaction batch cannot capture before execution handoff")
	}

	if tran == nil || tran.ID != transactionID.String() {
		return errors.New("atomic transaction batch initial response identity differs")
	}

	payload, err := json.Marshal(tran)
	if err != nil {
		return fmt.Errorf("marshal atomic transaction batch initial response: %w", err)
	}

	result, err := uc.AtomicTransactionBatchIdempotencyRepo.CaptureAtomicTransactionBatchInitialResponse(
		ctx,
		run.organizationID,
		run.ledgerID,
		run.executionID,
		run.idempotencyOwnerToken,
		transactionID,
		payload,
	)
	if err != nil {
		return fmt.Errorf("capture atomic transaction batch initial response: %w", err)
	}

	if result == nil || (result.Outcome != txRedis.AtomicTransactionBatchInitialResponseCaptured &&
		result.Outcome != txRedis.AtomicTransactionBatchInitialResponseAlreadyCaptured) {
		return errors.New("capture atomic transaction batch initial response: invalid outcome")
	}

	return nil
}
