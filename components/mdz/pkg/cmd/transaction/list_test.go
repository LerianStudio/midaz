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

func TestNewCmdTransactionList(t *testing.T) {
	// Setup
	mockRepo := new(mockTransactionRepo)
	f := &factoryTransactionList{
		repoTransaction: mockRepo,
	}

	// Execute
	cmd := newCmdTransactionList(f)

	// Verify
	assert.Equal(t, "list", cmd.Use)
	assert.Equal(t, "Lists transactions.", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
	assert.NotNil(t, cmd.RunE)

	// Verify flags
	flags := []string{
		"organization-id", "ledger-id", "limit", "page", "sort-order", 
		"start-date", "end-date", "help",
	}
	for _, flag := range flags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}

func TestNewInjectFacList(t *testing.T) {
	// Setup
	ios := iostreams.System()
	f := &factory.Factory{
		IOStreams: ios,
	}

	// Execute
	result := newInjectFacList(f)

	// Verify
	assert.NotNil(t, result)
	assert.Equal(t, f, result.factory)
	assert.NotNil(t, result.repoTransaction)
	assert.NotNil(t, result.tuiInput)
}

func TestFactoryTransactionListRunE(t *testing.T) {
	tests := []struct {
		name           string
		setupFlags     func(*factoryTransactionList, *cobra.Command)
		setupMocks     func(*mockTransactionRepo)
		expectedError  string
	}{
		{
			name: "successfully lists transactions",
			setupFlags: func(f *factoryTransactionList, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.Limit = 10
				f.Page = 1
				f.SortOrder = "desc"
				
				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("limit", "10")
				cmd.Flags().Set("page", "1")
				cmd.Flags().Set("sort-order", f.SortOrder)
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				now := time.Now()
				amount1 := int64(1000)
				amount2 := int64(2000)
				
				mockRepo.On("Get", "org123", "ledger123", 10, 1, "desc", "", "").Return(
					&mmodel.Transactions{
						Items: []mmodel.Transaction{
							{
								ID:          "tx123",
								Description: "Transaction 1",
								Template:    "TRANSFER",
								Amount:      &amount1,
								AssetCode:   "USD",
								CreatedAt:   now,
								UpdatedAt:   now,
							},
							{
								ID:          "tx124",
								Description: "Transaction 2",
								Template:    "PAYMENT",
								Amount:      &amount2,
								AssetCode:   "EUR",
								CreatedAt:   now,
								UpdatedAt:   now,
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
			name: "successfully handles empty transactions list",
			setupFlags: func(f *factoryTransactionList, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.Limit = 10
				f.Page = 1
				f.SortOrder = "desc"
				
				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("limit", "10")
				cmd.Flags().Set("page", "1")
				cmd.Flags().Set("sort-order", f.SortOrder)
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				mockRepo.On("Get", "org123", "ledger123", 10, 1, "desc", "", "").Return(
					&mmodel.Transactions{
						Items:      []mmodel.Transaction{},
						Pagination: &mmodel.Pagination{
							Limit: 10,
							Page:  1,
						},
					}, nil,
				)
			},
		},
		{
			name: "successfully lists transactions with date filters",
			setupFlags: func(f *factoryTransactionList, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.Limit = 10
				f.Page = 1
				f.SortOrder = "asc"
				f.StartDate = "2023-01-01"
				f.EndDate = "2023-12-31"
				
				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("limit", "10")
				cmd.Flags().Set("page", "1")
				cmd.Flags().Set("sort-order", f.SortOrder)
				cmd.Flags().Set("start-date", f.StartDate)
				cmd.Flags().Set("end-date", f.EndDate)
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				now := time.Now()
				amount := int64(1000)
				
				mockRepo.On("Get", "org123", "ledger123", 10, 1, "asc", "2023-01-01", "2023-12-31").Return(
					&mmodel.Transactions{
						Items: []mmodel.Transaction{
							{
								ID:          "tx123",
								Description: "Transaction 1",
								Template:    "TRANSFER",
								Amount:      &amount,
								AssetCode:   "USD",
								CreatedAt:   now,
								UpdatedAt:   now,
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
			setupFlags: func(f *factoryTransactionList, cmd *cobra.Command) {
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
			setupFlags: func(f *factoryTransactionList, cmd *cobra.Command) {
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
			name: "fails when repository call fails",
			setupFlags: func(f *factoryTransactionList, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.Limit = 10
				f.Page = 1
				f.SortOrder = "desc"
				
				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("limit", "10")
				cmd.Flags().Set("page", "1")
				cmd.Flags().Set("sort-order", f.SortOrder)
			},
			setupMocks: func(mockRepo *mockTransactionRepo) {
				mockRepo.On("Get", "org123", "ledger123", 10, 1, "desc", "", "").Return(
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
			
			facList := &factoryTransactionList{
				factory:        f,
				repoTransaction: mockRepo,
				tuiInput:       func(message string) (string, error) {
					return "default", nil
				},
			}
			
			cmd := &cobra.Command{}
			cmd.Flags().String("organization-id", "", "")
			cmd.Flags().String("ledger-id", "", "")
			cmd.Flags().Int("limit", 10, "")
			cmd.Flags().Int("page", 1, "")
			cmd.Flags().String("sort-order", "desc", "")
			cmd.Flags().String("start-date", "", "")
			cmd.Flags().String("end-date", "", "")
			
			// Apply test-specific setup
			tt.setupFlags(facList, cmd)
			tt.setupMocks(mockRepo)
			
			// Execute
			err := facList.runE(cmd, []string{})
			
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

func TestFactoryTransactionListSetFlags(t *testing.T) {
	// Setup
	f := &factoryTransactionList{}
	cmd := &cobra.Command{}
	
	// Execute
	f.setFlags(cmd)
	
	// Verify
	expectedFlags := []string{
		"organization-id", "ledger-id", "limit", "page", "sort-order", 
		"start-date", "end-date", "help",
	}
	
	for _, flag := range expectedFlags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
	
	// Verify default values
	assert.Equal(t, "10", cmd.Flag("limit").DefValue)
	assert.Equal(t, "1", cmd.Flag("page").DefValue)
	assert.Equal(t, "desc", cmd.Flag("sort-order").DefValue)
}
