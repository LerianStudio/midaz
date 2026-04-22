// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"

	// CreateAdditionalBalance creates a new additional balance.
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) CreateAdditionalBalance(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, cbi *mmodel.CreateAdditionalBalance) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_additional_balance")
	defer span.End()

	// Reserved key guard: "overdraft" (any case) is system-managed and MUST
	// not be created through the public API. This check runs BEFORE any
	// repository call so a rejected request never touches persistence.
	if strings.EqualFold(cbi.Key, constant.OverdraftBalanceKey) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reserved balance key", nil)
		logger.Log(ctx, libLog.LevelWarn, "Rejected reserved balance key", libLog.String("key", cbi.Key))

		return nil, pkg.ValidateBusinessError(constant.ErrReservedBalanceKey, constant.EntityBalance, cbi.Key)
	}

	// Direction validation runs before any repository call so invalid
	// payloads never touch persistence. Nil Direction is allowed and
	// defaults to "credit" at construction time below.
	if cbi.Direction != nil {
		switch *cbi.Direction {
		case constant.DirectionCredit, constant.DirectionDebit:
			// valid
		default:
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid balance direction", nil)
			logger.Log(ctx, libLog.LevelWarn, "Rejected invalid balance direction", libLog.String("direction", *cbi.Direction))

			return nil, pkg.ValidateBusinessError(constant.ErrInvalidBalanceDirection, constant.EntityBalance, *cbi.Direction)
		}
	}

	// Settings validation also runs pre-persistence to keep the repository
	// free of corrupt payloads.
	if cbi.Settings != nil {
		if err := cbi.Settings.Validate(); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid balance settings", err)
			logger.Log(ctx, libLog.LevelWarn, "Rejected invalid balance settings", libLog.Err(err))

			return nil, pkg.ValidateBusinessError(constant.ErrInvalidBalanceSettings, constant.EntityBalance)
		}
	}

	// Reserved scope guard: "internal" is reserved for system-managed
	// balances (e.g., the auto-created overdraft balance). A client-facing
	// request MUST NOT be able to create an internal-scoped balance —
	// those are permanently undeletable and bypass normal controls.
	if cbi.Settings != nil && cbi.Settings.BalanceScope == mmodel.BalanceScopeInternal {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reserved balance scope", nil)
		logger.Log(ctx, libLog.LevelWarn, "Rejected reserved balance scope", libLog.String("scope", cbi.Settings.BalanceScope))

		return nil, pkg.ValidateBusinessError(constant.ErrInvalidBalanceSettings, constant.EntityBalance)
	}

	existingBalance, err := uc.BalanceRepo.FindByAccountIDAndKey(ctx, organizationID, ledgerID, accountID, strings.ToLower(cbi.Key))
	if err != nil {
		var notFound pkg.EntityNotFoundError
		if !errors.As(err, &notFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to check if additional balance already exists", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to check if additional balance already exists: %v", err))

			return nil, err
		}
	}

	if existingBalance != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Additional balance already exists", nil)

		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Additional balance already exists: %v", cbi.Key))

		return nil, pkg.ValidateBusinessError(constant.ErrDuplicatedAliasKeyValue, constant.EntityBalance, cbi.Key)
	}

	defaultBalance, err := uc.BalanceRepo.FindByAccountIDAndKey(ctx, organizationID, ledgerID, accountID, constant.DefaultBalanceKey)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get default balance", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get default balance: %v", err))

		return nil, err
	}

	if defaultBalance.AccountType == constant.ExternalAccountType {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Additional balance not allowed for external account type", nil)

		return nil, pkg.ValidateBusinessError(constant.ErrAdditionalBalanceNotAllowed, constant.EntityBalance, defaultBalance.Alias)
	}

	// Direction defaults to "credit" when the caller omits it. Validation
	// above guarantees that any provided value is one of the supported
	// enum members.
	direction := constant.DirectionCredit
	if cbi.Direction != nil {
		direction = *cbi.Direction
	}

	additionalBalance := &mmodel.Balance{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
		Alias:          defaultBalance.Alias,
		Key:            strings.ToLower(cbi.Key),
		OrganizationID: defaultBalance.OrganizationID,
		LedgerID:       defaultBalance.LedgerID,
		AccountID:      defaultBalance.AccountID,
		AssetCode:      defaultBalance.AssetCode,
		AccountType:    defaultBalance.AccountType,
		AllowSending:   cbi.AllowSending == nil || *cbi.AllowSending,
		AllowReceiving: cbi.AllowReceiving == nil || *cbi.AllowReceiving,
		Direction:      direction,
		Settings:       cbi.Settings,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	created, err := uc.BalanceRepo.Create(ctx, additionalBalance)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating additional balance on repo: %v", err))

		return nil, err
	}

	return created, nil
}
