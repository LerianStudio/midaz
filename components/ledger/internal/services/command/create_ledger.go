// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v3"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// CreateLedger creates a new ledger and persists it in the repository.
func (uc *UseCase) CreateLedger(ctx context.Context, organizationID uuid.UUID, cli *mmodel.CreateLedgerInput) (_ *mmodel.Ledger, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_ledger")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "create_ledger", start, err)
	}()

	status := cli.Status
	if status.Code == "" {
		status.Code = "ACTIVE"
	}

	// Validate before creating the ledger. Persist only when the result differs from the
	// defaults, so an all-defaults request leaves the settings column at its '{}' default.
	var settingsToPersist *mmodel.LedgerSettings

	if sparseSettings := cli.Settings.ToSparseMap(); sparseSettings != nil {
		// Group presence only, never the sent keys or their values (T4). A settings
		// validation error names a field path; without these flags that error is
		// indistinguishable from one naming a group the client never sent.
		span.SetAttributes(
			attribute.Bool("app.request.settings.has_accounting", cli.Settings.Accounting != nil),
			attribute.Bool("app.request.settings.has_tracer", cli.Settings.Tracer != nil),
			attribute.Bool("app.request.settings.has_overrides", cli.Settings.Overrides != nil),
		)

		if err := mmodel.ValidateSettings(sparseSettings); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Settings validation failed", err)
			logger.Log(ctx, libLog.LevelWarn, "Settings validation failed", libLog.Err(err))

			return nil, err
		}

		parsed := mmodel.ParseLedgerSettings(sparseSettings)
		if !mmodel.LedgerSettingsIsDefault(&parsed) {
			settingsToPersist = &parsed
		}
	}

	// Bail out before the uniqueness round-trip when the caller is already gone.
	if err = ctx.Err(); err != nil {
		recordCommandError(ctx, span, logger, "Context cancelled before ledger creation", err)

		return nil, err
	}

	// Dual-class: span helper and log level are picked by error class.
	_, err = uc.LedgerRepo.FindByName(ctx, organizationID, cli.Name)
	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to find ledger by name", err)

		return nil, err
	}

	now := time.Now()

	ledger := &mmodel.Ledger{
		OrganizationID: organizationID.String(),
		Name:           cli.Name,
		Status:         status,
		CreatedAt:      now,
		UpdatedAt:      now,
		Settings:       settingsToPersist,
	}

	// Dual-class: span helper and log level are picked by error class.
	led, err := uc.LedgerRepo.Create(ctx, ledger)
	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to create ledger", err)

		return nil, err
	}

	uc.emitLedgerCreatedEvent(ctx, span, logger, led)

	metadata, err := uc.CreateOnboardingMetadata(ctx, constant.EntityLedger, led.ID, cli.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create ledger metadata", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create ledger metadata", libLog.Err(err))

		return nil, err
	}

	led.Metadata = metadata

	// Invalidate settings cache when we persisted settings so GetLedgerSettings sees fresh data.
	if settingsToPersist != nil {
		if ledgerID, parseErr := uuid.Parse(led.ID); parseErr == nil {
			uc.invalidateSettingsCache(ctx, organizationID, ledgerID)
		}
	}

	return led, nil
}

// emitLedgerCreatedEvent publishes the ledger.created event for a
// successfully persisted ledger. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
func (uc *UseCase) emitLedgerCreatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, led *mmodel.Ledger) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.LedgerCreatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewLedgerCreated(led).ToEmitRequest(tenantID, led.CreatedAt)
		})
}
