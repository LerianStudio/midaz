// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"

	// GetBalanceByID gets data in the repository.
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
)

func (uc *UseCase) GetBalanceByID(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_balance_by_id")
	defer span.End()

	balance, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get balance on repo by id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error getting balance: %v", err))

		return nil, err
	}

	if balance == nil {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Balance not found", err)

		logger.Log(ctx, libLog.LevelWarn, "Balance not found")

		return nil, err
	}

	// Overlay amounts from Redis cache when available to ensure freshest values
	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, balance.Alias+"#"+balance.Key)

	value, rerr := uc.TransactionRedisRepo.Get(ctx, internalKey)
	if rerr != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balance cache value on redis", rerr)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to get balance cache value on redis: %v", rerr))
	}

	if value != "" {
		cached := mmodel.BalanceRedis{}
		if uerr := json.Unmarshal([]byte(value), &cached); uerr != nil {
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Error unmarshalling balance cache value: %v", uerr))
		} else {
			applyBalanceCacheOverlay(balance, &cached)
		}
	}

	return balance, nil
}
