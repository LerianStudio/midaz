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
)

func TestNewCmdTransactionDescribe(t *testing.T) {
	// Setup
	mockRepo := new(mockTransactionRepo)
	f := &factoryTransactionDescribe{
		repoTransaction: mockRepo,
	}

	// Execute
	cmd := newCmdTransactionDescribe(f)

	// Verify
	assert.Equal(t, "describe", cmd.Use)
	assert.Equal(t, "Describes a transaction.", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
	assert.NotNil(t, cmd.RunE)

	// Verify flags
	flags := []string{
		"organization-id", "ledger-id", "transaction-id", "output", "help",
	}
	for _, flag := range flags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}

func TestNewInjectFacDescribe(t *testing.T) {
	// Setup
	ios := iostreams.System()
	f := &factory.Factory{
		IOStreams: ios,
	}

	// Execute
	result := newInjectFacDescribe(f)

	// Verify
	assert.NotNil(t, result)
	assert.Equal(t, f, result.factory)
	assert.NotNil(t, result.repoTransaction)
	assert.NotNil(t, result.tuiInput)
}

func TestFactoryTransactionDescribeRunE(t *testing.T) {
	tests := []struct {
		name          string
		setupFlags    func(*factoryTransactionDescribe, *cobra.Command)
		setupMocks    func(*mockTransactionRepo)
		expectedError string
	}{
		{
			name: "successfully describes transaction with table output",
			setupFlags: func(f *factoryTransactionDescribe, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.TransactionID = "tx123"
				f.OutputFormat = "table"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("transaction-id", f.TransactionID)
				cmd.Flags().Set("output", f.OutputFormat)
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				statusDesc := "Completed"
				parentTxID := "parent123"
				amount := int64(1000)
				amountScale := int64(0)

				metadata := map[string]interface{}{
					"source": "test",
				}

				status := &mmodel.Status{
					Code:        "COMPLETED",
					Description: &statusDesc,
				}

				mockRepo.On("GetByID", "org123", "ledger123", "tx123").Return(
					&mmodel.Transaction{
						ID:                       "tx123",
						Description:              "Test transaction",
						Template:                 "TRANSFER",
						Amount:                   &amount,
						AmountScale:              &amountScale,
						AssetCode:                "USD",
						ChartOfAccountsGroupName: "DEFAULT",
						ParentTransactionID:      &parentTxID,
						Status:                   status,
						Source: []string{
							"acc123",
						},
						Destination: []string{
							"acc124",
						},
						Metadata:  metadata,
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
						Operations: []mmodel.Operation{
							{
								ID:        "op123",
								AccountID: "acc123",
								Type:      "DEBIT",
								Amount:    1000,
								AssetCode: "USD",
								CreatedAt: time.Now(),
							},
							{
								ID:        "op124",
								AccountID: "acc124",
								Type:      "CREDIT",
								Amount:    1000,
								AssetCode: "USD",
								CreatedAt: time.Now(),
							},
						},
					}, nil,
				)
			},
		},
		{
			name: "successfully describes transaction with json output",
			setupFlags: func(f *factoryTransactionDescribe, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.TransactionID = "tx123"
				f.OutputFormat = "json"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("transaction-id", f.TransactionID)
				cmd.Flags().Set("output", f.OutputFormat)
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				amount := int64(1000)
				amountScale := int64(0)

				metadata := map[string]interface{}{
					"source": "test",
				}

				mockRepo.On("GetByID", "org123", "ledger123", "tx123").Return(
					&mmodel.Transaction{
						ID:                       "tx123",
						Description:              "Test transaction",
						Template:                 "TRANSFER",
						Amount:                   &amount,
						AmountScale:              &amountScale,
						AssetCode:                "USD",
						ChartOfAccountsGroupName: "DEFAULT",
						Metadata:                 metadata,
						CreatedAt:                time.Now(),
						UpdatedAt:                time.Now(),
					}, nil,
				)
			},
		},
		{
			name: "fails when organization ID is missing and input fails",
			setupFlags: func(f *factoryTransactionDescribe, cmd *cobra.Command) {
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
			setupFlags: func(f *factoryTransactionDescribe, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = ""
				f.tuiInput = func(message string) (string, error) {
					if message == "Enter your organization-id" {
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
			name: "fails when transaction ID is missing and input fails",
			setupFlags: func(f *factoryTransactionDescribe, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.TransactionID = ""
				f.tuiInput = func(message string) (string, error) {
					if message == "Enter your organization-id" {
						return "org123", nil
					}
					if message == "Enter your ledger-id" {
						return "ledger123", nil
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
			name: "fails when repository call fails",
			setupFlags: func(f *factoryTransactionDescribe, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.TransactionID = "tx123"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("transaction-id", f.TransactionID)
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				mockRepo.On("GetByID", "org123", "ledger123", "tx123").Return(
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

			facDescribe := &factoryTransactionDescribe{
				factory:         f,
				repoTransaction: mockRepo,
				tuiInput: func(message string) (string, error) {
					return "default", nil
				},
			}

			cmd := &cobra.Command{}
			cmd.Flags().String("organization-id", "", "")
			cmd.Flags().String("ledger-id", "", "")
			cmd.Flags().String("transaction-id", "", "")
			cmd.Flags().String("output", "table", "")

			// Apply test-specific setup
			tt.setupFlags(facDescribe, cmd)
			tt.setupMocks(mockRepo)

			// Execute
			err := facDescribe.runE(cmd, []string{})

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

func TestFactoryTransactionDescribeSetFlags(t *testing.T) {
	// Setup
	f := &factoryTransactionDescribe{}
	cmd := &cobra.Command{}

	// Execute
	f.setFlags(cmd)

	// Verify
	expectedFlags := []string{
		"organization-id", "ledger-id", "transaction-id", "output", "help",
	}

	for _, flag := range expectedFlags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}

	// Verify default values
	assert.Equal(t, "table", cmd.Flag("output").DefValue)
}
