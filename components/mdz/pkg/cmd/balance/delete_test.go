package balance

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdBalanceDelete(t *testing.T) {
	// Setup
	mockRepo := new(mockBalanceRepo)
	f := &factoryBalanceDelete{
		repoBalance: mockRepo,
	}

	// Execute
	cmd := newCmdBalanceDelete(f)

	// Verify
	assert.Equal(t, "delete", cmd.Use)
	assert.Equal(t, "Deletes a balance.", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
	assert.NotNil(t, cmd.RunE)

	// Verify flags
	flags := []string{
		"organization-id", "ledger-id", "balance-id", "help",
	}
	for _, flag := range flags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}

func TestNewInjectFacDelete(t *testing.T) {
	// Setup
	ios := iostreams.System()
	f := &factory.Factory{
		IOStreams: ios,
	}

	// Execute
	result := newInjectFacDelete(f)

	// Verify
	assert.NotNil(t, result)
	assert.Equal(t, f, result.factory)
	assert.NotNil(t, result.repoBalance)
	assert.NotNil(t, result.tuiInput)
}

func TestFactoryBalanceDeleteRunE(t *testing.T) {
	tests := []struct {
		name           string
		setupFlags     func(*factoryBalanceDelete, *cobra.Command)
		setupMocks     func(*mockBalanceRepo)
		expectedError  string
	}{
		{
			name: "successfully deletes balance",
			setupFlags: func(f *factoryBalanceDelete, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.BalanceID = "bal123"
				
				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("balance-id", f.BalanceID)
			},
			setupMocks: func(mockRepo *mockBalanceRepo) {
				mockRepo.On("Delete", "org123", "ledger123", "bal123").Return(nil)
			},
		},
		{
			name: "fails when organization ID is missing and input fails",
			setupFlags: func(f *factoryBalanceDelete, cmd *cobra.Command) {
				f.OrganizationID = ""
				f.tuiInput = func(message string) (string, error) {
					return "", errors.New("input error")
				}
			},
			setupMocks: func(mockRepo *mockBalanceRepo) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails when ledger ID is missing and input fails",
			setupFlags: func(f *factoryBalanceDelete, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = ""
				f.tuiInput = func(message string) (string, error) {
					if message == "Enter your organization-id" {
						return "org123", nil
					}
					return "", errors.New("input error")
				}
			},
			setupMocks: func(mockRepo *mockBalanceRepo) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails when balance ID is missing and input fails",
			setupFlags: func(f *factoryBalanceDelete, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.BalanceID = ""
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
			setupMocks: func(mockRepo *mockBalanceRepo) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails when repository call fails",
			setupFlags: func(f *factoryBalanceDelete, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.BalanceID = "bal123"
				
				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("balance-id", f.BalanceID)
			},
			setupMocks: func(mockRepo *mockBalanceRepo) {
				mockRepo.On("Delete", "org123", "ledger123", "bal123").Return(errors.New("repository error"))
			},
			expectedError: "repository error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ios := iostreams.System()
			mockRepo := new(mockBalanceRepo)
			
			f := &factory.Factory{
				IOStreams: ios,
			}
			
			facDelete := &factoryBalanceDelete{
				factory:      f,
				repoBalance: mockRepo,
				tuiInput:     func(message string) (string, error) {
					return "default", nil
				},
			}
			
			cmd := &cobra.Command{}
			cmd.Flags().String("organization-id", "", "")
			cmd.Flags().String("ledger-id", "", "")
			cmd.Flags().String("balance-id", "", "")
			
			// Apply test-specific setup
			tt.setupFlags(facDelete, cmd)
			tt.setupMocks(mockRepo)
			
			// Execute
			err := facDelete.runE(cmd, []string{})
			
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

func TestFactoryBalanceDeleteSetFlags(t *testing.T) {
	// Setup
	f := &factoryBalanceDelete{}
	cmd := &cobra.Command{}
	
	// Execute
	f.setFlags(cmd)
	
	// Verify
	expectedFlags := []string{
		"organization-id", "ledger-id", "balance-id", "help",
	}
	
	for _, flag := range expectedFlags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}
