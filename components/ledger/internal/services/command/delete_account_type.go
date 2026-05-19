// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// DeleteAccountTypeByID deletes an account type by its ID.
// It returns an error if the operation fails or if the account type is not found.
func (uc *UseCase) DeleteAccountTypeByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_account_type_by_id")
	defer span.End()

	if err := uc.AccountTypeRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, constant.EntityAccountType)

			logger.Log(ctx, libLog.LevelWarn, "Account type ID not found", libLog.Err(err), libLog.String("account_type_id", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete account type on repo", err)

			return err
		}

		logger.Log(ctx, libLog.LevelError, "Failed to delete account type", libLog.Err(err), libLog.String("account_type_id", id.String()))

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete account type on repo", err)

		return err
	}

	uc.emitAccountTypeDeletedEvent(ctx, span, logger, id.String(), organizationID.String(), ledgerID.String(), time.Now())

	return nil
}

// emitAccountTypeDeletedEvent publishes the account-type.deleted event
// for a successfully soft-deleted account type. IMPORTANT posture:
// build and emit failures are span-recorded and logged at Warn, never
// returned. Durability of the event is owned by PG and (follow-up task)
// the outbox subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked immediately after AccountTypeRepo.Delete succeeds.
// AccountTypeRepo.Delete does not return the post-delete record, so the
// payload sources identity from the use-case parameters (which match
// the request path) and stamps deletedAt with the wall-clock instant
// captured by the caller. The PG deleted_at column is set by the same
// wall clock at row-update time, so the values are effectively identical
// up to clock skew.
//
// Wire-format mapping lives in pkg/streaming/events/account_type_deleted.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitAccountTypeDeletedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, id, organizationID, ledgerID string, deletedAt time.Time) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.AccountTypeDeletedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAccountTypeDeleted(id, organizationID, ledgerID, deletedAt).ToEmitRequest(tenantID, deletedAt)
		})
}
