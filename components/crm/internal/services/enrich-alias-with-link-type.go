package services

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// enrichAliasWithLinkType enriches an alias with linkType from HolderLink
// It fetches all holder links for the alias and populates the HolderLinks array
func (uc *UseCase) enrichAliasWithLinkType(ctx context.Context, organizationID string, alias *mmodel.Alias) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.enrich_alias_with_link_type")
	defer span.End()

	// Precondition: alias.ID must not be nil - indicates programming error
	// If this triggers, the caller passed an uninitialized alias
	assert.NotNil(alias.ID, "alias.ID must not be nil for enrichment",
		"organizationID", organizationID)

	holderLinks, err := uc.HolderLinkRepo.FindByAliasID(ctx, organizationID, *alias.ID, false)
	if err != nil {
		libOpenTelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to fetch holder links for alias enrichment", err)

		logger.Errorf("Failed to fetch holder links for alias enrichment: %v", err)

		return
	}

	if len(holderLinks) == 0 {
		libOpenTelemetry.HandleSpanEvent(&span, "No holder links found for alias enrichment")

		logger.Infof("No holder links found for alias enrichment")

		return
	}

	for _, holderLink := range holderLinks {
		formatedHolderLink := &mmodel.HolderLink{
			ID:        holderLink.ID,
			LinkType:  holderLink.LinkType,
			CreatedAt: holderLink.CreatedAt,
			UpdatedAt: holderLink.UpdatedAt,
			DeletedAt: holderLink.DeletedAt,
		}
		alias.HolderLinks = append(alias.HolderLinks, formatedHolderLink)
	}
}
