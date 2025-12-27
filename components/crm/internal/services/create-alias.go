package services

import (
	"context"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type loggerInterface interface {
	Errorf(format string, args ...any)
}

func (uc *UseCase) CreateAlias(ctx context.Context, organizationID string, holderID uuid.UUID, cai *mmodel.CreateAliasInput) (*mmodel.Alias, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.create_alias")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
	)

	if err := uc.validateLinkTypeIfPresent(ctx, &span, logger, cai.LinkType); err != nil {
		return nil, err
	}

	alias, err := uc.buildAliasFromInput(ctx, &span, logger, organizationID, holderID, cai)
	if err != nil {
		return nil, err
	}

	createdAccount, err := uc.createAliasInRepo(ctx, &span, logger, organizationID, alias)
	if err != nil {
		return nil, err
	}

	if uc.shouldCreateHolderLink(cai.LinkType) {
		return uc.createAliasWithHolderLink(ctx, &span, logger, organizationID, holderID, cai, alias, createdAccount)
	}

	return createdAccount, nil
}

func (uc *UseCase) validateLinkTypeIfPresent(ctx context.Context, span *trace.Span, logger loggerInterface, linkType *string) error {
	if linkType == nil || strings.TrimSpace(*linkType) == "" {
		return nil
	}

	err := uc.ValidateLinkType(ctx, linkType)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to validate link type", err)
		logger.Errorf("Failed to validate link type: %v", err)

		return err
	}

	return nil
}

func (uc *UseCase) buildAliasFromInput(ctx context.Context, span *trace.Span, logger loggerInterface, organizationID string, holderID uuid.UUID, cai *mmodel.CreateAliasInput) (*mmodel.Alias, error) {
	accountID := libCommons.GenerateUUIDv7()

	newAlias := &mmodel.Alias{
		ID:                  &accountID,
		LedgerID:            &cai.LedgerID,
		AccountID:           &cai.AccountID,
		HolderID:            &holderID,
		Metadata:            cai.Metadata,
		ParticipantDocument: cai.ParticipantDocument,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	if cai.BankingDetails != nil {
		newAlias.BankingDetails = &mmodel.BankingDetails{
			Branch:      cai.BankingDetails.Branch,
			Account:     cai.BankingDetails.Account,
			Type:        cai.BankingDetails.Type,
			OpeningDate: cai.BankingDetails.OpeningDate,
			IBAN:        cai.BankingDetails.IBAN,
			CountryCode: cai.BankingDetails.CountryCode,
			BankID:      cai.BankingDetails.BankID,
		}
	}

	holder, err := uc.GetHolderByID(ctx, organizationID, holderID, false)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get holder by id", err)
		logger.Errorf("Failed to get holder by id %v", holderID.String())

		return nil, err
	}

	newAlias.Document = holder.Document
	newAlias.Type = holder.Type

	return newAlias, nil
}

func (uc *UseCase) createAliasInRepo(ctx context.Context, span *trace.Span, logger loggerInterface, organizationID string, alias *mmodel.Alias) (*mmodel.Alias, error) {
	createdAccount, err := uc.AliasRepo.Create(ctx, organizationID, alias)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to create alias", err)
		logger.Errorf("Failed to create alias: %v", err)

		return nil, pkg.ValidateInternalError(err, "Alias")
	}

	return createdAccount, nil
}

func (uc *UseCase) shouldCreateHolderLink(linkType *string) bool {
	return linkType != nil && strings.TrimSpace(*linkType) != ""
}

// createAliasWithHolderLink creates a holder link for the alias and updates the alias with it.
//
// Race Condition Note: A race condition window exists between ValidateHolderLinkConstraints
// and the actual holder link creation. If concurrent requests pass validation simultaneously,
// only one will succeed due to the database unique index on (alias_id, link_type).
// The database unique index is the authoritative source of truth for constraint enforcement.
// The rollback pattern (rollbackAliasCreation) handles cleanup when race conditions cause
// the holder link creation to fail after the alias has already been created.
func (uc *UseCase) createAliasWithHolderLink(ctx context.Context, span *trace.Span, logger loggerInterface, organizationID string, holderID uuid.UUID, cai *mmodel.CreateAliasInput, alias *mmodel.Alias, createdAccount *mmodel.Alias) (*mmodel.Alias, error) {
	// Validate pointers before any dereference - nil pointers in rollback paths
	// cause cryptic panics that mask the original error
	assert.NotNil(createdAccount, "createdAccount must not be nil for holder link creation",
		"organization_id", organizationID,
		"holder_id", holderID.String())
	assert.NotNil(createdAccount.ID, "createdAccount.ID must not be nil for holder link creation",
		"organization_id", organizationID,
		"holder_id", holderID.String())

	if err := uc.validateAndCreateHolderLinkConstraints(ctx, span, logger, organizationID, createdAccount.ID, cai.LinkType); err != nil {
		uc.rollbackAliasCreation(ctx, logger, organizationID, holderID, *createdAccount.ID)
		return nil, err
	}

	createdHolderLink, err := uc.createHolderLink(ctx, span, logger, organizationID, holderID, createdAccount.ID, cai.LinkType)
	if err != nil {
		uc.rollbackAliasCreation(ctx, logger, organizationID, holderID, *createdAccount.ID)

		return nil, err
	}

	updatedAccount, err := uc.updateAliasWithHolderLink(ctx, span, logger, organizationID, holderID, createdAccount, alias, createdHolderLink)
	if err != nil {
		uc.rollbackHolderLinkCreation(ctx, logger, organizationID, *createdHolderLink.ID)
		return nil, err
	}

	uc.enrichAliasWithLinkType(ctx, organizationID, updatedAccount)

	return updatedAccount, nil
}

func (uc *UseCase) validateAndCreateHolderLinkConstraints(ctx context.Context, span *trace.Span, logger loggerInterface, organizationID string, aliasID *uuid.UUID, linkType *string) error {
	err := uc.ValidateHolderLinkConstraints(ctx, organizationID, *aliasID, *linkType)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to validate holder link constraints", err)
		logger.Errorf("Failed to validate holder link constraints: %v", err)

		return err
	}

	return nil
}

func (uc *UseCase) createHolderLink(ctx context.Context, span *trace.Span, logger loggerInterface, organizationID string, holderID uuid.UUID, aliasID *uuid.UUID, linkType *string) (*mmodel.HolderLink, error) {
	holderLinkID := libCommons.GenerateUUIDv7()
	linkTypeStr := *linkType

	holderLink := &mmodel.HolderLink{
		ID:        &holderLinkID,
		HolderID:  &holderID,
		AliasID:   aliasID,
		LinkType:  &linkTypeStr,
		Metadata:  make(map[string]any),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	createdHolderLink, err := uc.HolderLinkRepo.Create(ctx, organizationID, holderLink)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to create holder link", err)
		logger.Errorf("Failed to create holder link: %v", err)

		return nil, pkg.ValidateInternalError(err, "HolderLink")
	}

	return createdHolderLink, nil
}

func (uc *UseCase) updateAliasWithHolderLink(ctx context.Context, span *trace.Span, logger loggerInterface, organizationID string, holderID uuid.UUID, createdAccount *mmodel.Alias, alias *mmodel.Alias, createdHolderLink *mmodel.HolderLink) (*mmodel.Alias, error) {
	alias.HolderLinks = []*mmodel.HolderLink{createdHolderLink}
	alias.UpdatedAt = time.Now()

	updatedAccount, err := uc.AliasRepo.Update(ctx, organizationID, holderID, *createdAccount.ID, alias, nil)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to update alias with holder link", err)
		logger.Errorf("Failed to update alias with holder link: %v", err)

		return nil, pkg.ValidateInternalError(err, "Alias")
	}

	return updatedAccount, nil
}

func (uc *UseCase) rollbackAliasCreation(ctx context.Context, logger loggerInterface, organizationID string, holderID, aliasID uuid.UUID) {
	deleteErr := uc.AliasRepo.Delete(ctx, organizationID, holderID, aliasID, true)
	if deleteErr != nil {
		logger.Errorf("Failed to rollback alias creation after validation error: %v", deleteErr)
	}
}

func (uc *UseCase) rollbackHolderLinkCreation(ctx context.Context, logger loggerInterface, organizationID string, holderLinkID uuid.UUID) {
	deleteErr := uc.HolderLinkRepo.Delete(ctx, organizationID, holderLinkID, true)
	if deleteErr != nil {
		logger.Errorf("Failed to rollback holder link creation after alias update error: %v", deleteErr)
	}
}
