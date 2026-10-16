// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// reserveSourceScopeUnresolvedEvent is the span event an advisory reserve
// records when the source account's segment and portfolio could not be read.
const reserveSourceScopeUnresolvedEvent = "reserve source account scope unresolved"

// reservationActive reports whether a reserve is built and sent at all: a
// reservation client is injected, the ledger mode is advisory or enforce, and
// no per-call skip was honored. The reserve anchor and the source account
// resolution share this one condition, so an inactive seam does no work in
// either.
func reservationActive(reserver TracerReserver, settings mmodel.TracerSettings, honoredTracerSkip bool) bool {
	return reserver != nil && settings.Mode != mmodel.TracerModeOff && settings.Mode != "" && !honoredTracerSkip
}

// reserveSourceAccount builds the account the reserve carries: the id and type
// of the first internal source balance plus that account's segment and
// portfolio, which the balance row does not hold and both change by an account
// update, so they are read from the account at reserve time. The account is
// read in the organization and ledger the balance belongs to.
//
// The read is best effort except for one case:
//
//   - inactive seam, no internal source account or no account repository: no
//     read; the account goes without segment and portfolio (zero when inactive).
//   - an id that is not a UUID, or an account no longer found: no scope, Warn.
//   - any other read failure under advisory: no scope, Warn and a span event,
//     because advisory never blocks.
//   - any other read failure under enforce: the error is returned wrapped, and
//     the caller fails the create before reserving, exactly as a rejected
//     reservation.
func (uc *UseCase) reserveSourceAccount(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	settings mmodel.TracerSettings,
	honoredTracerSkip bool,
	sources []string,
	balances []*mmodel.Balance,
) (tracer.ReserveAccount, error) {
	if !reservationActive(uc.TracerReserver, settings, honoredTracerSkip) {
		return tracer.ReserveAccount{}, nil
	}

	account, balance := firstSourceAccount(sources, balances)
	if balance == nil {
		return tracer.ReserveAccount{}, nil
	}

	if uc.AccountRepo == nil {
		return account, nil
	}

	organizationID, ledgerID, accountID, err := parseSourceBalanceIDs(balance)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Source account ids are not UUIDs; reserving without segment and portfolio",
			libLog.String("account_id", balance.AccountID),
			libLog.Err(err))

		return account, nil
	}

	found, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1)
	if err != nil {
		return uc.handleSourceAccountReadError(ctx, span, logger, settings, account, err)
	}

	if found == nil {
		logger.Log(ctx, libLog.LevelWarn, "Source account not found; reserving without segment and portfolio",
			libLog.String("account_id", balance.AccountID))

		return account, nil
	}

	if found.SegmentID != nil {
		account.SegmentID = *found.SegmentID
	}

	if found.PortfolioID != nil {
		account.PortfolioID = *found.PortfolioID
	}

	span.SetAttributes(
		attribute.Bool("app.tracer.source_segment_resolved", account.SegmentID != ""),
		attribute.Bool("app.tracer.source_portfolio_resolved", account.PortfolioID != ""),
	)

	return account, nil
}

// handleSourceAccountReadError applies the failure policy of the source
// account read. A missing account is a fact about the account, never a reason
// to fail the create; any other failure blocks only under enforce.
func (uc *UseCase) handleSourceAccountReadError(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	settings mmodel.TracerSettings,
	account tracer.ReserveAccount,
	err error,
) (tracer.ReserveAccount, error) {
	var notFound pkg.EntityNotFoundError
	if errors.As(err, &notFound) {
		logger.Log(ctx, libLog.LevelWarn, "Source account not found; reserving without segment and portfolio",
			libLog.String("account_id", account.AccountID))

		return account, nil
	}

	if settings.Mode == mmodel.TracerModeAdvisory {
		span.AddEvent(reserveSourceScopeUnresolvedEvent, trace.WithAttributes(
			attribute.String("app.tracer.source_account_id", account.AccountID),
			attribute.String("error", err.Error()),
		))
		logger.Log(ctx, libLog.LevelWarn, "Failed to read the source account in advisory mode; reserving without segment and portfolio",
			libLog.String("account_id", account.AccountID),
			libLog.Err(err))

		return account, nil
	}

	libOpentelemetry.HandleSpanError(span, "Failed to read the source account for the reserve", err)
	logger.Log(ctx, libLog.LevelError, "Failed to read the source account for the reserve; rejecting transaction",
		libLog.String("account_id", account.AccountID),
		libLog.Err(err))

	return tracer.ReserveAccount{}, fmt.Errorf("read reserve source account %s: %w", account.AccountID, err)
}

// parseSourceBalanceIDs parses the organization, ledger and account ids the
// source balance carries.
func parseSourceBalanceIDs(balance *mmodel.Balance) (uuid.UUID, uuid.UUID, uuid.UUID, error) {
	organizationID, err := uuid.Parse(balance.OrganizationID)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, fmt.Errorf("organization id: %w", err)
	}

	ledgerID, err := uuid.Parse(balance.LedgerID)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, fmt.Errorf("ledger id: %w", err)
	}

	accountID, err := uuid.Parse(balance.AccountID)
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, fmt.Errorf("account id: %w", err)
	}

	return organizationID, ledgerID, accountID, nil
}
