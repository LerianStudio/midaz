package services

import (
	"context"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) CreateAlias(ctx context.Context, organizationID string, holderID uuid.UUID, cai *mmodel.CreateAliasInput) (*mmodel.Alias, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.create_alias")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
	)

	if cai.LinkType != nil && strings.TrimSpace(*cai.LinkType) != "" {
		err := uc.ValidateLinkType(ctx, cai.LinkType)
		if err != nil {
			libOpenTelemetry.HandleSpanError(&span, "Failed to validate link type", err)
			logger.Errorf("Failed to validate link type: %v", err)

			return nil, err
		}
	}

	accountID := libCommons.GenerateUUIDv7()

	alias := &mmodel.Alias{
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
		alias.BankingDetails = &mmodel.BankingDetails{
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
		libOpenTelemetry.HandleSpanError(&span, "Failed to get holder by id", err)

		logger.Errorf("Failed to get holder by id %v", holderID.String())

		return nil, err
	}

	alias.Document = holder.Document
	alias.Type = holder.Type

	createdAccount, err := uc.AliasRepo.Create(ctx, organizationID, alias)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to create alias", err)

		logger.Errorf("Failed to create alias: %v", err)

		return nil, err
	}

	if cai.LinkType != nil && strings.TrimSpace(*cai.LinkType) != "" {
		err = uc.ValidateHolderLinkConstraints(ctx, organizationID, *createdAccount.ID, *cai.LinkType)
		if err != nil {
			libOpenTelemetry.HandleSpanError(&span, "Failed to validate holder link constraints", err)

			logger.Errorf("Failed to validate holder link constraints: %v", err)

			deleteErr := uc.AliasRepo.Delete(ctx, organizationID, holderID, *createdAccount.ID, true)
			if deleteErr != nil {
				logger.Errorf("Failed to rollback alias creation after validation error: %v", deleteErr)
			}

			return nil, err
		}

		holderLinkID := libCommons.GenerateUUIDv7()
		linkTypeStr := *cai.LinkType

		holderLink := &mmodel.HolderLink{
			ID:        &holderLinkID,
			HolderID:  &holderID,
			AliasID:   createdAccount.ID,
			LinkType:  &linkTypeStr,
			Metadata:  make(map[string]any),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		createdHolderLink, err := uc.HolderLinkRepo.Create(ctx, organizationID, holderLink)
		if err != nil {
			libOpenTelemetry.HandleSpanError(&span, "Failed to create holder link", err)

			logger.Errorf("Failed to create holder link: %v", err)

			return nil, err
		}

		alias.HolderLinks = []*mmodel.HolderLink{createdHolderLink}
		alias.UpdatedAt = time.Now()

		updatedAccount, err := uc.AliasRepo.Update(ctx, organizationID, holderID, *createdAccount.ID, alias, nil)
		if err != nil {
			libOpenTelemetry.HandleSpanError(&span, "Failed to update alias with holder link", err)

			logger.Errorf("Failed to update alias with holder link: %v", err)

			deleteErr := uc.HolderLinkRepo.Delete(ctx, organizationID, *createdHolderLink.ID, true)
			if deleteErr != nil {
				logger.Errorf("Failed to rollback holder link creation after alias update error: %v", deleteErr)
			}

			return nil, err
		}

		err = uc.enrichAliasWithLinkType(ctx, organizationID, updatedAccount)
		if err != nil {
			logger.Warnf("Failed to enrich alias with holder links: %v", err)
		}

		return updatedAccount, nil
	}

	return createdAccount, nil
}
