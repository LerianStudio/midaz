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

func TestNewCmdTransactionRevert(t *testing.T) {
	// Setup
	mockRepo := new(mockTransactionRepo)
	f := &factoryTransactionRevert{
		repoTransaction: mockRepo,
	}

	// Execute
	cmd := newCmdTransactionRevert(f)

	// Verify
	assert.Equal(t, "revert", cmd.Use)
	assert.Equal(t, "Reverts a transaction.", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
	assert.NotNil(t, cmd.RunE)

	// Verify flags
	expectedFlags := []string{
		"organization-id", "ledger-id", "transaction-id", "help",
	}
	for _, flag := range expectedFlags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}

func TestNewInjectFacRevert(t *testing.T) {
	// Setup
	ios := iostreams.System()
	f := &factory.Factory{
		IOStreams: ios,
	}

	// Execute
	result := newInjectFacRevert(f)

	// Verify
	assert.NotNil(t, result)
	assert.Equal(t, f, result.factory)
	assert.NotNil(t, result.repoTransaction)
	assert.NotNil(t, result.tuiInput)
}

func TestFactoryTransactionRevertRunE(t *testing.T) {
	tests := []struct {
		name           string
		setupFlags     func(*factoryTransactionRevert, *cobra.Command)
		setupMocks     func(*mockTransactionRepo)
		expectedError  string
	}{
		{
			name: "successfully reverts transaction",
			setupFlags: func(f *factoryTransactionRevert, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.TransactionID = "tx123"
				
				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("transaction-id", f.TransactionID)
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				now := time.Now()
				amount := int64(1000)
				
				mockRepo.On("Revert", "org123", "ledger123", "tx123").Return(
					&mmodel.Transaction{
						ID:          "tx124",
						Description: "Reversal of transaction tx123",
						Template:    "REVERSAL",
						Amount:      &amount,
						AssetCode:   "USD",
						CreatedAt:   now,
						UpdatedAt:   now,
					}, nil,
				)
			},
		},
		{
			name: "fails when organization ID is missing and input fails",
			setupFlags: func(f *factoryTransactionRevert, cmd *cobra.Command) {
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
			setupFlags: func(f *factoryTransactionRevert, cmd *cobra.Command) {
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
			setupFlags: func(f *factoryTransactionRevert, cmd *cobra.Command) {
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
			setupFlags: func(f *factoryTransactionRevert, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.TransactionID = "tx123"
				
				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("transaction-id", f.TransactionID)
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				mockRepo.On("Revert", "org123", "ledger123", "tx123").Return(
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
			
			facRevert := &factoryTransactionRevert{
				factory:        f,
				repoTransaction: mockRepo,
				tuiInput:       func(message string) (string, error) {
					return "default", nil
				},
			}
			
			cmd := &cobra.Command{}
			cmd.Flags().String("organization-id", "", "")
			cmd.Flags().String("ledger-id", "", "")
			cmd.Flags().String("transaction-id", "", "")
			
			// Apply test-specific setup
			tt.setupFlags(facRevert, cmd)
			tt.setupMocks(mockRepo)
			
			// Execute
			err := facRevert.runE(cmd, []string{})
			
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

func TestFactoryTransactionRevertSetFlags(t *testing.T) {
	// Setup
	f := &factoryTransactionRevert{}
	cmd := &cobra.Command{}
	
	// Execute
	f.setFlags(cmd)
	
	// Verify
	expectedFlags := []string{
		"organization-id", "ledger-id", "transaction-id", "help",
	}
	
	for _, flag := range expectedFlags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}
