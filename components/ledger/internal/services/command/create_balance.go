// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"

	// CreateBalanceSync creates a new balance synchronously using the request-supplied properties.
	// If key != "default", it validates that the default balance exists and that the account type allows additional balances.

	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
)

func (uc *UseCase) CreateBalanceSync(ctx context.Context, input mmodel.CreateBalanceInput) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_balance_sync")
	defer span.End()

	normalizedKey := strings.ToLower(strings.TrimSpace(input.Key))

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Creating balance for account id: %v with key: %v", input.AccountID, normalizedKey))

	if normalizedKey != constant.DefaultBalanceKey {
		existsDefault, err := uc.BalanceRepo.ExistsByAccountIDAndKey(ctx, input.OrganizationID, input.LedgerID, input.AccountID, constant.DefaultBalanceKey)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to check default balance existence", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to check default balance existence: %v", err))

			return nil, err
		}

		if !existsDefault {
			berr := pkg.ValidateBusinessError(constant.ErrDefaultBalanceNotFound, reflect.TypeOf(mmodel.Balance{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Default balance not found", berr)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Default balance not found: %v", berr))

			return nil, berr
		}

		// Validate additional balance not allowed for external account type
		if input.AccountType == constant.ExternalAccountType {
			err := pkg.ValidateBusinessError(constant.ErrAdditionalBalanceNotAllowed, reflect.TypeOf(mmodel.Balance{}).Name(), input.Alias)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Additional balance not allowed for external account type", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Additional balance not allowed for external account type: %v", err))

			return nil, err
		}
	}

	existsKey, err := uc.BalanceRepo.ExistsByAccountIDAndKey(ctx, input.OrganizationID, input.LedgerID, input.AccountID, normalizedKey)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to check if balance already exists", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to check if balance already exists: %v", err))

		return nil, err
	}

	if existsKey {
		err := pkg.ValidateBusinessError(constant.ErrDuplicatedAliasKeyValue, reflect.TypeOf(mmodel.Balance{}).Name(), normalizedKey)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Balance key already exists", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Balance key already exists: %v", err))

		return nil, err
	}

	// Default balances (created alongside an account or asset) always use
	// the "credit" direction. The migration defines direction as NOT NULL
	// with a CHECK constraint (credit|debit), so we must set an explicit
	// value rather than relying on the DB default — the INSERT column list
	// includes "direction" and would otherwise send an empty string and
	// fail the CHECK. Setting it here also ensures the Go model stays
	// consistent with the persisted row returned by RETURNING.
	now := time.Now()

	balanceUUID, uuidErr := libCommons.GenerateUUIDv7()
	if uuidErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to generate UUID for balance", uuidErr)
		logger.Log(ctx, libLog.LevelError, "Failed to generate UUID for balance", libLog.Err(uuidErr))

		return nil, uuidErr
	}

	newBalance := &mmodel.Balance{
		ID:             balanceUUID.String(),
		Alias:          input.Alias,
		Key:            normalizedKey,
		OrganizationID: input.OrganizationID.String(),
		LedgerID:       input.LedgerID.String(),
		AccountID:      input.AccountID.String(),
		AssetCode:      input.AssetCode,
		AccountType:    input.AccountType,
		AllowSending:   input.AllowSending,
		AllowReceiving: input.AllowReceiving,
		Direction:      constant.DirectionCredit,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	created, err := uc.BalanceRepo.Create(ctx, newBalance)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create balance on repo", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to create balance on repo: %v", err))

		return nil, err
	}

	return created, nil
}
