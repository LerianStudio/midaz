// Package main provides examples of creating resources using the Midaz Go SDK.
package main

import (
	"context"
	"fmt"

	midaz "github.com/LerianStudio/midaz/sdks/go-sdk"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// GetBalance demonstrates how to retrieve the balance for an account using the Builder interface.
//
// This function retrieves the current balance for the specified account using the builder pattern.
// It shows how to set required fields like organization ID, ledger ID, and account reference.
//
// Parameters:
//   - ctx: The context for the API request
//   - client: The Midaz client instance
//   - organizationID: The ID of the organization
//   - ledgerID: The ID of the ledger
//   - accountRef: The reference of the account to get the balance for
//
// Returns:
//   - *models.Balance: The account balance
//   - error: An error if the operation fails
func GetBalance(
	ctx context.Context,
	client *midaz.Client,
	organizationID, ledgerID, accountRef string,
) (*models.Balance, error) {
	// Get the balance for the account using the Builder interface
	balance, err := client.Builder.NewBalance().
		WithOrganization(organizationID).
		WithLedger(ledgerID).
		WithAccountReference(accountRef).
		Get(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get balance for account %s: %w", accountRef, err)
	}

	return balance, nil
}
