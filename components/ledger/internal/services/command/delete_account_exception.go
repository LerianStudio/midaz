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
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// DeleteAccountException soft-deletes a live account exception scoped to one account.
//
// A second delete addresses no live row and returns 0503 — there is no state change on
// the retry, so the operation is safe to repeat. On success it emits
// account_exception.deleted, best-effort, after the row is durable.
func (uc *UseCase) DeleteAccountException(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID) (err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_account_exception")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "delete_account_exception", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
		attribute.String("app.request.account_exception_id", id.String()),
	)

	// Invalidate-first: empty the exceptions cache entry BEFORE the Postgres delete.
	// A failure refuses the delete while nothing has changed, so the cache stays
	// consistent with the database (see invalidateAccountExceptionsCache).
	if err = uc.invalidateAccountExceptionsCache(ctx, span, logger, organizationID, ledgerID, accountID); err != nil {
		return err
	}

	deletedAt := time.Now().UTC()

	if err = uc.AccountExceptionRepo.Delete(ctx, organizationID, ledgerID, accountID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountExceptionNotFound, constant.EntityAccountException)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Account exception not found on delete", err)
			logger.Log(ctx, libLog.LevelWarn, "Account exception not found on delete",
				libLog.String("account_exception_id", id.String()))

			return err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to delete account exception", err)
		logger.Log(ctx, libLog.LevelError, "Failed to delete account exception", libLog.Err(err))

		return err
	}

	uc.emitAccountExceptionDeletedEvent(ctx, span, logger, id.String(), organizationID.String(), ledgerID.String(), accountID.String(), deletedAt)

	return nil
}

// emitAccountExceptionDeletedEvent publishes account_exception.deleted for a soft-deleted
// exception. IMPORTANT posture: build/emit failures are span-recorded and logged at Warn
// by the helper, never returned. The use case does not return the persisted row on delete,
// so identity comes from the request-path parameters and deletedAt is the wall-clock
// instant captured by the caller.
//
// Wire-format mapping lives in pkg/streaming/events/account_exception_deleted.go.
func (uc *UseCase) emitAccountExceptionDeletedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, id, organizationID, ledgerID, accountID string, deletedAt time.Time) {
	pkgStreaming.EmitBrokerBestEffort(ctx, span, logger, uc.Streaming, events.AccountExceptionDeletedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAccountExceptionDeleted(id, organizationID, ledgerID, accountID, deletedAt).ToEmitRequest(tenantID, deletedAt)
		})
}
