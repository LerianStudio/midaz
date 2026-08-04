// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/google/uuid"
)

// resolveDefaultBalanceDirectionForType returns the account type's configured
// default balance direction for the initial-balance path. External accounts
// bypass the lookup entirely (their direction is fixed by the account-type-
// implied default downstream), so the result is empty for them and the lookup
// is skipped. For non-external types it delegates to resolveTypeDefaultDirection,
// which is best-effort and never fails the caller.
func (uc *UseCase) resolveDefaultBalanceDirectionForType(ctx context.Context, organizationID, ledgerID uuid.UUID, accountType string, isExternal bool) string {
	if isExternal {
		return ""
	}

	return uc.resolveTypeDefaultDirection(ctx, organizationID, ledgerID, accountType)
}

// resolveTypeDefaultDirection returns the account type's configured default
// balance direction for the given key, best-effort. It is graceful by design:
// an unwired repository, a not-found type, or any technical error resolves to
// the empty string ("no type default") and never fails the caller, so the
// direction falls back to the account-type-implied default downstream. The
// caller MUST NOT invoke this for external account types.
func (uc *UseCase) resolveTypeDefaultDirection(ctx context.Context, organizationID, ledgerID uuid.UUID, accountType string) string {
	if uc.AccountTypeRepo == nil || accountType == "" {
		return ""
	}

	if ctx.Err() != nil {
		return ""
	}

	logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)

	at, err := uc.AccountTypeRepo.FindByKey(ctx, organizationID, ledgerID, accountType)
	if err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Account type default direction lookup miss; falling back to implied default",
			libLog.String("account_type", accountType))

		return ""
	}

	if at == nil {
		return ""
	}

	return at.DefaultDirection
}
