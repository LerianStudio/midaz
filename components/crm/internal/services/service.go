package services

import (
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
	holderlink "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder-link"
)

// UseCase provides business logic operations for holders and aliases.
type UseCase struct {
	HolderRepo     holder.Repository
	AliasRepo      alias.Repository
	HolderLinkRepo holderlink.Repository
}
