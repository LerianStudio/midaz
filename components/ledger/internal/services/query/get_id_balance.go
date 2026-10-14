// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"

	// GetBalanceByID gets data in the repository.
	libLog "github.com/LerianStudio/lib-observability/v4/log"
)

func (uc *UseCase) GetBalanceByID(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_balance_by_id")
	defer span.End()

	balance, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		recordQueryError(ctx, span, logger, "Failed to get balance on repo by id", err)

		return nil, err
	}

	if balance == nil {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Balance not found", err)

		return nil, err
	}

	// Overlay amounts from Redis cache when available to ensure freshest values
	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, balance.Alias+"#"+balance.Key)

	value, rerr := uc.TransactionRedisRepo.Get(ctx, internalKey)
	if rerr != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balance cache value on redis", rerr)

		logger.Log(ctx, libLog.LevelWarn, "Failed to get balance cache value on redis", libLog.Err(rerr))
	}

	if value != "" {
		cached := mmodel.BalanceRedis{}
		if uerr := json.Unmarshal([]byte(value), &cached); uerr != nil {
			logger.Log(ctx, libLog.LevelWarn, "Error unmarshalling balance cache value", libLog.Err(uerr))
		} else {
			applyBalanceCacheOverlay(balance, &cached)
		}
	}

	return balance, nil
}
