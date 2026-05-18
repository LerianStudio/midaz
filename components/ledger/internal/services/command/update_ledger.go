// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"

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

// UpdateLedgerByID updates a ledger from the repository.
func (uc *UseCase) UpdateLedgerByID(ctx context.Context, organizationID, id uuid.UUID, uli *mmodel.UpdateLedgerInput) (*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_ledger_by_id")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Trying to update ledger %s", id.String()))

	ledger := &mmodel.Ledger{
		Name:           uli.Name,
		OrganizationID: organizationID.String(),
		Status:         uli.Status,
	}

	ledgerUpdated, err := uc.LedgerRepo.Update(ctx, organizationID, id, ledger)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error updating ledger on repo by id: %v", err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Ledger ID not found: %s", id.String()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update ledger on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update ledger on repo by id", err)

		return nil, err
	}

	// LedgerRepo.Update returns a ledger built from the input via
	// FromEntity which regenerates the ID (intended for Create). Override
	// the identity fields with the function's known-good parameters so the
	// emitted event references the actual row. The HTTP handler avoids this
	// by calling GetLedgerByID afterwards; the streaming emit must do the
	// same defensively.
	ledgerUpdated.ID = id.String()
	ledgerUpdated.OrganizationID = organizationID.String()

	uc.emitLedgerUpdatedEvent(ctx, span, logger, ledgerUpdated)

	metadataUpdated, err := uc.UpdateOnboardingMetadata(ctx, reflect.TypeOf(mmodel.Ledger{}).Name(), id.String(), uli.Metadata)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error updating metadata: %v", err))

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on repo", err)

		return nil, err
	}

	ledgerUpdated.Metadata = metadataUpdated

	return ledgerUpdated, nil
}

// emitLedgerUpdatedEvent publishes the ledger.updated event for a
// successfully persisted update. IMPORTANT posture: build and emit failures
// are span-recorded and logged at Warn, never returned.
func (uc *UseCase) emitLedgerUpdatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, led *mmodel.Ledger) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, uc.StreamingSource, events.LedgerUpdatedDefinition.Key(),
		func(tenantID, source string) (libStreaming.Event, error) {
			return events.NewLedgerUpdated(led).ToEvent(tenantID, source, led.UpdatedAt)
		})
}
