// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"reflect"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// CreateBalanceSync creates a new balance synchronously using the request-supplied properties.
// If key != "default", it validates that the default balance exists and that the account type allows additional balances.
// This method implements mbootstrap.BalancePort, allowing the transaction module
// to be used directly by the onboarding module in unified ledger mode.
func (uc *UseCase) CreateBalanceSync(ctx context.Context, input mmodel.CreateBalanceInput) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_balance_sync")
	defer span.End()

	normalizedKey := strings.ToLower(strings.TrimSpace(input.Key))

	logger.Infof("Creating balance for account id: %v with key: %v", input.AccountID, normalizedKey)

	if normalizedKey != constant.DefaultBalanceKey {
		existsDefault, err := uc.BalanceRepo.ExistsByAccountIDAndKey(ctx, input.OrganizationID, input.LedgerID, input.AccountID, constant.DefaultBalanceKey)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to check default balance existence", err)

			logger.Errorf("Failed to check default balance existence: %v", err)

			return nil, err
		}

		if !existsDefault {
			berr := pkg.ValidateBusinessError(constant.ErrDefaultBalanceNotFound, reflect.TypeOf(mmodel.Balance{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Default balance not found", berr)

			logger.Errorf("Default balance not found: %v", berr)

			return nil, berr
		}

		// Validate additional balance not allowed for external account type
		if input.AccountType == constant.ExternalAccountType {
			err := pkg.ValidateBusinessError(constant.ErrAdditionalBalanceNotAllowed, reflect.TypeOf(mmodel.Balance{}).Name(), input.Alias)

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Additional balance not allowed for external account type", err)

			logger.Errorf("Additional balance not allowed for external account type: %v", err)

			return nil, err
		}
	}

	existsKey, err := uc.BalanceRepo.ExistsByAccountIDAndKey(ctx, input.OrganizationID, input.LedgerID, input.AccountID, normalizedKey)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to check if balance already exists", err)

		logger.Errorf("Failed to check if balance already exists: %v", err)

		return nil, err
	}

	if existsKey {
		err := pkg.ValidateBusinessError(constant.ErrDuplicatedAliasKeyValue, reflect.TypeOf(mmodel.Balance{}).Name(), normalizedKey)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance key already exists", err)

		logger.Errorf("Balance key already exists: %v", err)

		return nil, err
	}

	newBalance := &mmodel.Balance{
		ID:             libCommons.GenerateUUIDv7().String(),
		Alias:          input.Alias,
		Key:            normalizedKey,
		OrganizationID: input.OrganizationID.String(),
		LedgerID:       input.LedgerID.String(),
		AccountID:      input.AccountID.String(),
		AssetCode:      input.AssetCode,
		AccountType:    input.AccountType,
		AllowSending:   input.AllowSending,
		AllowReceiving: input.AllowReceiving,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := uc.BalanceRepo.Create(ctx, newBalance); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create balance on repo", err)

		logger.Errorf("Failed to create balance on repo: %v", err)

		return nil, err
	}

	return newBalance, nil
}
