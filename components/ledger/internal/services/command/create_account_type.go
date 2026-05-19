// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// CreateAccountType creates a new account type.
// It returns the created account type and an error if the operation fails.
func (uc *UseCase) CreateAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateAccountTypeInput) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account_type")
	defer span.End()

	now := time.Now()

	accountType := &mmodel.AccountType{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Name:           payload.Name,
		Description:    payload.Description,
		KeyValue:       payload.KeyValue,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	createdAccountType, err := uc.AccountTypeRepo.Create(ctx, organizationID, ledgerID, accountType)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create account type", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create account type", libLog.Err(err))

		return nil, err
	}

	uc.emitAccountTypeCreatedEvent(ctx, span, logger, createdAccountType)

	metadata, err := uc.CreateOnboardingMetadata(ctx, constant.EntityAccountType, createdAccountType.ID.String(), payload.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create metadata", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create account type metadata", libLog.Err(err))

		return nil, err
	}

	createdAccountType.Metadata = metadata

	return createdAccountType, nil
}

// emitAccountTypeCreatedEvent publishes the account-type.created event
// for a successfully persisted account type. IMPORTANT posture: build
// and emit failures are span-recorded and logged at Warn, never
// returned. Durability of the event is owned by PG and (follow-up task)
// the outbox subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked immediately after AccountTypeRepo.Create succeeds and
// before CreateOnboardingMetadata runs, so a downstream Mongo failure
// cannot mask the event.
//
// Wire-format mapping lives in pkg/streaming/events/account_type_created.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitAccountTypeCreatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, a *mmodel.AccountType) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.AccountTypeCreatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAccountTypeCreated(a).ToEmitRequest(tenantID, a.CreatedAt)
		})
}
