package rest

import (
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
)

// Factory defines an interface for REST client factories
type Factory interface {
	Account() *account
	Asset() *asset
	AssetRate() *assetRate
	Auth() *Auth
	Balance() *balance
	Ledger() *ledger
	Operation() *operation
	Organization() *organization
	Portfolio() *portfolio
	Segment() *segment
	Transaction() *transaction
}

type factoryImpl struct {
	factory *factory.Factory
}

// NewFactory creates a new REST factory with the given factory
func NewFactory(f *factory.Factory) Factory {
	return &factoryImpl{
		factory: f,
	}
}

func (f *factoryImpl) Account() *account {
	return NewAccount(f.factory)
}

func (f *factoryImpl) Asset() *asset {
	return NewAsset(f.factory)
}

func (f *factoryImpl) AssetRate() *assetRate {
	return NewAssetRate(f.factory)
}

func (f *factoryImpl) Auth() *Auth {
	return NewAuth(f.factory)
}

func (f *factoryImpl) Balance() *balance {
	return NewBalance(f.factory)
}

func (f *factoryImpl) Ledger() *ledger {
	return NewLedger(f.factory)
}

func (f *factoryImpl) Operation() *operation {
	return NewOperation(f.factory)
}

func (f *factoryImpl) Organization() *organization {
	return NewOrganization(f.factory)
}

func (f *factoryImpl) Portfolio() *portfolio {
	return NewPortfolio(f.factory)
}

func (f *factoryImpl) Segment() *segment {
	return NewSegment(f.factory)
}

func (f *factoryImpl) Transaction() *transaction {
	return NewTransaction(f.factory)
}