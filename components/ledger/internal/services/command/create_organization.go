// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
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

	now := time.Now()

	organization := &mmodel.Organization{
		ParentOrganizationID: coi.ParentOrganizationID,
		LegalName:            coi.LegalName,
		DoingBusinessAs:      coi.DoingBusinessAs,
		LegalDocument:        coi.LegalDocument,
		Address:              coi.Address,
		Status:               status,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	org, err := uc.OrganizationRepo.Create(ctx, organization)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create organization on repository", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create organization", libLog.Err(err))

		return nil, err
	}

	uc.emitOrganizationCreatedEvent(ctx, span, logger, org)

	uc.provisionSelfHolder(ctx, span, logger, org)

	// NOTE: The organization is already persisted at this point. If metadata creation
	// fails, the org exists in PostgreSQL without its metadata in MongoDB. This is a
	// known consistency gap that affects all entity creates. A proper fix requires
	// either a cross-store transaction or an async metadata creation with retries.
	metadata, err := uc.CreateOnboardingMetadata(ctx, constant.EntityOrganization, org.ID, coi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create organization metadata", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create organization metadata, organization persisted without metadata",
			libLog.Err(err), libLog.String("organizationId", org.ID))

		return nil, err
	}

	org.Metadata = metadata

	return org, nil
}

// emitOrganizationCreatedEvent publishes the organization.created event for a
// successfully persisted organization. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
func (uc *UseCase) emitOrganizationCreatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, org *mmodel.Organization) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.OrganizationCreatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewOrganizationCreated(org).ToEmitRequest(tenantID, org.CreatedAt)
		})
}

// provisionSelfHolder eagerly creates the organization's deterministic self-holder
// (a LEGAL_PERSON holder whose ID is derived from the org ID via UUIDv5). It runs
// after the PG commit and is non-fatal: there is no cross-store transaction, so a
// Mongo failure is span-recorded, logged at Warn, and swallowed. The idempotent
// backfill runner is the repair path for any miss.
func (uc *UseCase) provisionSelfHolder(ctx context.Context, span trace.Span, logger libLog.Logger, org *mmodel.Organization) {
	if uc.HolderProvisioner == nil {
		return
	}

	organizationID, err := uuid.Parse(org.ID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to parse organization ID for self-holder", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to parse organization ID for self-holder provisioning", libLog.Err(err))

		return
	}

	holderType := "LEGAL_PERSON"
	input := &mmodel.CreateHolderInput{
		Type:     &holderType,
		Name:     org.LegalName,
		Document: org.LegalDocument,
	}

	if _, err := uc.HolderProvisioner.CreateHolderWithID(ctx, org.ID, deriveSelfHolderID(organizationID), input); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to provision organization self-holder", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to provision organization self-holder", libLog.Err(err))
	}
}
