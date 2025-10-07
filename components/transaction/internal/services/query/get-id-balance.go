// Package query implements read operations (queries) for the transaction service.
// This file contains query implementation.

package query

import (
	"context"
	"encoding/json"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// GetBalanceByID retrieves a single balance by ID with metadata.
//
// Fetches balance from PostgreSQL and enriches with MongoDB metadata.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - balanceID: UUID of the balance to retrieve
//
// Returns:
//   - *mmodel.Balance: Balance with metadata
//   - error: Business error if not found or query fails
//
// OpenTelemetry: Creates span "query.get_balance_by_id"
func (uc *UseCase) GetBalanceByID(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_balance_by_id")
	defer span.End()

	balance, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get balance on repo by id", err)

		logger.Errorf("Error getting balance: %v", err)

		return nil, err
	}

	if balance == nil {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.Balance{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance not found", err)

		logger.Warnf("Balance not found")

		return nil, err
	}

	// Overlay amounts from Redis cache when available to ensure freshest values
	internalKey := libCommons.BalanceInternalKey(organizationID.String(), ledgerID.String(), balance.Alias+"#"+balance.Key)

	value, rerr := uc.RedisRepo.Get(ctx, internalKey)
	if rerr != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balance cache value on redis", rerr)

		logger.Warnf("Failed to get balance cache value on redis: %v", rerr)
	}

	if value != "" {
		cached := mmodel.BalanceRedis{}
		if uerr := json.Unmarshal([]byte(value), &cached); uerr != nil {
			logger.Warnf("Error unmarshalling balance cache value: %v", uerr)
		} else {
			balance.Available = cached.Available
			balance.OnHold = cached.OnHold
			balance.Version = cached.Version
		}
	}

	return balance, nil
}
