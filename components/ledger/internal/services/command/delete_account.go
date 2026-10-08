// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/spanattr"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// DeleteAccountByID soft-deletes an account and the references other stores hold to it.
// The order is: balances, then the CRM instruments linked to the account, then the
// account alias in the ledger's fee and billing packages, then the account row, and
// only then the account.deleted event. Every step before the row is fail-closed: an
// error leaves the row in place and is returned, so the client repeats the DELETE and
// the idempotent steps converge. A nil cascade port skips its step.
func (uc *UseCase) DeleteAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID, token string) (err error) {
	logger, tracer, requestID, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_account_by_id")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "delete_account", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", id.String()),
	)

	// HolderOffV1: the account is read to guard the delete; nothing downstream
	// reads or rewrites its holder, and account.deleted carries no holder field.
	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id, mmodel.HolderOffV1)
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
		spanattr.HandleSpanByErrorClass(span, "Failed to delete all balances by account id", err)

		var (
			unauthorized  pkg.UnauthorizedError
			forbidden     pkg.ForbiddenError
			unprocessable pkg.UnprocessableOperationError
		)

		if errors.As(err, &unauthorized) || errors.As(err, &forbidden) || isAccountClosingIndeterminate(err) ||
			(errors.As(err, &unprocessable) && (unprocessable.Code == constant.ErrBalanceHasOpenFeeDebt.Error() ||
				unprocessable.Code == constant.ErrBalanceOwedFeeDebt.Error())) {
			return err
		}

		return pkg.ValidateBusinessError(constant.ErrAccountBalanceDeletion, constant.EntityAccount)
	}

	if err := uc.cascadeAccountDelete(ctx, span, logger, organizationID, ledgerID, accountID, accFound.Alias); err != nil {
		return err
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

	uc.softDeleteOnboardingMetadata(ctx, span, logger, constant.EntityAccount, id.String())

	uc.emitAccountDeletedEvent(ctx, span, logger, accFound, time.Now())

	return nil
}

// cascadeAccountDelete soft-deletes the account's CRM instruments and then detaches
// its alias from the fee and billing packages. Port errors are technical by contract,
// so they are recorded as span errors and returned unchanged. An account without an
// alias has nothing to detach in fees.
func (uc *UseCase) cascadeAccountDelete(ctx context.Context, span trace.Span, logger libLog.Logger, organizationID, ledgerID, accountID uuid.UUID, alias *string) error {
	if uc.InstrumentCascader != nil {
		cascaded, err := uc.InstrumentCascader.SoftDeleteInstrumentsByAccount(ctx, organizationID, ledgerID, accountID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to cascade account delete to instruments", err)
			logger.Log(ctx, libLog.LevelError, "Failed to cascade account delete to instruments", libLog.Err(err))

			return err
		}

		span.SetAttributes(attribute.Int("app.account_delete.instruments_cascaded", cascaded))
	}

	if uc.FeeAliasDetacher == nil || alias == nil || *alias == "" {
		return nil
	}

	result, err := uc.FeeAliasDetacher.DetachAccountAlias(ctx, organizationID, ledgerID, *alias)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to detach account alias from fee packages", err)
		logger.Log(ctx, libLog.LevelError, "Failed to detach account alias from fee packages", libLog.Err(err))

		return err
	}

	span.SetAttributes(
		attribute.Int("app.account_delete.fee_packages_updated", result.PackagesUpdated),
		attribute.Int("app.account_delete.fee_packages_disabled", result.PackagesDisabled),
	)

	return nil
}

// emitAccountDeletedEvent publishes the account.deleted event for a
// successfully soft-deleted account. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// The persisted database mutation is durable; this helper does not make broker delivery transactional.
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
	pkgStreaming.EmitBrokerBestEffort(ctx, span, logger, uc.Streaming, events.AccountDeletedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAccountDeleted(acc, deletedAt).ToEmitRequest(tenantID, deletedAt)
		})
}
