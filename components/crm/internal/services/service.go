package services

import (
	"github.com/LerianStudio/midaz/v4/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v4/components/crm/internal/adapters/mongodb/holder"
)

type UseCase struct {
	HolderRepo holder.Repository
	AliasRepo  alias.Repository
}
