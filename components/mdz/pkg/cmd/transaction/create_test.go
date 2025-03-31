package transaction

import (
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewCmdTransactionCreate(t *testing.T) {
	// Setup
	mockRepo := new(mockTransactionRepo)
	f := &factoryTransactionCreate{
		repoTransaction: mockRepo,
	}

	// Execute
	cmd := newCmdTransactionCreate(f)

	// Verify
	assert.Equal(t, "create", cmd.Use)
	assert.Equal(t, "Create a transaction", cmd.Short)
	assert.Equal(t, "Create a transaction with the specified parameters", cmd.Long)
	assert.NotNil(t, cmd.RunE)

	// Verify flags
	expectedFlags := []string{
		"organization-id", "ledger-id", "description", "template", "amount",
		"amount-scale", "asset-code", "chart-of-accounts-group", "source",
		"destination", "parent-transaction-id", "status-code", "status-description",
		"metadata", "json-file",
	}
	for _, flag := range expectedFlags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}

func TestNewInjectFacCreate(t *testing.T) {
	// Setup
	ios := iostreams.System()
	f := &factory.Factory{
		IOStreams: ios,
	}

	// Execute
	result := newInjectFacCreate(f)

	// Verify
	assert.NotNil(t, result)
	assert.Equal(t, f, result.factory)
	assert.NotNil(t, result.repoTransaction)
	assert.NotNil(t, result.tuiInput)
}

func TestFactoryTransactionCreateRunE(t *testing.T) {
	tests := []struct {
		name           string
		setupFlags     func(*factoryTransactionCreate, *cobra.Command)
		setupMocks     func(*mockTransactionRepo)
		expectedError  string
	}{
		{
			name: "successfully creates transaction with flags",
			setupFlags: func(f *factoryTransactionCreate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.Description = "Test transaction"
				f.Template = "TRANSFER"
				f.Amount = "1000"
				f.AssetCode = "USD"
				f.ChartOfAccountsGroup = "DEFAULT"
				f.Source = `["acc123"]`
				f.Destination = `["acc124"]`
				
				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("description", f.Description)
				cmd.Flags().Set("template", f.Template)
				cmd.Flags().Set("amount", f.Amount)
				cmd.Flags().Set("asset-code", f.AssetCode)
				cmd.Flags().Set("chart-of-accounts-group", f.ChartOfAccountsGroup)
				cmd.Flags().Set("source", f.Source)
				cmd.Flags().Set("destination", f.Destination)
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				amount := int64(1000)
				
				mockRepo.On("Create", "org123", "ledger123", mock.MatchedBy(func(inp mmodel.CreateTransactionInput) bool {
					return inp.Description == "Test transaction" &&
						inp.Template == "TRANSFER" &&
						*inp.Amount == 1000 &&
						inp.AssetCode == "USD" &&
						inp.ChartOfAccountsGroupName == "DEFAULT" &&
						len(inp.Source) == 1 &&
						inp.Source[0] == "acc123" &&
						len(inp.Destination) == 1 &&
						inp.Destination[0] == "acc124"
				})).Return(
					&mmodel.Transaction{
						ID:          "tx123",
						Description: "Test transaction",
						Template:    "TRANSFER",
						Amount:      &amount,
						AssetCode:   "USD",
						CreatedAt:   time.Now(),
						UpdatedAt:   time.Now(),
					}, nil,
				)
			},
		},
		{
			name: "fails when organization ID is missing and input fails",
			setupFlags: func(f *factoryTransactionCreate, cmd *cobra.Command) {
				f.OrganizationID = ""
				f.tuiInput = func(message string) (string, error) {
					return "", errors.New("input error")
				}
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails when ledger ID is missing and input fails",
			setupFlags: func(f *factoryTransactionCreate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = ""
				f.tuiInput = func(message string) (string, error) {
					if message == "organization-id" {
						return "org123", nil
					}
					return "", errors.New("input error")
				}
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails when description is missing and input fails",
			setupFlags: func(f *factoryTransactionCreate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.Description = ""
				f.tuiInput = func(message string) (string, error) {
					if message == "description" {
						return "", errors.New("input error")
					}
					return "default", nil
				}
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails when invalid amount is provided",
			setupFlags: func(f *factoryTransactionCreate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.Description = "Test transaction"
				f.Template = "TRANSFER"
				f.Amount = "invalid"
				
				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("description", f.Description)
				cmd.Flags().Set("template", f.Template)
				cmd.Flags().Set("amount", f.Amount)
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "invalid syntax",
		},
		{
			name: "fails when invalid source JSON is provided",
			setupFlags: func(f *factoryTransactionCreate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.Description = "Test transaction"
				f.Template = "TRANSFER"
				f.Amount = "1000"
				f.AssetCode = "USD"
				f.ChartOfAccountsGroup = "DEFAULT"
				f.Source = `invalid json`
				
				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("description", f.Description)
				cmd.Flags().Set("template", f.Template)
				cmd.Flags().Set("amount", f.Amount)
				cmd.Flags().Set("asset-code", f.AssetCode)
				cmd.Flags().Set("chart-of-accounts-group", f.ChartOfAccountsGroup)
				cmd.Flags().Set("source", f.Source)
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "source must be a valid JSON array of strings",
		},
		{
			name: "fails when repository call fails",
			setupFlags: func(f *factoryTransactionCreate, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.Description = "Test transaction"
				f.Template = "TRANSFER"
				f.Amount = "1000"
				f.AssetCode = "USD"
				f.ChartOfAccountsGroup = "DEFAULT"
				
				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("description", f.Description)
				cmd.Flags().Set("template", f.Template)
				cmd.Flags().Set("amount", f.Amount)
				cmd.Flags().Set("asset-code", f.AssetCode)
				cmd.Flags().Set("chart-of-accounts-group", f.ChartOfAccountsGroup)
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				mockRepo.On("Create", "org123", "ledger123", mock.Anything).Return(
					nil, errors.New("repository error"),
				)
			},
			expectedError: "repository error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ios := iostreams.System()
			mockRepo := new(mockTransactionRepo)
			
			f := &factory.Factory{
				IOStreams: ios,
			}
			
			facCreate := &factoryTransactionCreate{
				factory:        f,
				repoTransaction: mockRepo,
				tuiInput:       func(message string) (string, error) {
					return "default", nil
				},
			}
			
			cmd := &cobra.Command{}
			cmd.Flags().String("organization-id", "", "")
			cmd.Flags().String("ledger-id", "", "")
			cmd.Flags().String("description", "", "")
			cmd.Flags().String("template", "", "")
			cmd.Flags().String("amount", "", "")
			cmd.Flags().String("amount-scale", "", "")
			cmd.Flags().String("asset-code", "", "")
			cmd.Flags().String("chart-of-accounts-group", "", "")
			cmd.Flags().String("source", "", "")
			cmd.Flags().String("destination", "", "")
			cmd.Flags().String("parent-transaction-id", "", "")
			cmd.Flags().String("status-code", "", "")
			cmd.Flags().String("status-description", "", "")
			cmd.Flags().String("metadata", "", "")
			cmd.Flags().String("json-file", "", "")
			
			// Apply test-specific setup
			tt.setupFlags(facCreate, cmd)
			tt.setupMocks(mockRepo)
			
			// Execute
			err := facCreate.runE(cmd, []string{})
			
			// Verify
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
			
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestFactoryTransactionCreateSetFlags(t *testing.T) {
	// Setup
	f := &factoryTransactionCreate{}
	cmd := &cobra.Command{}
	
	// Execute
	f.setFlags(cmd)
	
	// Verify
	expectedFlags := []string{
		"organization-id", "ledger-id", "description", "template", "amount",
		"amount-scale", "asset-code", "chart-of-accounts-group", "source",
		"destination", "parent-transaction-id", "status-code", "status-description",
		"metadata", "json-file",
	}
	
	for _, flag := range expectedFlags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}
