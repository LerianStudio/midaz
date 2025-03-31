package balance

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

func TestNewCmdBalanceDescribe(t *testing.T) {
	// Setup
	mockRepo := new(mockBalanceRepo)
	f := &factoryBalanceDescribe{
		repoBalance: mockRepo,
	}

	// Execute
	cmd := newCmdBalanceDescribe(f)

	// Verify
	assert.Equal(t, "describe", cmd.Use)
	assert.Equal(t, "Describes a balance.", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
	assert.NotNil(t, cmd.RunE)

	// Verify flags
	flags := []string{
		"organization-id", "ledger-id", "balance-id", "output", "help",
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
	assert.NotNil(t, result.repoBalance)
	assert.NotNil(t, result.tuiInput)
}

func TestFactoryBalanceDescribeRunE(t *testing.T) {
	tests := []struct {
		name          string
		setupFlags    func(*factoryBalanceDescribe, *cobra.Command)
		setupMocks    func(*mockBalanceRepo)
		expectedError string
	}{
		{
			name: "successfully describes balance with table output",
			setupFlags: func(f *factoryBalanceDescribe, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.BalanceID = "bal123"
				f.OutputFormat = "table"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("balance-id", f.BalanceID)
				cmd.Flags().Set("output", f.OutputFormat)
			},
			setupMocks: func(mockRepo *mockBalanceRepo) {
				now := time.Now()
				metadata := map[string]interface{}{
					"source": "test",
				}
				mockRepo.On("GetByID", "org123", "ledger123", "bal123").Return(
					&mmodel.Balance{
						ID:             "bal123",
						AccountID:      "acc123",
						Amount:         1000,
						AmountScale:    2,
						AssetCode:      "USD",
						OrganizationID: "org123",
						LedgerID:       "ledger123",
						Metadata:       metadata,
						CreatedAt:      now,
						UpdatedAt:      now,
					}, nil,
				)
			},
		},
		{
			name: "successfully describes balance with json output",
			setupFlags: func(f *factoryBalanceDescribe, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.BalanceID = "bal123"
				f.OutputFormat = "json"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("balance-id", f.BalanceID)
				cmd.Flags().Set("output", f.OutputFormat)
			},
			setupMocks: func(mockRepo *mockBalanceRepo) {
				now := time.Now()
				metadata := map[string]interface{}{
					"source": "test",
				}
				mockRepo.On("GetByID", "org123", "ledger123", "bal123").Return(
					&mmodel.Balance{
						ID:             "bal123",
						AccountID:      "acc123",
						Amount:         1000,
						AmountScale:    2,
						AssetCode:      "USD",
						OrganizationID: "org123",
						LedgerID:       "ledger123",
						Metadata:       metadata,
						CreatedAt:      now,
						UpdatedAt:      now,
					}, nil,
				)
			},
		},
		{
			name: "successfully describes balance with deleted_at",
			setupFlags: func(f *factoryBalanceDescribe, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.BalanceID = "bal123"
				f.OutputFormat = "table"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("balance-id", f.BalanceID)
				cmd.Flags().Set("output", f.OutputFormat)
			},
			setupMocks: func(mockRepo *mockBalanceRepo) {
				now := time.Now()
				deletedAt := now.Add(time.Hour)
				metadata := map[string]interface{}{
					"source": "test",
				}
				mockRepo.On("GetByID", "org123", "ledger123", "bal123").Return(
					&mmodel.Balance{
						ID:             "bal123",
						AccountID:      "acc123",
						Amount:         1000,
						AmountScale:    2,
						AssetCode:      "USD",
						OrganizationID: "org123",
						LedgerID:       "ledger123",
						Metadata:       metadata,
						CreatedAt:      now,
						UpdatedAt:      now,
						DeletedAt:      &deletedAt,
					}, nil,
				)
			},
		},
		{
			name: "fails when organization ID is missing and input fails",
			setupFlags: func(f *factoryBalanceDescribe, cmd *cobra.Command) {
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
			setupFlags: func(f *factoryBalanceDescribe, cmd *cobra.Command) {
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
			setupFlags: func(f *factoryBalanceDescribe, cmd *cobra.Command) {
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
			setupFlags: func(f *factoryBalanceDescribe, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.BalanceID = "bal123"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("balance-id", f.BalanceID)
			},
			setupMocks: func(mockRepo *mockBalanceRepo) {
				mockRepo.On("GetByID", "org123", "ledger123", "bal123").Return(
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
			mockRepo := new(mockBalanceRepo)

			f := &factory.Factory{
				IOStreams: ios,
			}

			facDescribe := &factoryBalanceDescribe{
				factory:     f,
				repoBalance: mockRepo,
				tuiInput: func(message string) (string, error) {
					return "default", nil
				},
			}

			cmd := &cobra.Command{}
			cmd.Flags().String("organization-id", "", "")
			cmd.Flags().String("ledger-id", "", "")
			cmd.Flags().String("balance-id", "", "")
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

func TestFactoryBalanceDescribeSetFlags(t *testing.T) {
	// Setup
	f := &factoryBalanceDescribe{}
	cmd := &cobra.Command{}

	// Execute
	f.setFlags(cmd)

	// Verify
	expectedFlags := []string{
		"organization-id", "ledger-id", "balance-id", "output", "help",
	}

	for _, flag := range expectedFlags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}

	// Verify default values
	assert.Equal(t, "table", cmd.Flag("output").DefValue)
}
