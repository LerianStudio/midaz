package services

import (
	"context"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// enrichAliasWithLinkType enriches an alias with linkType from HolderLink
// It fetches all holder links for the alias and populates the HolderLinks array
func (uc *UseCase) enrichAliasWithLinkType(ctx context.Context, organizationID string, alias *mmodel.Alias) {
	if alias.ID == nil {
		return
	}

	holderLinks, _ := uc.HolderLinkRepo.FindByAliasID(ctx, organizationID, *alias.ID, false)

	if len(holderLinks) == 0 {
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
