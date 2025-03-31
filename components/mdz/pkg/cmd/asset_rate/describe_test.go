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

func TestNewCmdAssetRateDescribe(t *testing.T) {
	// Setup
	mockRepo := new(mockAssetRateRepo)
	f := &factoryAssetRateDescribe{
		repoAssetRate: mockRepo,
	}

	// Execute
	cmd := newCmdAssetRateDescribe(f)

	// Verify
	assert.Equal(t, "describe", cmd.Use)
	assert.Equal(t, "Describes an asset rate.", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
	assert.NotNil(t, cmd.RunE)

	// Verify flags
	flags := []string{
		"organization-id", "ledger-id", "asset-rate-id", "output", "help",
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
	assert.NotNil(t, result.repoAssetRate)
	assert.NotNil(t, result.tuiInput)
}

func TestFactoryAssetRateDescribeRunE(t *testing.T) {
	tests := []struct {
		name          string
		setupFlags    func(*factoryAssetRateDescribe, *cobra.Command)
		setupMocks    func(*mockAssetRateRepo)
		expectedError string
	}{
		{
			name: "successfully describes asset rate with table output",
			setupFlags: func(f *factoryAssetRateDescribe, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AssetRateID = "ar123"
				f.OutputFormat = "table"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("asset-rate-id", f.AssetRateID)
				cmd.Flags().Set("output", f.OutputFormat)
			},
			setupMocks: func(mockRepo *mockAssetRateRepo) {
				statusDesc := "Active"
				mockRepo.On("GetByID", "org123", "ledger123", "ar123").Return(
					&mmodel.AssetRate{
						ID:            "ar123",
						FromAssetCode: "USD",
						ToAssetCode:   "EUR",
						Rate:          120,
						RateScale:     2,
						Status: &mmodel.Status{
							Code:        "active",
							Description: &statusDesc,
						},
						Metadata:  map[string]interface{}{"source": "test"},
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil,
				)
			},
		},
		{
			name: "successfully describes asset rate with json output",
			setupFlags: func(f *factoryAssetRateDescribe, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AssetRateID = "ar123"
				f.OutputFormat = "json"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("asset-rate-id", f.AssetRateID)
				cmd.Flags().Set("output", f.OutputFormat)
			},
			setupMocks: func(mockRepo *mockAssetRateRepo) {
				statusDesc := "Active"
				mockRepo.On("GetByID", "org123", "ledger123", "ar123").Return(
					&mmodel.AssetRate{
						ID:            "ar123",
						FromAssetCode: "USD",
						ToAssetCode:   "EUR",
						Rate:          120,
						RateScale:     2,
						Status: &mmodel.Status{
							Code:        "active",
							Description: &statusDesc,
						},
						Metadata:  map[string]interface{}{"source": "test"},
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil,
				)
			},
		},
		{
			name: "fails when organization ID is missing and input fails",
			setupFlags: func(f *factoryAssetRateDescribe, cmd *cobra.Command) {
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
			setupFlags: func(f *factoryAssetRateDescribe, cmd *cobra.Command) {
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
			name: "fails when asset rate ID is missing and input fails",
			setupFlags: func(f *factoryAssetRateDescribe, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AssetRateID = ""
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
			setupFlags: func(f *factoryAssetRateDescribe, cmd *cobra.Command) {
				f.OrganizationID = "org123"
				f.LedgerID = "ledger123"
				f.AssetRateID = "ar123"

				cmd.Flags().Set("organization-id", f.OrganizationID)
				cmd.Flags().Set("ledger-id", f.LedgerID)
				cmd.Flags().Set("asset-rate-id", f.AssetRateID)
			},
			setupMocks: func(mockRepo *mockAssetRateRepo) {
				mockRepo.On("GetByID", "org123", "ledger123", "ar123").Return(
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

			facDescribe := &factoryAssetRateDescribe{
				factory:       f,
				repoAssetRate: mockRepo,
				tuiInput: func(message string) (string, error) {
					return "default", nil
				},
			}

			cmd := &cobra.Command{}
			cmd.Flags().String("organization-id", "", "")
			cmd.Flags().String("ledger-id", "", "")
			cmd.Flags().String("asset-rate-id", "", "")
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

func TestFactoryAssetRateDescribeSetFlags(t *testing.T) {
	// Setup
	f := &factoryAssetRateDescribe{}
	cmd := &cobra.Command{}

	// Execute
	f.setFlags(cmd)

	// Verify
	expectedFlags := []string{
		"organization-id", "ledger-id", "asset-rate-id", "output", "help",
	}

	for _, flag := range expectedFlags {
		assert.NotNil(t, cmd.Flag(flag), "Flag %s should exist", flag)
	}

	// Verify default values
	assert.Equal(t, "table", cmd.Flag("output").DefValue)
}
