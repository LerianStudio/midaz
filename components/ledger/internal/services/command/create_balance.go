// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"go.opentelemetry.io/otel/attribute"
)

// CreateDefaultBalance creates the default balance for a newly-created
// account or asset. The balance key is hardcoded to
// constant.DefaultBalanceKey and the direction to constant.DirectionCredit.
//
// Additional (non-default) balances are created via CreateAdditionalBalance,
// which is exposed through the POST /balances HTTP endpoint and applies
// its own validation rules (reserved keys, direction, settings).
//
// This function is the bootstrap path called inline from CreateAccount and
// CreateAsset; it is not exposed via an HTTP route. The input's Key field
// is ignored — the contract is encoded in the function name.
func (uc *UseCase) CreateDefaultBalance(ctx context.Context, input mmodel.CreateBalanceInput) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_default_balance")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", input.OrganizationID.String()),
		attribute.String("app.request.ledger_id", input.LedgerID.String()),
		attribute.String("app.request.account_id", input.AccountID.String()),
		attribute.String("app.request.asset_code", input.AssetCode),
		attribute.String("app.request.account_type", input.AccountType),
	)

	// Defensive duplicate guard. The account is normally brand-new at this
	// point, so no balance exists; this check catches retried compensation
	// paths and other edge cases where an orphan default row could already
	// be present.
	existsKey, err := uc.BalanceRepo.ExistsByAccountIDAndKey(ctx, input.OrganizationID, input.LedgerID, input.AccountID, constant.DefaultBalanceKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to check if balance already exists", err)
		logger.Log(ctx, libLog.LevelError, "Failed to check if balance already exists", libLog.Err(err))

		return nil, err
	}

	if existsKey {
		err := pkg.ValidateBusinessError(constant.ErrDuplicatedAliasKeyValue, constant.EntityBalance, constant.DefaultBalanceKey)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Default balance already exists", err)

		return nil, err
	}

	balanceUUID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to generate UUID for balance", err)
		logger.Log(ctx, libLog.LevelError, "Failed to generate UUID for balance", libLog.Err(err))

		return nil, err
	}

	// Default balances always use the "credit" direction. The migration
	// defines direction as NOT NULL with a CHECK constraint (credit|debit),
	// so we set the value explicitly rather than relying on the DB default —
	// the INSERT column list includes "direction" and would otherwise send
	// an empty string and fail the CHECK. Setting it here also keeps the
	// Go model consistent with the persisted row returned by RETURNING.
	now := time.Now()

	newBalance := &mmodel.Balance{
		ID:             balanceUUID.String(),
		Alias:          input.Alias,
		Key:            constant.DefaultBalanceKey,
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
		libOpentelemetry.HandleSpanError(span, "Failed to create balance on repo", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create balance on repo", libLog.Err(err))

		return nil, err
	}

	return created, nil
}
