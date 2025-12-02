package services

import (
	"context"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// enrichAliasWithLinkType enriches an alias with linkType from HolderLink
// It fetches all holder links for the alias and populates the HolderLinks array
func (uc *UseCase) enrichAliasWithLinkType(ctx context.Context, organizationID string, alias *mmodel.Alias) error {
	if alias.ID == nil {
		return nil
	}

	holderLinks, err := uc.HolderLinkRepo.FindByAliasID(ctx, organizationID, *alias.ID, false)
	if err != nil {
		return nil
	}

	if len(holderLinks) == 0 {
		return nil
	}

	alias.HolderLinks = holderLinks

	return nil
}
