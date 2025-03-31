package assetrate

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

func TestNewCmdAssetRateListByAsset(t *testing.T) {
	// Setup
	mockRepo := new(mockAssetRateRepo)
	f := &factoryAssetRateListByAsset{
		repoAssetRate: mockRepo,
	}

	// Execute
	cmd := newCmdAssetRateListByAsset(f)

	// Verify
	assert.Equal(t, "list-by-asset", cmd.Use)
	assert.Equal(t, "Lists asset rates for a specific asset.", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
	assert.NotNil(t, cmd.RunE)

	// Verify flags
	flags := []string{
		"organization-id", "ledger-id", "asset-code", "limit", "page",
		"sort-order", "start-date", "end-date", "help",
	}
	for _, flag := range flags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}
}

func TestNewInjectFacListByAsset(t *testing.T) {
	// Setup
	ios := iostreams.System()
	f := &factory.Factory{
		IOStreams: ios,
	}

	// Execute
	result := newInjectFacListByAsset(f)

	// Verify
	assert.NotNil(t, result)
	assert.Equal(t, f, result.factory)
	assert.NotNil(t, result.repoAssetRate)
	assert.NotNil(t, result.tuiInput)
}

func TestFactoryAssetRateListByAssetRunE(t *testing.T) {
	tests := []struct {
		name          string
		setupFlags    func(*factoryAssetRateListByAsset, *cobra.Command)
		setupMocks    func(*mockAssetRateRepo)
		expectedError string
	}{
		{
			name: "successfully lists asset rates by asset code",
			setupFlags: func(f *factoryAssetRateListByAsset, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AssetCode = "USD"
				f.Limit = 10
				f.Page = 1
				f.SortOrder = "desc"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("asset-code", f.AssetCode)
				cmd.Flags().Set("limit", "10")
				cmd.Flags().Set("page", "1")
				cmd.Flags().Set("sort-order", f.SortOrder)
			},
			setupMocks: func(mockRepo *mockAssetRateRepo) {
				statusDesc := "Active"
				mockRepo.On("GetByAssetCode", "org123", "ledger123", "USD", 10, 1, "desc", "", "").Return(
					&mmodel.AssetRates{
						Items: []mmodel.AssetRate{
							{
								ID:            "ar123",
								FromAssetCode: "USD",
								ToAssetCode:   "EUR",
								Rate:          120,
								RateScale:     2,
								Status: &mmodel.Status{
									Code:        "active",
									Description: &statusDesc,
								},
								CreatedAt: time.Now(),
								UpdatedAt: time.Now(),
							},
							{
								ID:            "ar124",
								FromAssetCode: "USD",
								ToAssetCode:   "GBP",
								Rate:          87,
								RateScale:     2,
								Status: &mmodel.Status{
									Code:        "active",
									Description: &statusDesc,
								},
								CreatedAt: time.Now(),
								UpdatedAt: time.Now(),
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
			name: "successfully handles empty asset rates list",
			setupFlags: func(f *factoryAssetRateListByAsset, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AssetCode = "XYZ"
				f.Limit = 10
				f.Page = 1
				f.SortOrder = "desc"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("asset-code", f.AssetCode)
				cmd.Flags().Set("limit", "10")
				cmd.Flags().Set("page", "1")
				cmd.Flags().Set("sort-order", f.SortOrder)
			},
			setupMocks: func(mockRepo *mockAssetRateRepo) {
				mockRepo.On("GetByAssetCode", "org123", "ledger123", "XYZ", 10, 1, "desc", "", "").Return(
					&mmodel.AssetRates{
						Items: []mmodel.AssetRate{},
						Pagination: &mmodel.Pagination{
							Limit: 10,
							Page:  1,
						},
					}, nil,
				)
			},
		},
		{
			name: "successfully lists asset rates with date filters",
			setupFlags: func(f *factoryAssetRateListByAsset, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AssetCode = "USD"
				f.Limit = 10
				f.Page = 1
				f.SortOrder = "asc"
				f.StartDate = "2023-01-01"
				f.EndDate = "2023-12-31"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("asset-code", f.AssetCode)
				cmd.Flags().Set("limit", "10")
				cmd.Flags().Set("page", "1")
				cmd.Flags().Set("sort-order", f.SortOrder)
				cmd.Flags().Set("start-date", f.StartDate)
				cmd.Flags().Set("end-date", f.EndDate)
			},
			setupMocks: func(mockRepo *mockAssetRateRepo) {
				mockRepo.On("GetByAssetCode", "org123", "ledger123", "USD", 10, 1, "asc", "2023-01-01", "2023-12-31").Return(
					&mmodel.AssetRates{
						Items: []mmodel.AssetRate{
							{
								ID:            "ar123",
								FromAssetCode: "USD",
								ToAssetCode:   "EUR",
								Rate:          120,
								RateScale:     2,
								CreatedAt:     time.Now(),
								UpdatedAt:     time.Now(),
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
			setupFlags: func(f *factoryAssetRateListByAsset, cmd *cobra.Command) {
				f.OrganizationID = ""
				f.tuiInput = func(message string) (string, error) {
					return "", errors.New("input error")
				}
			},
			setupMocks: func(mockRepo *mockAssetRateRepo) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails when ledger ID is missing and input fails",
			setupFlags: func(f *factoryAssetRateListByAsset, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = ""
				f.tuiInput = func(message string) (string, error) {
					if message == "Enter your organization-id" {
						return "org123", nil
					}
					return "", errors.New("input error")
				}
			},
			setupMocks: func(mockRepo *mockAssetRateRepo) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails when asset code is missing and input fails",
			setupFlags: func(f *factoryAssetRateListByAsset, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AssetCode = ""
				f.tuiInput = func(message string) (string, error) {
					if message == "Enter your organization-id" {
						return "org123", nil
					} else if message == "Enter your ledger-id" {
						return "ledger123", nil
					}
					return "", errors.New("input error")
				}
			},
			setupMocks: func(mockRepo *mockAssetRateRepo) {
				// No mock setup needed as it should fail before repository call
			},
			expectedError: "input error",
		},
		{
			name: "fails when repository call fails",
			setupFlags: func(f *factoryAssetRateListByAsset, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AssetCode = "USD"
				f.Limit = 10
				f.Page = 1
				f.SortOrder = "desc"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("asset-code", f.AssetCode)
				cmd.Flags().Set("limit", "10")
				cmd.Flags().Set("page", "1")
				cmd.Flags().Set("sort-order", f.SortOrder)
			},
			setupMocks: func(mockRepo *mockAssetRateRepo) {
				mockRepo.On("GetByAssetCode", "org123", "ledger123", "USD", 10, 1, "desc", "", "").Return(
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
			mockRepo := new(mockAssetRateRepo)

			f := &factory.Factory{
				IOStreams: ios,
			}

			facListByAsset := &factoryAssetRateListByAsset{
				factory:       f,
				repoAssetRate: mockRepo,
				tuiInput: func(message string) (string, error) {
					return "default", nil
				},
			}

			cmd := &cobra.Command{}
			cmd.Flags().String("organization-id", "", "")
			cmd.Flags().String("ledger-id", "", "")
			cmd.Flags().String("asset-code", "", "")
			cmd.Flags().Int("limit", 10, "")
			cmd.Flags().Int("page", 1, "")
			cmd.Flags().String("sort-order", "desc", "")
			cmd.Flags().String("start-date", "", "")
			cmd.Flags().String("end-date", "", "")

			// Apply test-specific setup
			tt.setupFlags(facListByAsset, cmd)
			tt.setupMocks(mockRepo)

			// Execute
			err := facListByAsset.runE(cmd, []string{})

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

func TestFactoryAssetRateListByAssetSetFlags(t *testing.T) {
	// Setup
	f := &factoryAssetRateListByAsset{}
	cmd := &cobra.Command{}

	// Execute
	f.setFlags(cmd)

	// Verify
	expectedFlags := []string{
		"organization-id", "ledger-id", "asset-code", "limit", "page",
		"sort-order", "start-date", "end-date", "help",
	}

	for _, flag := range expectedFlags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}

	// Verify default values
	assert.Equal(t, "10", cmd.Flag("limit").DefValue)
	assert.Equal(t, "1", cmd.Flag("page").DefValue)
	assert.Equal(t, "desc", cmd.Flag("sort-order").DefValue)
}

func TestFactoryAssetRateListByAssetPrintAssetRates(t *testing.T) {
	tests := []struct {
		name       string
		assetRates *mmodel.AssetRates
		assetCode  string
	}{
		{
			name:      "prints asset rates with pagination",
			assetCode: "USD",
			assetRates: &mmodel.AssetRates{
				Items: []mmodel.AssetRate{
					{
						ID:            "ar123",
						FromAssetCode: "USD",
						ToAssetCode:   "EUR",
						Rate:          120,
						RateScale:     2,
						Status: &mmodel.Status{
							Code: "active",
						},
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					},
				},
				Pagination: &mmodel.Pagination{
					Limit: 10,
					Page:  1,
				},
			},
		},
		{
			name:      "prints asset rates without pagination",
			assetCode: "USD",
			assetRates: &mmodel.AssetRates{
				Items: []mmodel.AssetRate{
					{
						ID:            "ar123",
						FromAssetCode: "USD",
						ToAssetCode:   "EUR",
						Rate:          120,
						RateScale:     2,
						CreatedAt:     time.Now(),
						UpdatedAt:     time.Now(),
					},
				},
			},
		},
		{
			name:      "prints empty asset rates list",
			assetCode: "XYZ",
			assetRates: &mmodel.AssetRates{
				Items: []mmodel.AssetRate{},
			},
		},
		{
			name:      "handles asset rates with different rate scales",
			assetCode: "USD",
			assetRates: &mmodel.AssetRates{
				Items: []mmodel.AssetRate{
					{
						ID:            "ar123",
						FromAssetCode: "USD",
						ToAssetCode:   "EUR",
						Rate:          120,
						RateScale:     2, // 1.20
						CreatedAt:     time.Now(),
						UpdatedAt:     time.Now(),
					},
					{
						ID:            "ar124",
						FromAssetCode: "USD",
						ToAssetCode:   "GBP",
						Rate:          8750,
						RateScale:     4, // 0.8750
						CreatedAt:     time.Now(),
						UpdatedAt:     time.Now(),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ios := iostreams.System()
			f := &factory.Factory{
				IOStreams: ios,
			}

			facListByAsset := &factoryAssetRateListByAsset{
				factory: f,
				flagsListByAsset: flagsListByAsset{
					OrganizationID: "org123",
					LedgerID:       "ledger123",
					AssetCode:      tt.assetCode,
					Limit:          10,
					Page:           1,
				},
			}

			// Execute - this is a visual test, we're just ensuring it doesn't panic
			facListByAsset.printAssetRates(tt.assetRates)
		})
	}
}
