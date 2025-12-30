package services

import (
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
)

type UseCase struct {
	HolderRepo holder.Repository
	AliasRepo  alias.Repository
}
