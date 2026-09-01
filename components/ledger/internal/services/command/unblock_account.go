// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// UnblockAccount clears the block on an account and drives that state into every
// read model the transactional hot path consults, converging account row,
// balance projection and balance cache exactly as BlockAccount does in the
// opposite direction.
//
// The transition rules — including the idempotent re-propagation that lets a
// partially-failed attempt be retried to convergence — live in
// setAccountBlockState, shared with BlockAccount so the two directions cannot
// drift apart.
func (uc *UseCase) UnblockAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, holderPolicy mmodel.HolderPolicy) (_ *mmodel.Account, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.unblock_account")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "unblock_account", start, err)
	}()

	return uc.setAccountBlockState(ctx, organizationID, ledgerID, accountID, false, holderPolicy)
}
