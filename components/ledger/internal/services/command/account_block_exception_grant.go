// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/spanattr"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// resolveAccountBlockExceptionGrant reads the single-use exception a transaction
// presented and returns it as the grant the rest of the pipeline carries.
//
// A nil exceptionID means the body presented none: the return is (nil, nil) and
// every downstream barrier behaves exactly as it did before the field existed.
//
// An identifier with no live key rejects HERE, before any balance is read or
// staged. Absent, already consumed and expired-by-TTL are one outcome with one
// error (0508): there is no grant to present. Rejecting this early costs the
// caller nothing — the identifier could not have been consumed, because only the
// balance script consumes, and it never runs.
//
// The values read here are advisory: they let the Go pre-validation stop
// fast-failing a blocked account whose grant plausibly matches. The AUTHORITY is
// the balance script, which re-reads the same key inside the atomic step, checks
// it against the transaction's own debit, and deletes it there. So a grant that
// expires between this read and the script is still refused, and one consumed by
// a concurrent transaction in that window is refused too.
func (uc *UseCase) resolveAccountBlockExceptionGrant(ctx context.Context, span trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, exceptionID *uuid.UUID) (*mtransaction.AccountBlockExceptionGrant, error) {
	if exceptionID == nil {
		return nil, nil
	}

	cached, err := uc.TransactionRedisRepo.GetAccountBlockException(ctx, organizationID, ledgerID, *exceptionID)
	if err != nil {
		spanattr.HandleSpanByErrorClass(span, "Failed to read account block exception", err)
		logger.Log(ctx, libLog.LevelError, "Failed to read account block exception", libLog.Err(err))

		return nil, err
	}

	if cached == nil {
		invalid := pkg.ValidateBusinessError(constant.ErrAccountBlockExceptionInvalid, constant.EntityTransaction)

		spanattr.HandleSpanByErrorClass(span, "Account block exception not found", invalid)
		logger.Log(ctx, libLog.LevelWarn, "Account block exception not found", libLog.Err(invalid))

		return nil, invalid
	}

	return &mtransaction.AccountBlockExceptionGrant{
		ID:     *exceptionID,
		Alias:  cached.Alias,
		Amount: cached.Amount,
	}, nil
}
