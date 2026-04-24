// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
)

// VerifyAccountsTransactable loads the accounts referenced by the given IDs
// and returns a terminal business error when any of them is not eligible to
// participate in a transaction. An account is ineligible when:
//
//   - account.Status.Code == PENDING_CRM_LINK, or
//   - account.Status.Code == FAILED_CRM_LINK, or
//   - account.Blocked == true.
//
// This is the account-level gate for the Ledger-owned CRM saga. It runs in
// addition to the existing balance-level gate (AllowSending / AllowReceiving
// flags validated inside mtransaction.ValidateBalancesRules): the balance
// flags are defense-in-depth, and this account check guarantees that any
// pending or blocked account rejects the transaction even if a stale balance
// row somehow has transact-friendly flags.
//
// Returns constant.ErrAccountStatusTransactionRestriction (the existing
// account-level eligibility sentinel, 0024) on failure so callers and API
// consumers see the same error they already know for
// INACTIVE/SUSPENDED/DELETED accounts.
//
// A nil or empty ids slice returns no error (nothing to verify).
func (uc *UseCase) VerifyAccountsTransactable(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.verify_accounts_transactable")
	defer span.End()

	if len(ids) == 0 {
		return nil
	}

	accounts, err := uc.AccountRepo.ListAccountsByIDs(ctx, organizationID, ledgerID, ids)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to load accounts for transaction eligibility check", err)
		logger.Log(ctx, libLog.LevelError, "Failed to load accounts for transaction eligibility check", libLog.Err(err))

		return err
	}

	for _, acc := range accounts {
		if acc == nil {
			continue
		}

		blocked := acc.Blocked != nil && *acc.Blocked

		if acc.Status.Code == constant.AccountStatusPendingCRMLink ||
			acc.Status.Code == constant.AccountStatusFailedCRMLink ||
			blocked {
			bizErr := pkg.ValidateBusinessError(constant.ErrAccountStatusTransactionRestriction, constant.EntityAccount)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Account ineligible for transaction", bizErr)
			logger.Log(ctx, libLog.LevelWarn, "Account ineligible for transaction",
				libLog.String("account_id", acc.ID),
				libLog.String("status", acc.Status.Code),
				libLog.Bool("blocked", blocked))

			return bizErr
		}
	}

	return nil
}
