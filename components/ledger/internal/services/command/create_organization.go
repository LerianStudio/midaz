// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// CreateOrganization creates a new organization and persists it in the repository.
func (uc *UseCase) CreateOrganization(ctx context.Context, coi *mmodel.CreateOrganizationInput) (_ *mmodel.Organization, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_organization")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "create_organization", start, err)
	}()

	status := coi.Status
	if status.Code == "" {
		status.Code = "ACTIVE"
	}

	if libCommons.IsNilOrEmpty(coi.ParentOrganizationID) {
		coi.ParentOrganizationID = nil
	}

	if !coi.Address.IsEmpty() {
		if err := utils.ValidateCountryAddress(coi.Address.Country); err != nil {
			err := pkg.ValidateBusinessError(constant.ErrInvalidCountryCode, constant.EntityOrganization)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate country address", err)

			return nil, err
		}
	}

	organizationID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to generate organization ID", err)

		return nil, err
	}

	now := time.Now()

	organization := &mmodel.Organization{
		ID:                   organizationID.String(),
		ParentOrganizationID: coi.ParentOrganizationID,
		LegalName:            coi.LegalName,
		DoingBusinessAs:      coi.DoingBusinessAs,
		LegalDocument:        coi.LegalDocument,
		Address:              coi.Address,
		Status:               status,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	// Metadata is staged before the insert: a metadata failure leaves nothing in PostgreSQL,
	// and an insert failure discards the staged document.
	metadata, err := uc.CreateOnboardingMetadata(ctx, constant.EntityOrganization, organization.ID, coi.Metadata)
	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to create organization metadata", err, libLog.String("organizationId", organization.ID))

		return nil, err
	}

	org, err := uc.OrganizationRepo.Create(ctx, organization)
	if err != nil {
		uc.discardStagedOnboardingMetadata(ctx, span, logger, constant.EntityOrganization, organization.ID, coi.Metadata)
		recordCommandError(ctx, span, logger, "Failed to create organization on repository", err)

		return nil, err
	}

	uc.emitOrganizationCreatedEvent(ctx, span, logger, org)

	org.Metadata = metadata

	return org, nil
}

// emitOrganizationCreatedEvent publishes the organization.created event for a
// successfully persisted organization. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
func (uc *UseCase) emitOrganizationCreatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, org *mmodel.Organization) {
	pkgStreaming.EmitBrokerBestEffort(ctx, span, logger, uc.Streaming, events.OrganizationCreatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewOrganizationCreated(org).ToEmitRequest(tenantID, org.CreatedAt)
		})
}
