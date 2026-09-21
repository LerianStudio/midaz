// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// CreateDefaultBalance creates the default balance for a newly-created
// account or asset. The balance key is hardcoded to
// constant.DefaultBalanceKey and the direction is resolved by precedence:
// the account type's default (input.DefaultDirection, when set) wins,
// otherwise the account-type-implied default applies (external ->
// constant.DirectionDebit, all others -> constant.DirectionCredit). The
// default path has no explicit caller override.
//
// Additional (non-default) balances are created via CreateAdditionalBalance,
// which is exposed through the POST /balances HTTP endpoint and applies
// its own validation rules (reserved keys, direction, settings).
//
// This function is the bootstrap path called inline from CreateAccount and
// CreateAsset; it is not exposed via an HTTP route. The input's Key field
// is ignored — the contract is encoded in the function name.
func (uc *UseCase) CreateDefaultBalance(ctx context.Context, input mmodel.CreateBalanceInput) (_ *mmodel.Balance, err error) {
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

	// The account is normally brand-new here, but "normally" is not a guarantee:
	// this path is also reached by retried compensation, and a closing racing the
	// account creation would otherwise persist a balance into an account that is
	// already being closed. So the default balance takes the same per-account
	// ownership every other admitting writer takes.
	//
	// External is the one account type outside the coordination, because it is
	// ineligible for closing (0074) and therefore has no closing to race. Its
	// balance is provisioned by the asset flow, which must not depend on the
	// transaction cache being reachable.
	// writeIssued opens the window in which the ownership may no longer be given
	// back on an unresolved failure.
	writeIssued := false

	if !strings.EqualFold(input.AccountType, constant.ExternalAccountType) {
		admission, admissionErr := uc.acquireAccountAdmission(ctx, input.OrganizationID, input.LedgerID, input.AccountID)
		if admissionErr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to protect the account for the default balance", admissionErr)

			return nil, admissionErr
		}

		defer func() { resolveAccountAdmission(ctx, admission, writeIssued, err) }()

		if closedErr := uc.ensureAccountsNotClosed(ctx, input.OrganizationID, input.LedgerID, constant.ErrAccountClosed, input.AccountID); closedErr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Refused to create the default balance of a closed account", closedErr)
			logger.Log(ctx, libLog.LevelWarn, "Refused to create the default balance of a closed account", libLog.Err(closedErr))

			return nil, closedErr
		}
	}

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

	// The migration defines direction as NOT NULL with a CHECK constraint
	// (credit|debit), so we set the value explicitly rather than relying on
	// the DB default — the INSERT column list includes "direction" and would
	// otherwise send an empty string and fail the CHECK. Setting it here also
	// keeps the Go model consistent with the persisted row returned by
	// RETURNING.
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
		Direction:      resolveBalanceDirection("", input.DefaultDirection, input.AccountType),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	writeIssued = true

	created, err := uc.BalanceRepo.Create(ctx, newBalance)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create balance on repo", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create balance on repo", libLog.Err(err))

		return nil, err
	}

	return created, nil
}

// defaultBalanceDirection returns the balance direction implied by an account
// type: debit for external accounts, credit for everything else.
func defaultBalanceDirection(accountType string) string {
	if strings.EqualFold(accountType, constant.ExternalAccountType) {
		return constant.DirectionDebit
	}

	return constant.DirectionCredit
}
