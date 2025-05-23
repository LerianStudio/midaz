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

func TestNewCmdBalanceListByAccount(t *testing.T) {
	// Setup
	mockRepo := new(mockBalanceRepo)
	f := &factoryBalanceListByAccount{
		repoBalance: mockRepo,
	}

	// Execute
	cmd := newCmdBalanceListByAccount(f)

	// Verify
	assert.Equal(t, "list-by-account", cmd.Use)
	assert.Equal(t, "Lists balances for a specific account.", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
	assert.NotNil(t, cmd.RunE)

	// Verify flags
	flags := []string{
		"organization-id", "ledger-id", "account-id", "limit", "cursor", "sort-order",
		"start-date", "end-date", "help",
	}
	for _, flag := range flags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}

func TestNewInjectFacListByAccount(t *testing.T) {
	// Setup
	ios := iostreams.System()
	f := &factory.Factory{
		IOStreams: ios,
	}

	// Execute
	result := newInjectFacListByAccount(f)

	// Verify
	assert.NotNil(t, result)
	assert.Equal(t, f, result.factory)
	assert.NotNil(t, result.repoBalance)
	assert.NotNil(t, result.tuiInput)
}

func TestFactoryBalanceListByAccountRunE(t *testing.T) {
	tests := []struct {
		name          string
		setupFlags    func(*factoryBalanceListByAccount, *cobra.Command)
		setupMocks    func(*mockBalanceRepo)
		expectedError string
	}{
		{
			name: "successfully lists balances by account",
			setupFlags: func(f *factoryBalanceListByAccount, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AccountID = "acc123"
				f.Limit = 10
				f.Cursor = ""
				f.SortOrder = "desc"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("account-id", f.AccountID)
				cmd.Flags().Set("limit", "10")
				cmd.Flags().Set("cursor", f.Cursor)
				cmd.Flags().Set("sort-order", f.SortOrder)
			},
			setupMocks: func(mockRepo *mockBalanceRepo) {
				now := time.Now()
				mockRepo.On("GetByAccount", "org123", "ledger123", "acc123", 10, "", "desc", "", "").Return(
					&mmodel.Balances{
						Items: []mmodel.Balance{
							{
								ID:        "bal123",
								AccountID: "acc123",
								Available: 1000,
								OnHold:    0, Scale: 2,
								AssetCode:      "USD",
								OrganizationID: "org123",
								LedgerID:       "ledger123",
								CreatedAt:      now,
								UpdatedAt:      now,
							},
							{
								ID:        "bal124",
								AccountID: "acc123",
								Available: 2000,
								OnHold:    0, Scale: 2,
								AssetCode:      "EUR",
								OrganizationID: "org123",
								LedgerID:       "ledger123",
								CreatedAt:      now,
								UpdatedAt:      now,
							},
						},
						Pagination: &mmodel.Pagination{
							Limit: 10,
							Page:  1,
						},
					}, nil,
				)
			},
		},
		{
			name: "successfully handles empty balances list",
			setupFlags: func(f *factoryBalanceListByAccount, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AccountID = "acc123"
				f.Limit = 10
				f.Cursor = ""
				f.SortOrder = "desc"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("account-id", f.AccountID)
				cmd.Flags().Set("limit", "10")
				cmd.Flags().Set("cursor", f.Cursor)
				cmd.Flags().Set("sort-order", f.SortOrder)
			},
			setupMocks: func(mockRepo *mockBalanceRepo) {
				mockRepo.On("GetByAccount", "org123", "ledger123", "acc123", 10, "", "desc", "", "").Return(
					&mmodel.Balances{
						Items: []mmodel.Balance{},
						Pagination: &mmodel.Pagination{
							Limit: 10,
							Page:  1,
						},
					}, nil,
				)
			},
		},
		{
			name: "successfully lists balances with date filters",
			setupFlags: func(f *factoryBalanceListByAccount, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AccountID = "acc123"
				f.Limit = 10
				f.Cursor = ""
				f.SortOrder = "asc"
				f.StartDate = "2023-01-01"
				f.EndDate = "2023-12-31"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("account-id", f.AccountID)
				cmd.Flags().Set("limit", "10")
				cmd.Flags().Set("cursor", f.Cursor)
				cmd.Flags().Set("sort-order", f.SortOrder)
				cmd.Flags().Set("start-date", f.StartDate)
				cmd.Flags().Set("end-date", f.EndDate)
			},
			setupMocks: func(mockRepo *mockBalanceRepo) {
				now := time.Now()
				mockRepo.On("GetByAccount", "org123", "ledger123", "acc123", 10, "", "asc", "2023-01-01", "2023-12-31").Return(
					&mmodel.Balances{
						Items: []mmodel.Balance{
							{
								ID:        "bal123",
								AccountID: "acc123",
								Available: 1000,
								OnHold:    0, Scale: 2,
								AssetCode:      "USD",
								OrganizationID: "org123",
								LedgerID:       "ledger123",
								CreatedAt:      now,
								UpdatedAt:      now,
							},
						},
						Pagination: &mmodel.Pagination{
							Limit: 10,
							Page:  1,
						},
					}, nil,
				)
			},
		},
		{
			name: "fails when organization ID is missing and input fails",
			setupFlags: func(f *factoryBalanceListByAccount, cmd *cobra.Command) {
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
			setupFlags: func(f *factoryBalanceListByAccount, cmd *cobra.Command) {
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
			name: "fails when account ID is missing and input fails",
			setupFlags: func(f *factoryBalanceListByAccount, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AccountID = ""
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
			setupFlags: func(f *factoryBalanceListByAccount, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AccountID = "acc123"
				f.Limit = 10
				f.Cursor = ""
				f.SortOrder = "desc"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("account-id", f.AccountID)
				cmd.Flags().Set("limit", "10")
				cmd.Flags().Set("cursor", f.Cursor)
				cmd.Flags().Set("sort-order", f.SortOrder)
			},
			setupMocks: func(mockRepo *mockBalanceRepo) {
				mockRepo.On("GetByAccount", "org123", "ledger123", "acc123", 10, "", "desc", "", "").Return(
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

			facListByAccount := &factoryBalanceListByAccount{
				factory:     f,
				repoBalance: mockRepo,
				tuiInput: func(message string) (string, error) {
					return "default", nil
				},
			}

			cmd := &cobra.Command{}
			cmd.Flags().String("organization-id", "", "")
			cmd.Flags().String("ledger-id", "", "")
			cmd.Flags().String("account-id", "", "")
			cmd.Flags().Int("limit", 10, "")
			cmd.Flags().String("cursor", "", "")
			cmd.Flags().String("sort-order", "desc", "")
			cmd.Flags().String("start-date", "", "")
			cmd.Flags().String("end-date", "", "")

			// Apply test-specific setup
			tt.setupFlags(facListByAccount, cmd)
			tt.setupMocks(mockRepo)

			// Execute
			err := facListByAccount.runE(cmd, []string{})

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

func TestFactoryBalanceListByAccountSetFlags(t *testing.T) {
	// Setup
	f := &factoryBalanceListByAccount{}
	cmd := &cobra.Command{}

	// Execute
	f.setFlags(cmd)

	// Verify
	expectedFlags := []string{
		"organization-id", "ledger-id", "account-id", "limit", "cursor", "sort-order",
		"start-date", "end-date", "help",
	}

	for _, flag := range expectedFlags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}

	// Verify default values
	assert.Equal(t, "10", cmd.Flag("limit").DefValue)
	assert.Equal(t, "desc", cmd.Flag("sort-order").DefValue)
}
