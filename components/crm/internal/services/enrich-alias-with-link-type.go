package services

import (
	"context"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// enrichAliasWithLinkType enriches an alias with linkType from HolderLink
func (uc *UseCase) enrichAliasWithLinkType(ctx context.Context, organizationID string, alias *mmodel.Alias) error {
	if alias.HolderLinkID == nil {
		return nil
	}

	holderLink, err := uc.HolderLinkRepo.Find(ctx, organizationID, *alias.HolderLinkID, false)
	if err != nil {
		return nil
	}

	if holderLink != nil && holderLink.LinkType != nil {
		alias.LinkType = holderLink.LinkType
	}

	return nil
}
