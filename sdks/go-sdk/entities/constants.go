// Package entities provides direct access to Midaz API services.
package entities

// Service types for API endpoints.
// These constants define the different service types used to identify
// which API endpoint to use when making requests to the Midaz platform.
const (
	// ServiceOnboarding identifies the onboarding service API.
	// This service handles organization, ledger, account, asset, and portfolio management.
	ServiceOnboarding = "onboarding"

	// ServiceTransaction identifies the transaction service API.
	// This service handles transaction creation, retrieval, and management,
	// as well as operations and balances.
	ServiceTransaction = "transaction"
)
