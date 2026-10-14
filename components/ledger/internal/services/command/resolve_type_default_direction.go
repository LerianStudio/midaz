// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
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
//
// A miss (not-found or any business error) is expected and logs at Debug only.
// A technical error is tolerated by the same fallback, so it does not fail the
// operation and must not change the caller's span status: it is recorded as a
// plain event on the current span and logged once at Warn with the error,
// because the repository records it on its own span without logging.
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
		const message = "Account type default direction lookup miss; falling back to implied default"

		if pkg.IsBusinessError(err) || errors.Is(err, services.ErrDatabaseItemNotFound) {
			logger.Log(ctx, libLog.LevelDebug, message,
				libLog.String("account_type", accountType))

			return ""
		}

		libOpentelemetry.HandleSpanEvent(trace.SpanFromContext(ctx), message)
		logger.Log(ctx, libLog.LevelWarn, message,
			libLog.String("account_type", accountType),
			libLog.Err(err))

		return ""
	}

	if at == nil {
		return ""
	}

	return at.DefaultDirection
}
