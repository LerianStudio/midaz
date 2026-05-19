// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// UpdateAccountType updates an account type by its ID.
// It returns the updated account type and an error if the operation fails.
func (uc *UseCase) UpdateAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, input *mmodel.UpdateAccountTypeInput) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_account_type")
	defer span.End()

	accountType := &mmodel.AccountType{
		Name:        input.Name,
		Description: input.Description,
	}

	accountTypeUpdated, err := uc.AccountTypeRepo.Update(ctx, organizationID, ledgerID, id, accountType)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, constant.EntityAccountType)

			logger.Log(ctx, libLog.LevelWarn, "Account type ID not found", libLog.Err(err), libLog.String("account_type_id", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update account type on repo by id", err)

			return nil, err
		}

		logger.Log(ctx, libLog.LevelError, "Failed to update account type on repo by id", libLog.Err(err), libLog.String("account_type_id", id.String()))

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update account type on repo by id", err)

		return nil, err
	}

	uc.emitAccountTypeUpdatedEvent(ctx, span, logger, accountTypeUpdated)

	metadataUpdated, err := uc.UpdateOnboardingMetadata(ctx, constant.EntityAccountType, id.String(), input.Metadata)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to update account type metadata", libLog.Err(err), libLog.String("account_type_id", id.String()))

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata", err)

		return nil, err
	}

	accountTypeUpdated.Metadata = metadataUpdated

	return accountTypeUpdated, nil
}

// emitAccountTypeUpdatedEvent publishes the account-type.updated event
// for a successfully persisted update. IMPORTANT posture: build and
// emit failures are span-recorded and logged at Warn, never returned.
// Durability of the event is owned by PG and (follow-up task) the
// outbox subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked between the AccountTypeRepo.Update success branch and
// the metadata-write call in UpdateAccountType, so a downstream Mongo
// failure cannot mask the event.
//
// Caller invariant: a must be the value returned by AccountTypeRepo.Update
// (post-commit), not the input struct. Specifically a.ID, a.UpdatedAt,
// a.KeyValue and the persisted Name/Description must reflect the row
// state.
//
// Wire-format mapping lives in pkg/streaming/events/account_type_updated.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitAccountTypeUpdatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, a *mmodel.AccountType) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.AccountTypeUpdatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAccountTypeUpdated(a).ToEmitRequest(tenantID, a.UpdatedAt)
		})
}
