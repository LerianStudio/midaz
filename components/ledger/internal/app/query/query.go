package query

import (
	m "github.com/LerianStudio/midaz/components/ledger/internal/domain/metadata"
	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	o "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	i "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/instrument"
	p "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/portfolio"
	r "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
)

// UseCase is a struct that aggregates various repositories for simplified access in use case implementations.
type UseCase struct {
	// OrganizationRepo provides an abstraction on top of the organization data source.
	OrganizationRepo o.Repository

	// LedgerRepo provides an abstraction on top of the ledger data source.
	LedgerRepo l.Repository

	// ProductRepo provides an abstraction on top of the product data source.
	ProductRepo r.Repository

	// PortfolioRepo provides an abstraction on top of the portfolio data source.
	PortfolioRepo p.Repository

	// AccountRepo provides an abstraction on top of the account data source.
	AccountRepo a.Repository

	// InstrumentRepo provides an abstraction on top of the instrument data source.
	InstrumentRepo i.Repository

	// MetadataRepo provides an abstraction on top of the metadata data source.
	MetadataRepo m.Repository
}
