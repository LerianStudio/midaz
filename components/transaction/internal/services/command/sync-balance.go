// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// SyncBalance is responsible to sync balance from redis to database.
// The cache balance might take a long time to be persisted in the database, so this function syncs the balance before the key expires, to avoid data inconsistency.
func (uc *UseCase) SyncBalance(ctx context.Context, organizationID, ledgerID uuid.UUID, balance mmodel.BalanceRedis) (bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.sync_balance")
	defer span.End()

	synchedBalance, err := uc.BalanceRepo.Sync(ctx, organizationID, ledgerID, balance)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to sync balance from redis", err)

		logger.Errorf("Failed to sync balance from redis: %v", err)

		return false, err
	}

	if !synchedBalance {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance is newer, skipping sync", nil)

		logger.Infof("Balance is newer, skipping sync")

		return false, nil
	}

	return true, nil
}
