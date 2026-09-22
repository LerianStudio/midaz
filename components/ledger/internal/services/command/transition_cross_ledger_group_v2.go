// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func (uc *UseCase) transitionCrossLedgerGroupV2(
	ctx context.Context,
	in PendingTransitionInput,
	target *transaction.Transaction,
	status string,
) (*CreateAtomicTransactionBatchV2Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if target == nil || target.GroupID == nil {
		return nil, pkg.ValidateBusinessError(constant.ErrCrossLedgerGroupIncomplete, constant.EntityTransaction)
	}
	if uc.TransactionGroupRepo == nil {
		return nil, errors.New("cross-ledger transaction group repository is not configured")
	}

	groupID, err := uuid.Parse(*target.GroupID)
	if err != nil || groupID == uuid.Nil {
		return nil, fmt.Errorf("parse cross-ledger transaction group id: %w", ErrInvalidTransactionCompletionRecord)
	}

	group, err := uc.TransactionGroupRepo.FindByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil || group.ID != groupID {
		return nil, pkg.ValidateBusinessError(constant.ErrCrossLedgerGroupIncomplete, constant.EntityTransaction)
	}
	if group.Status != constant.PENDING {
		return nil, pkg.ValidateBusinessError(
			constant.ErrCrossLedgerGroupNotPending,
			constant.EntityTransaction,
			group.Status,
		)
	}

	return nil, errors.New("cross-ledger group lifecycle execution is not configured")
}
