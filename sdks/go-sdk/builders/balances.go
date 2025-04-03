// Package builders provides fluent builder interfaces for the Midaz SDK.
// It implements the builder pattern to simplify the creation and manipulation
// of Midaz resources through a chainable API.
package builders

import (
	"context"
	"fmt"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// UpdateBalance is an alias for UpdateBalanceInput to maintain compatibility with client interface.
// This type represents the input parameters for updating a balance.
type UpdateBalance = models.UpdateBalanceInput

// BalanceClientInterface defines the minimal client interface required by the balance builders.
// Implementations of this interface are responsible for the actual API communication
// with the Midaz backend services for balance operations.
type BalanceClientInterface interface {
	// UpdateBalance sends a request to update an existing balance with the specified parameters.
	// It requires a context for the API request, organization ID, ledger ID, balance ID,
	// and an input object containing the fields to update.
	// Returns the updated balance or an error if the API request fails.
	UpdateBalance(ctx context.Context, organizationID, ledgerID, balanceID string, input *UpdateBalance) (*models.Balance, error)
}

// BalanceUpdateBuilder defines the builder interface for updating balances.
// A balance represents the current state of an account for a specific asset.
// This builder provides a fluent API for configuring and updating balance resources.
type BalanceUpdateBuilder interface {
	// WithAllowSending sets whether sending is allowed for the balance.
	// When set to true, funds can be transferred out of the account.
	// When set to false, the account becomes a receive-only account.
	// This is an optional field for balance updates.
	WithAllowSending(allowed bool) BalanceUpdateBuilder

	// WithAllowReceiving sets whether receiving is allowed for the balance.
	// When set to true, funds can be transferred into the account.
	// When set to false, the account becomes a send-only account.
	// This is an optional field for balance updates.
	WithAllowReceiving(allowed bool) BalanceUpdateBuilder

	// Update executes the balance update operation and returns the updated balance.
	// It requires a context for the API request.
	// Returns an error if no fields are specified for update or if the API request fails.
	Update(ctx context.Context) (*models.Balance, error)
}

// balanceUpdateBuilder implements the BalanceUpdateBuilder interface.
type balanceUpdateBuilder struct {
	client         BalanceClientInterface
	organizationID string
	ledgerID       string
	balanceID      string
	allowSending   *bool
	allowReceiving *bool
	fieldsToUpdate map[string]bool
}

// NewBalanceUpdate creates a new builder for updating balances.
//
// Balances represent the current state of an account for a specific asset, including
// the amount and permissions for sending and receiving funds. This function returns a builder
// that allows for fluent configuration of balance updates before applying them.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the BalanceClientInterface with the UpdateBalance method.
//   - organizationID: The organization ID that the balance belongs to.
//   - ledgerID: The ledger ID that the balance belongs to.
//   - balanceID: The ID of the balance to update.
//
// Returns:
//   - BalanceUpdateBuilder: A builder interface for configuring and executing balance updates.
//     Use the builder's methods to set the fields to update, then call Update()
//     to perform the balance update operation.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create a balance update builder
//	updateBuilder := builders.NewBalanceUpdate(client, "org-123", "ledger-456", "balance-789")
//
//	// Configure and execute the update
//	updatedBalance, err := updateBuilder.
//	    WithAllowSending(false).     // Disable sending funds
//	    WithAllowReceiving(true).    // Enable receiving funds
//	    Update(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to update balance: %v", err)
//	}
//
//	fmt.Printf("Balance updated: %s (sending allowed: %t, receiving allowed: %t)\n",
//	    updatedBalance.ID, updatedBalance.AllowSending, updatedBalance.AllowReceiving)
func NewBalanceUpdate(client BalanceClientInterface, organizationID, ledgerID, balanceID string) BalanceUpdateBuilder {
	return &balanceUpdateBuilder{
		client:         client,
		organizationID: organizationID,
		ledgerID:       ledgerID,
		balanceID:      balanceID,
		fieldsToUpdate: make(map[string]bool),
	}
}

// WithAllowSending sets whether sending is allowed for the balance.
// When set to true, funds can be transferred out of the account.
// When set to false, the account becomes a receive-only account.
// This is an optional field for balance updates.
func (b *balanceUpdateBuilder) WithAllowSending(allowed bool) BalanceUpdateBuilder {
	b.allowSending = &allowed
	b.fieldsToUpdate["allowSending"] = true

	return b
}

// WithAllowReceiving sets whether receiving is allowed for the balance.
// When set to true, funds can be transferred into the account.
// When set to false, the account becomes a send-only account.
// This is an optional field for balance updates.
func (b *balanceUpdateBuilder) WithAllowReceiving(allowed bool) BalanceUpdateBuilder {
	b.allowReceiving = &allowed
	b.fieldsToUpdate["allowReceiving"] = true

	return b
}

// Update executes the balance update operation and returns the updated balance.
//
// This method validates that at least one field is set for update, constructs the update input,
// and sends the request to the Midaz API to update the balance. Only fields that have been
// explicitly set using the With* methods will be included in the update.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.Balance: The updated balance object if successful.
//     This contains all details about the balance, including its ID, amount,
//     and permissions for sending and receiving funds.
//   - error: An error if the operation fails. Possible error types include:
//   - errors.ErrValidation: If no fields are specified for update
//   - errors.ErrAuthentication: If authentication fails
//   - errors.ErrPermission: If the client lacks permission
//   - errors.ErrNotFound: If the organization, ledger, or balance is not found
//   - errors.ErrInternal: For other internal errors
//
// Example:
//
//	// Update only the sending permission
//	updatedBalance, err := builders.NewBalanceUpdate(client, "org-123", "ledger-456", "balance-789").
//	    WithAllowSending(false).
//	    Update(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Updated balance sending permission: %t\n", updatedBalance.AllowSending)
//
//	// Update both sending and receiving permissions
//	updatedBalance, err = builders.NewBalanceUpdate(client, "org-123", "ledger-456", "balance-789").
//	    WithAllowSending(true).
//	    WithAllowReceiving(false).
//	    Update(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Balance permissions updated - sending: %t, receiving: %t\n",
//	    updatedBalance.AllowSending, updatedBalance.AllowReceiving)
func (b *balanceUpdateBuilder) Update(ctx context.Context) (*models.Balance, error) {
	// Check if any fields are set for update
	if len(b.fieldsToUpdate) == 0 {
		return nil, fmt.Errorf("no fields specified for update")
	}

	// Validate required IDs
	if b.organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if b.ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if b.balanceID == "" {
		return nil, fmt.Errorf("balance ID is required")
	}

	// Create update input
	input := &models.UpdateBalanceInput{}

	// Add fields that are set for update
	if b.fieldsToUpdate["allowSending"] {
		input.AllowSending = b.allowSending
	}

	if b.fieldsToUpdate["allowReceiving"] {
		input.AllowReceiving = b.allowReceiving
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid balance update input: %v", err)
	}

	// Execute balance update
	return b.client.UpdateBalance(ctx, b.organizationID, b.ledgerID, b.balanceID, input)
}
