// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v3"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// UpdateAccountException applies a partial change to a live account exception.
//
// PATCH semantics: a field absent from the input leaves the stored value unchanged; a
// non-nil BalanceKey pointing at the empty string clears the balance restriction (the
// repository maps that sentinel to SQL NULL). The validity window is validated over the
// FINAL merged state — a PATCH that moves only one bound can invert a window that was
// valid when it was stored, so the merge happens before validation (0505).
func (uc *UseCase) UpdateAccountException(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID, input *mmodel.UpdateAccountExceptionInput) (_ *mmodel.AccountException, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_account_exception")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "update_account_exception", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
		attribute.String("app.request.account_exception_id", id.String()),
	)

	current, err := uc.AccountExceptionRepo.FindByID(ctx, organizationID, ledgerID, accountID, id)
	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to load account exception for update", err)

		return nil, err
	}

	effectiveAt := current.EffectiveAt
	if input.EffectiveAt != nil {
		effectiveAt = input.EffectiveAt
	}

	expiresAt := current.ExpiresAt
	if input.ExpiresAt != nil {
		expiresAt = input.ExpiresAt
	}

	if err = mmodel.ValidateAccountExceptionWindow(effectiveAt, expiresAt); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid merged account exception validity window", err)
		logger.Log(ctx, libLog.LevelWarn, "Invalid merged account exception validity window", libLog.Err(err))

		return nil, err
	}

	patch := buildAccountExceptionPatch(input)

	updated, err := uc.AccountExceptionRepo.Update(ctx, organizationID, ledgerID, accountID, id, patch)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountExceptionNotFound, constant.EntityAccountException)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Account exception not found on update", err)
			logger.Log(ctx, libLog.LevelWarn, "Account exception not found on update",
				libLog.String("account_exception_id", id.String()))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to update account exception", err)
		logger.Log(ctx, libLog.LevelError, "Failed to update account exception", libLog.Err(err))

		return nil, err
	}

	uc.emitAccountExceptionUpdatedEvent(ctx, span, logger, updated)

	return updated, nil
}

// buildAccountExceptionPatch translates the PATCH input into the partial entity the
// repository applies: only the fields the caller sent are populated, and the empty-string
// BalanceKey sentinel is passed through untouched so the repository can clear the column.
func buildAccountExceptionPatch(input *mmodel.UpdateAccountExceptionInput) *mmodel.AccountException {
	patch := &mmodel.AccountException{}

	if input.OperationalTypeCodes != nil {
		patch.OperationalTypeCodes = input.OperationalTypeCodes
	}

	if input.BalanceKey != nil {
		patch.BalanceKey = input.BalanceKey
	}

	if input.Context != nil {
		patch.Context = *input.Context
	}

	if input.EffectiveAt != nil {
		patch.EffectiveAt = input.EffectiveAt
	}

	if input.ExpiresAt != nil {
		patch.ExpiresAt = input.ExpiresAt
	}

	return patch
}

// emitAccountExceptionUpdatedEvent publishes account_exception.updated for a persisted
// update. IMPORTANT posture: build/emit failures are span-recorded and logged at Warn by
// the helper, never returned.
//
// Wire-format mapping lives in pkg/streaming/events/account_exception_updated.go.
func (uc *UseCase) emitAccountExceptionUpdatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, e *mmodel.AccountException) {
	pkgStreaming.EmitBrokerBestEffort(ctx, span, logger, uc.Streaming, events.AccountExceptionUpdatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAccountExceptionUpdated(e).ToEmitRequest(tenantID, e.UpdatedAt)
		})
}
