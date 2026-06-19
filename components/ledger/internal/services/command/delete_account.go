// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libCommons "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// DeleteAccountByID deletes an account from the repository by IDs.
// It first deletes all balances associated with the account via the BalancePort interface.
func (uc *UseCase) DeleteAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, token string) error {
	logger, tracer, requestID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_account_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", id.String()),
	)

	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to find account by id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to find account by id", libLog.Err(err))

		return err
	}

	if accFound != nil && accFound.ID == id.String() && accFound.Type == "external" {
		return pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, constant.EntityAccount)
	}

	if accFound == nil {
		return pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, constant.EntityAccount)
	}

	accountID, err := uuid.Parse(accFound.ID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse account id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to parse account id from repository data", libLog.Err(err))

		return err
	}

	err = uc.DeleteAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, requestID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete all balances by account id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to delete all balances by account id", libLog.Err(err))

		var (
			unauthorized pkg.UnauthorizedError
			forbidden    pkg.ForbiddenError
		)

		if errors.As(err, &unauthorized) || errors.As(err, &forbidden) {
			return err
		}

		return pkg.ValidateBusinessError(constant.ErrAccountBalanceDeletion, constant.EntityAccount)
	}

	if err := uc.AccountRepo.Delete(ctx, organizationID, ledgerID, portfolioID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, constant.EntityAccount)

			logger.Log(ctx, libLog.LevelWarn, "Account ID not found on delete", libLog.String("account_id", id.String()))
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete account on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to delete account on repo by id", err)
		logger.Log(ctx, libLog.LevelError, "Failed to delete account on repo by id", libLog.Err(err))

		return err
	}

	uc.emitAccountDeletedEvent(ctx, span, logger, accFound, time.Now())

	return nil
}

// emitAccountDeletedEvent publishes the account.deleted event for a
// successfully soft-deleted account. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// Durability of the event is owned by PG and (follow-up task) the
// outbox subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked immediately after AccountRepo.Delete succeeds.
// AccountRepo.Delete does not return the post-delete record, so the
// payload sources identity + portfolio scope from the pre-delete record
// (accFound) and stamps deletedAt with the wall-clock instant captured
// by the caller. The PG deleted_at column is set by the same wall clock
// at row-update time, so the values are effectively identical up to
// clock skew.
//
// Wire-format mapping lives in pkg/streaming/events/account_deleted.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitAccountDeletedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, acc *mmodel.Account, deletedAt time.Time) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.AccountDeletedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAccountDeleted(acc, deletedAt).ToEmitRequest(tenantID, deletedAt)
		})
}
